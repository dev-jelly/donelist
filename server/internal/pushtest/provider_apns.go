package notification

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// APNsProvider implements PushNotificationProvider for Apple Push Notification service
type APNsProvider struct {
	config      *PushProviderConfig
	logger      *zap.Logger
	httpClient  *http.Client
	authKey     *ecdsa.PrivateKey
	authToken   atomic.Value // stores current JWT token
	tokenExpiry atomic.Value // stores token expiry time
	metrics     *apnsMetrics
	rateLimiter *rateLimiter
	isAvailable atomic.Bool
	baseURL     string
	mu          sync.RWMutex
}

type apnsMetrics struct {
	totalSent     atomic.Int64
	totalFailed   atomic.Int64
	totalBatches  atomic.Int64
	lastError     atomic.Value
	lastErrorTime atomic.Value
}

const (
	apnsDevelopmentURL = "https://api.development.push.apple.com"
	apnsProductionURL  = "https://api.push.apple.com"
	apnsTokenTTL       = 55 * time.Minute // Apple recommends refreshing every hour
)

// NewAPNsProvider creates a new APNs push notification provider
func NewAPNsProvider(config *PushProviderConfig, logger *zap.Logger) (*APNsProvider, error) {
	if config.APNsAuthKeyPath == "" {
		return nil, fmt.Errorf("APNs auth key path is required")
	}
	if config.APNsKeyID == "" {
		return nil, fmt.Errorf("APNs key ID is required")
	}
	if config.APNsTeamID == "" {
		return nil, fmt.Errorf("APNs team ID is required")
	}
	if config.APNsBundleID == "" {
		return nil, fmt.Errorf("APNs bundle ID is required")
	}

	// Load the auth key
	authKey, err := loadAPNsAuthKey(config.APNsAuthKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load APNs auth key: %w", err)
	}

	// Determine the base URL
	baseURL := apnsDevelopmentURL
	if config.APNsProduction {
		baseURL = apnsProductionURL
	}

	provider := &APNsProvider{
		config:      config,
		logger:      logger,
		authKey:     authKey,
		metrics:     &apnsMetrics{},
		rateLimiter: newRateLimiter(config.RateLimitPerSec),
		baseURL:     baseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	// Generate initial auth token
	if err := provider.refreshAuthToken(); err != nil {
		return nil, fmt.Errorf("failed to generate initial auth token: %w", err)
	}

	provider.isAvailable.Store(true)

	// Start token refresh goroutine
	go provider.tokenRefreshLoop()

	// Start health check goroutine
	go provider.healthCheck()

	logger.Info("APNs provider initialized successfully",
		zap.String("environment", map[bool]string{true: "production", false: "development"}[config.APNsProduction]))

	return provider, nil
}

// Send sends a single push notification via APNs
func (p *APNsProvider) Send(ctx context.Context, notification *PushNotification) (*PushResult, error) {
	if !p.isAvailable.Load() {
		return &PushResult{
			Success:   false,
			Error:     ErrProviderUnavailable,
			Retryable: true,
			SentAt:    time.Now(),
		}, ErrProviderUnavailable
	}

	// Validate notification
	if err := ValidatePushNotification(notification); err != nil {
		return &PushResult{
			Success: false,
			Error:   err,
			SentAt:  time.Now(),
		}, err
	}

	// Validate token format for APNs
	formattedToken := FormatDeviceToken(notification.DeviceToken, PushProviderAPNs)
	if len(formattedToken) != 64 { // APNs tokens are 32 bytes (64 hex characters)
		return &PushResult{
			Success:           false,
			Error:             ErrInvalidToken,
			ShouldRemoveToken: true,
			SentAt:            time.Now(),
		}, ErrInvalidToken
	}

	// Apply rate limiting
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return &PushResult{
			Success:   false,
			Error:     ErrRateLimitExceeded,
			Retryable: true,
			SentAt:    time.Now(),
		}, ErrRateLimitExceeded
	}

	// Send the notification
	result, err := p.sendAPNsNotification(ctx, formattedToken, notification)
	if err != nil {
		p.metrics.totalFailed.Add(1)
		p.metrics.lastError.Store(err.Error())
		p.metrics.lastErrorTime.Store(time.Now())
	} else if result.Success {
		p.metrics.totalSent.Add(1)
	}

	return result, err
}

// SendBatch sends multiple notifications in a batch
func (p *APNsProvider) SendBatch(ctx context.Context, notifications []*PushNotification) ([]*PushResult, error) {
	if !p.isAvailable.Load() {
		results := make([]*PushResult, len(notifications))
		for i := range results {
			results[i] = &PushResult{
				Success:   false,
				Error:     ErrProviderUnavailable,
				Retryable: true,
				SentAt:    time.Now(),
			}
		}
		return results, ErrProviderUnavailable
	}

	results := make([]*PushResult, len(notifications))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent requests

	p.metrics.totalBatches.Add(1)

	for i, notification := range notifications {
		wg.Add(1)
		go func(idx int, notif *PushNotification) {
			defer wg.Done()

			semaphore <- struct{}{} // Acquire
			defer func() { <-semaphore }() // Release

			result, err := p.Send(ctx, notif)
			results[idx] = result

			if err != nil {
				p.logger.Debug("Failed to send APNs notification in batch",
					zap.Int("index", idx),
					zap.Error(err))
			}
		}(i, notification)
	}

	wg.Wait()
	return results, nil
}

// ValidateToken validates an APNs device token
func (p *APNsProvider) ValidateToken(token string) error {
	if token == "" {
		return ErrInvalidToken
	}

	// Format the token
	formatted := FormatDeviceToken(token, PushProviderAPNs)

	// APNs tokens are exactly 64 hex characters (32 bytes)
	if len(formatted) != 64 {
		return ErrInvalidToken
	}

	// Verify all characters are hex
	for _, r := range formatted {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return ErrInvalidToken
		}
	}

	return nil
}

// GetProviderType returns the provider type
func (p *APNsProvider) GetProviderType() PushProvider {
	return PushProviderAPNs
}

// IsAvailable checks if the provider is available
func (p *APNsProvider) IsAvailable(ctx context.Context) bool {
	return p.isAvailable.Load()
}

// GetMetrics returns provider metrics
func (p *APNsProvider) GetMetrics() map[string]interface{} {
	lastError := ""
	if v := p.metrics.lastError.Load(); v != nil {
		lastError = v.(string)
	}

	lastErrorTime := time.Time{}
	if v := p.metrics.lastErrorTime.Load(); v != nil {
		lastErrorTime = v.(time.Time)
	}

	return map[string]interface{}{
		"provider":        "apns",
		"environment":     map[bool]string{true: "production", false: "development"}[p.config.APNsProduction],
		"total_sent":      p.metrics.totalSent.Load(),
		"total_failed":    p.metrics.totalFailed.Load(),
		"total_batches":   p.metrics.totalBatches.Load(),
		"is_available":    p.isAvailable.Load(),
		"last_error":      lastError,
		"last_error_time": lastErrorTime,
		"success_rate":    p.calculateSuccessRate(),
	}
}

// Close closes the provider
func (p *APNsProvider) Close() error {
	p.isAvailable.Store(false)
	p.httpClient.CloseIdleConnections()
	p.logger.Info("APNs provider closed")
	return nil
}

// sendAPNsNotification sends a notification to APNs
func (p *APNsProvider) sendAPNsNotification(ctx context.Context, deviceToken string, notification *PushNotification) (*PushResult, error) {
	// Build the payload
	payload := p.buildAPNsPayload(notification)

	// Create the URL
	url := fmt.Sprintf("%s/3/device/%s", p.baseURL, deviceToken)

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	p.setAPNsHeaders(req, notification)

	// Get auth token
	token := p.authToken.Load()
	if token == nil {
		if err := p.refreshAuthToken(); err != nil {
			return &PushResult{
				Success:   false,
				Error:     ErrAuthenticationFailed,
				Retryable: false,
				SentAt:    time.Now(),
			}, ErrAuthenticationFailed
		}
		token = p.authToken.Load()
	}
	req.Header.Set("Authorization", fmt.Sprintf("bearer %s", token.(string)))

	// Send the request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &PushResult{
			Success:   false,
			Error:     err,
			Retryable: true,
			SentAt:    time.Now(),
		}, err
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	// Handle response
	return p.handleAPNsResponse(resp.StatusCode, resp.Header, body)
}

// buildAPNsPayload builds the APNs payload JSON
func (p *APNsProvider) buildAPNsPayload(notification *PushNotification) string {
	var builder strings.Builder
	builder.WriteString(`{"aps":{`)

	if !notification.Silent {
		// Add alert
		builder.WriteString(`"alert":{`)
		builder.WriteString(`"title":"`)
		builder.WriteString(escapeJSON(notification.Title))
		builder.WriteString(`","body":"`)
		builder.WriteString(escapeJSON(notification.Body))
		builder.WriteString(`"}`)

		// Add sound
		if notification.Sound != "" {
			builder.WriteString(`,"sound":"`)
			builder.WriteString(notification.Sound)
			builder.WriteString(`"`)
		}
	}

	// Add badge
	if notification.Badge != nil {
		if !notification.Silent {
			builder.WriteString(`,`)
		}
		builder.WriteString(fmt.Sprintf(`"badge":%d`, *notification.Badge))
	}

	// Add category
	if notification.Category != "" {
		builder.WriteString(`,"category":"`)
		builder.WriteString(notification.Category)
		builder.WriteString(`"`)
	}

	// Add thread-id
	if notification.ThreadID != "" {
		builder.WriteString(`,"thread-id":"`)
		builder.WriteString(notification.ThreadID)
		builder.WriteString(`"`)
	}

	// Add content-available for silent notifications or background updates
	if notification.Silent || notification.ContentAvailable {
		builder.WriteString(`,"content-available":1`)
	}

	// Add mutable-content
	if notification.MutableContent {
		builder.WriteString(`,"mutable-content":1`)
	}

	builder.WriteString(`}`)

	// Add custom data
	if len(notification.Data) > 0 {
		for k, v := range notification.Data {
			builder.WriteString(`,"`)
			builder.WriteString(k)
			builder.WriteString(`":"`)
			builder.WriteString(escapeJSON(v))
			builder.WriteString(`"`)
		}
	}

	builder.WriteString(`}`)

	return builder.String()
}

// setAPNsHeaders sets the required headers for APNs request
func (p *APNsProvider) setAPNsHeaders(req *http.Request, notification *PushNotification) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apns-topic", p.config.APNsBundleID)

	// Set priority
	if notification.HighPriority {
		req.Header.Set("apns-priority", "10")
	} else {
		req.Header.Set("apns-priority", "5")
	}

	// Set expiration
	if notification.TTL > 0 {
		expiration := time.Now().Add(time.Duration(notification.TTL) * time.Second).Unix()
		req.Header.Set("apns-expiration", fmt.Sprintf("%d", expiration))
	}

	// Set collapse ID
	if notification.CollapseKey != "" {
		req.Header.Set("apns-collapse-id", notification.CollapseKey)
	}

	// Set push type
	if notification.Silent {
		req.Header.Set("apns-push-type", "background")
	} else {
		req.Header.Set("apns-push-type", "alert")
	}
}

// handleAPNsResponse handles the APNs HTTP/2 response
func (p *APNsProvider) handleAPNsResponse(statusCode int, headers http.Header, body []byte) (*PushResult, error) {
	result := &PushResult{
		SentAt: time.Now(),
	}

	// Get APNs ID from response
	if apnsID := headers.Get("apns-id"); apnsID != "" {
		result.MessageID = apnsID
	}

	switch statusCode {
	case http.StatusOK:
		result.Success = true
		return result, nil

	case http.StatusBadRequest:
		// Parse error reason from body
		reason := string(body)
		switch {
		case strings.Contains(reason, "BadDeviceToken"):
			result.Error = ErrInvalidToken
			result.ShouldRemoveToken = true

		case strings.Contains(reason, "Unregistered"):
			result.Error = ErrTokenExpired
			result.ShouldRemoveToken = true

		case strings.Contains(reason, "PayloadTooLarge"):
			result.Error = ErrPayloadTooLarge

		case strings.Contains(reason, "TooManyRequests"):
			result.Error = ErrRateLimitExceeded
			result.Retryable = true
			retryAfter := time.Minute
			result.RetryAfter = &retryAfter

		default:
			result.Error = fmt.Errorf("bad request: %s", reason)
		}

	case http.StatusForbidden:
		result.Error = ErrAuthenticationFailed
		p.isAvailable.Store(false)

	case http.StatusGone:
		// The device token is no longer valid
		result.Error = ErrTokenExpired
		result.ShouldRemoveToken = true

	case http.StatusTooManyRequests:
		result.Error = ErrRateLimitExceeded
		result.Retryable = true

		// Check for Retry-After header
		if retryAfter := headers.Get("Retry-After"); retryAfter != "" {
			if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
				result.RetryAfter = &seconds
			}
		}

	case http.StatusServiceUnavailable:
		result.Error = ErrProviderUnavailable
		result.Retryable = true

	default:
		result.Error = fmt.Errorf("unexpected status code: %d", statusCode)
		result.Retryable = statusCode >= 500
	}

	return result, result.Error
}

// refreshAuthToken generates a new JWT token for APNs authentication
func (p *APNsProvider) refreshAuthToken() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	// Create the JWT claims
	claims := jwt.MapClaims{
		"iss": p.config.APNsTeamID,
		"iat": now.Unix(),
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = p.config.APNsKeyID

	// Sign the token
	tokenString, err := token.SignedString(p.authKey)
	if err != nil {
		return fmt.Errorf("failed to sign JWT token: %w", err)
	}

	// Store the token and expiry
	p.authToken.Store(tokenString)
	p.tokenExpiry.Store(now.Add(apnsTokenTTL))

	p.logger.Debug("APNs auth token refreshed")

	return nil
}

// tokenRefreshLoop periodically refreshes the auth token
func (p *APNsProvider) tokenRefreshLoop() {
	ticker := time.NewTicker(50 * time.Minute) // Refresh before expiry
	defer ticker.Stop()

	for range ticker.C {
		if !p.isAvailable.Load() {
			continue
		}

		if err := p.refreshAuthToken(); err != nil {
			p.logger.Error("Failed to refresh APNs auth token", zap.Error(err))
			p.isAvailable.Store(false)
		}
	}
}

// healthCheck performs periodic health checks
func (p *APNsProvider) healthCheck() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		successRate := p.calculateSuccessRate()
		if successRate < 0.5 && p.metrics.totalSent.Load() > 100 {
			p.logger.Warn("APNs provider success rate is low",
				zap.Float64("success_rate", successRate))
		}

		// Check token expiry
		if expiry := p.tokenExpiry.Load(); expiry != nil {
			if time.Now().After(expiry.(time.Time)) {
				p.logger.Warn("APNs auth token has expired, refreshing")
				if err := p.refreshAuthToken(); err != nil {
					p.logger.Error("Failed to refresh expired APNs auth token", zap.Error(err))
					p.isAvailable.Store(false)
				}
			}
		}
	}
}

// calculateSuccessRate calculates the success rate of sent notifications
func (p *APNsProvider) calculateSuccessRate() float64 {
	sent := p.metrics.totalSent.Load()
	failed := p.metrics.totalFailed.Load()
	total := sent + failed

	if total == 0 {
		return 1.0
	}

	return float64(sent) / float64(total)
}

// loadAPNsAuthKey loads the APNs auth key from a .p8 file
func loadAPNsAuthKey(path string) (*ecdsa.PrivateKey, error) {
	// Read the file
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read auth key file: %w", err)
	}

	// Decode PEM
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Parse the key
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Assert to ECDSA key
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("auth key is not an ECDSA key")
	}

	return ecdsaKey, nil
}

// escapeJSON escapes a string for use in JSON
func escapeJSON(s string) string {
	// Simple JSON escaping - in production, use proper JSON encoding
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
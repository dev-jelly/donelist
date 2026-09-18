package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// IAPPlatform represents the mobile platform
type IAPPlatform string

const (
	IAPPlatformApple  IAPPlatform = "apple"
	IAPPlatformGoogle IAPPlatform = "google"
)

// IAPService handles In-App Purchase validation for iOS and Android
type IAPService struct {
	logger           *zap.Logger
	subscriptionRepo *subscription.Repository
	appleConfig      *AppleIAPConfig
	googleConfig     *GoogleIAPConfig
	httpClient       *http.Client
}

// AppleIAPConfig contains Apple App Store configuration
type AppleIAPConfig struct {
	SharedSecret    string // App-specific shared secret for receipt validation
	BundleID        string // App bundle ID
	UseSandbox      bool   // Use sandbox environment for testing
	ProductionURL   string // Production verification URL
	SandboxURL      string // Sandbox verification URL
}

// GoogleIAPConfig contains Google Play Store configuration
type GoogleIAPConfig struct {
	ServiceAccountJSON []byte // Service account credentials JSON
	PackageName        string // App package name
}

// NewIAPService creates a new IAP validation service
func NewIAPService(logger *zap.Logger, subscriptionRepo *subscription.Repository, appleConfig *AppleIAPConfig, googleConfig *GoogleIAPConfig) *IAPService {
	// Set default Apple URLs if not provided
	if appleConfig != nil {
		if appleConfig.ProductionURL == "" {
			appleConfig.ProductionURL = "https://buy.itunes.apple.com/verifyReceipt"
		}
		if appleConfig.SandboxURL == "" {
			appleConfig.SandboxURL = "https://sandbox.itunes.apple.com/verifyReceipt"
		}
	}

	return &IAPService{
		logger:           logger,
		subscriptionRepo: subscriptionRepo,
		appleConfig:      appleConfig,
		googleConfig:     googleConfig,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// VerifyReceipt verifies a receipt from either iOS or Android
func (s *IAPService) VerifyReceipt(ctx context.Context, platform IAPPlatform, receiptData string, userID uuid.UUID) (*IAPValidationResult, error) {
	s.logger.Info("Verifying IAP receipt",
		zap.String("platform", string(platform)),
		zap.String("user_id", userID.String()),
	)

	switch platform {
	case IAPPlatformApple:
		return s.verifyAppleReceipt(ctx, receiptData, userID)
	case IAPPlatformGoogle:
		return s.verifyGoogleReceipt(ctx, receiptData, userID)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// verifyAppleReceipt verifies an iOS App Store receipt
func (s *IAPService) verifyAppleReceipt(ctx context.Context, receiptData string, userID uuid.UUID) (*IAPValidationResult, error) {
	if s.appleConfig == nil {
		return nil, fmt.Errorf("Apple IAP not configured")
	}

	// Build request payload
	requestBody := map[string]interface{}{
		"receipt-data":             receiptData,
		"password":                 s.appleConfig.SharedSecret,
		"exclude-old-transactions": true,
	}

	requestBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Try production first
	url := s.appleConfig.ProductionURL
	if s.appleConfig.UseSandbox {
		url = s.appleConfig.SandboxURL
	}

	response, err := s.verifyAppleReceiptWithURL(ctx, url, requestBytes)
	if err != nil {
		return nil, err
	}

	// If production returns sandbox receipt error, try sandbox
	if response.Status == 21007 && !s.appleConfig.UseSandbox {
		s.logger.Info("Receipt is from sandbox, retrying with sandbox URL")
		url = s.appleConfig.SandboxURL
		response, err = s.verifyAppleReceiptWithURL(ctx, url, requestBytes)
		if err != nil {
			return nil, err
		}
	}

	// Parse and validate the response
	return s.parseAppleResponse(response, userID)
}

// verifyAppleReceiptWithURL makes the actual HTTP request to Apple
func (s *IAPService) verifyAppleReceiptWithURL(ctx context.Context, url string, requestBody []byte) (*AppleReceiptResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var appleResp AppleReceiptResponse
	if err := json.NewDecoder(resp.Body).Decode(&appleResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &appleResp, nil
}

// parseAppleResponse parses the Apple verification response
func (s *IAPService) parseAppleResponse(response *AppleReceiptResponse, userID uuid.UUID) (*IAPValidationResult, error) {
	if response.Status != 0 {
		return nil, fmt.Errorf("apple receipt validation failed: status=%d", response.Status)
	}

	// Verify bundle ID
	if response.Receipt.BundleID != s.appleConfig.BundleID {
		return nil, fmt.Errorf("bundle ID mismatch: expected=%s, got=%s", s.appleConfig.BundleID, response.Receipt.BundleID)
	}

	// Get the latest receipt info
	if len(response.LatestReceiptInfo) == 0 {
		return nil, fmt.Errorf("no purchase information found")
	}

	latestInfo := response.LatestReceiptInfo[0]

	// Parse dates
	expiresDate, err := parseAppleDate(latestInfo.ExpiresDateMS)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expiry date: %w", err)
	}

	purchaseDate, err := parseAppleDate(latestInfo.PurchaseDateMS)
	if err != nil {
		return nil, fmt.Errorf("failed to parse purchase date: %w", err)
	}

	result := &IAPValidationResult{
		Platform:         IAPPlatformApple,
		TransactionID:    latestInfo.TransactionID,
		OriginalTransactionID: latestInfo.OriginalTransactionID,
		ProductID:        latestInfo.ProductID,
		PurchaseDate:     purchaseDate,
		ExpiryDate:       &expiresDate,
		IsActive:         time.Now().Before(expiresDate),
		AutoRenewing:     latestInfo.AutoRenewStatus == "1",
	}

	// Check cancellation
	if latestInfo.CancellationDateMS != "" {
		cancelDate, err := parseAppleDate(latestInfo.CancellationDateMS)
		if err == nil {
			result.CancellationDate = &cancelDate
			result.IsActive = false
		}
	}

	return result, nil
}

// verifyGoogleReceipt verifies an Android Play Store receipt
func (s *IAPService) verifyGoogleReceipt(ctx context.Context, purchaseToken string, userID uuid.UUID) (*IAPValidationResult, error) {
	if s.googleConfig == nil {
		return nil, fmt.Errorf("Google IAP not configured")
	}

	// TODO: Implement Google Play billing API verification
	// This requires using the Google Play Developer API client library
	// and validating the purchase token with the subscription.get endpoint

	s.logger.Warn("Google IAP verification not fully implemented yet",
		zap.String("user_id", userID.String()),
	)

	return nil, fmt.Errorf("Google IAP verification not yet implemented")
}

// SyncSubscriptionFromReceipt synchronizes a subscription based on verified receipt
func (s *IAPService) SyncSubscriptionFromReceipt(ctx context.Context, userID uuid.UUID, result *IAPValidationResult) error {
	// Check if subscription already exists
	existingSub, err := s.subscriptionRepo.GetSubscriptionByUserID(ctx, userID)
	if err != nil && existingSub == nil {
		// Create new subscription
		sub := &subscription.Subscription{
			ID:                 uuid.New(),
			UserID:             userID,
			PlanID:             result.ProductID,
			Status:             mapIAPStatusToSubscriptionStatus(result),
			CurrentPeriodStart: result.PurchaseDate,
			CurrentPeriodEnd:   *result.ExpiryDate,
			CancelAtPeriodEnd:  !result.AutoRenewing,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
			Metadata: subscription.JSONB{
				"platform":                string(result.Platform),
				"transaction_id":          result.TransactionID,
				"original_transaction_id": result.OriginalTransactionID,
			},
		}

		if result.CancellationDate != nil {
			sub.CanceledAt = result.CancellationDate
		}

		if err := s.subscriptionRepo.CreateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("failed to create subscription: %w", err)
		}

		// Log event
		if err := s.subscriptionRepo.LogSubscriptionEvent(ctx, sub.ID, subscription.EventTypeCreated, nil, sub, "Created from IAP receipt"); err != nil {
			s.logger.Warn("Failed to log subscription event", zap.Error(err))
		}

		s.logger.Info("Created subscription from IAP receipt",
			zap.String("user_id", userID.String()),
			zap.String("platform", string(result.Platform)),
		)

		return nil
	}

	// Update existing subscription
	if existingSub != nil {
		previousState := *existingSub
		existingSub.Status = mapIAPStatusToSubscriptionStatus(result)
		existingSub.CurrentPeriodStart = result.PurchaseDate
		existingSub.CurrentPeriodEnd = *result.ExpiryDate
		existingSub.CancelAtPeriodEnd = !result.AutoRenewing
		existingSub.UpdatedAt = time.Now()

		if result.CancellationDate != nil {
			existingSub.CanceledAt = result.CancellationDate
		}

		// Update metadata
		if existingSub.Metadata == nil {
			existingSub.Metadata = make(subscription.JSONB)
		}
		existingSub.Metadata["platform"] = string(result.Platform)
		existingSub.Metadata["transaction_id"] = result.TransactionID
		existingSub.Metadata["original_transaction_id"] = result.OriginalTransactionID

		if err := s.subscriptionRepo.UpdateSubscription(ctx, existingSub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Log event
		if err := s.subscriptionRepo.LogSubscriptionEvent(ctx, existingSub.ID, subscription.EventTypeActivated, previousState, existingSub, "Updated from IAP receipt"); err != nil {
			s.logger.Warn("Failed to log subscription event", zap.Error(err))
		}

		s.logger.Info("Updated subscription from IAP receipt",
			zap.String("user_id", userID.String()),
			zap.String("platform", string(result.Platform)),
		)
	}

	return nil
}

// Helper functions

func parseAppleDate(dateMS string) (time.Time, error) {
	if dateMS == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}

	var ms int64
	if _, err := fmt.Sscanf(dateMS, "%d", &ms); err != nil {
		return time.Time{}, err
	}

	return time.Unix(ms/1000, (ms%1000)*1000000), nil
}

func mapIAPStatusToSubscriptionStatus(result *IAPValidationResult) subscription.SubscriptionStatus {
	if !result.IsActive {
		if result.CancellationDate != nil {
			return subscription.StatusCanceled
		}
		return subscription.StatusExpired
	}

	if result.ExpiryDate != nil && time.Now().After(*result.ExpiryDate) {
		return subscription.StatusExpired
	}

	return subscription.StatusActive
}

// Type definitions for Apple Receipt Response

type AppleReceiptResponse struct {
	Status            int                    `json:"status"`
	Environment       string                 `json:"environment"`
	Receipt           AppleReceipt           `json:"receipt"`
	LatestReceiptInfo []AppleReceiptInfo     `json:"latest_receipt_info"`
	LatestReceipt     string                 `json:"latest_receipt"`
	PendingRenewalInfo []ApplePendingRenewal `json:"pending_renewal_info"`
}

type AppleReceipt struct {
	BundleID           string             `json:"bundle_id"`
	ApplicationVersion string             `json:"application_version"`
	InApp              []AppleReceiptInfo `json:"in_app"`
}

type AppleReceiptInfo struct {
	TransactionID         string `json:"transaction_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
	ProductID             string `json:"product_id"`
	PurchaseDateMS        string `json:"purchase_date_ms"`
	ExpiresDateMS         string `json:"expires_date_ms"`
	CancellationDateMS    string `json:"cancellation_date_ms"`
	AutoRenewStatus       string `json:"auto_renew_status"`
}

type ApplePendingRenewal struct {
	AutoRenewProductID string `json:"auto_renew_product_id"`
	AutoRenewStatus    string `json:"auto_renew_status"`
	ExpirationIntent   string `json:"expiration_intent"`
}

// IAPValidationResult contains the result of receipt validation
type IAPValidationResult struct {
	Platform              IAPPlatform
	TransactionID         string
	OriginalTransactionID string
	ProductID             string
	PurchaseDate          time.Time
	ExpiryDate            *time.Time
	CancellationDate      *time.Time
	IsActive              bool
	AutoRenewing          bool
}

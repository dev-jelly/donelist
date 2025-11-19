package websocket

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ReconnectConfig contains configuration for reconnection logic
type ReconnectConfig struct {
	InitialInterval     time.Duration
	MaxInterval         time.Duration
	MaxElapsedTime      time.Duration
	Multiplier          float64
	RandomizationFactor float64
}

// DefaultReconnectConfig returns default reconnection configuration
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialInterval:     1 * time.Second,
		MaxInterval:         60 * time.Second,
		MaxElapsedTime:      15 * time.Minute,
		Multiplier:          2.0,
		RandomizationFactor: 0.5,
	}
}

// ReconnectManager manages client reconnection state and logic
type ReconnectManager struct {
	config        ReconnectConfig
	logger        *zap.Logger
	sessions      map[string]*ReconnectSession
	offlineQueue  *OfflineQueue
	mu            sync.RWMutex
}

// ReconnectSession tracks reconnection state for a client
type ReconnectSession struct {
	UserID            string
	SessionID         string
	LastDisconnect    time.Time
	ReconnectAttempts int
	NextRetryTime     time.Time
	CurrentInterval   time.Duration
}

// NewReconnectManager creates a new reconnection manager
func NewReconnectManager(cfg ReconnectConfig, offlineQueue *OfflineQueue, logger *zap.Logger) *ReconnectManager {
	if cfg.InitialInterval == 0 {
		cfg = DefaultReconnectConfig()
	}

	return &ReconnectManager{
		config:       cfg,
		logger:       logger,
		sessions:     make(map[string]*ReconnectSession),
		offlineQueue: offlineQueue,
	}
}

// OnClientDisconnect handles client disconnection
func (rm *ReconnectManager) OnClientDisconnect(userID string) string {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	sessionID := uuid.New().String()

	session := &ReconnectSession{
		UserID:            userID,
		SessionID:         sessionID,
		LastDisconnect:    time.Now(),
		ReconnectAttempts: 0,
		CurrentInterval:   rm.config.InitialInterval,
		NextRetryTime:     time.Now().Add(rm.config.InitialInterval),
	}

	rm.sessions[sessionID] = session

	rm.logger.Info("Client disconnected, session created",
		zap.String("user_id", userID),
		zap.String("session_id", sessionID),
		zap.Duration("next_retry_in", rm.config.InitialInterval),
	)

	return sessionID
}

// OnClientReconnect handles client reconnection attempt
func (rm *ReconnectManager) OnClientReconnect(sessionID string, userID string) (*ReconnectSession, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	session, exists := rm.sessions[sessionID]
	if !exists {
		// Create new session if doesn't exist
		session = &ReconnectSession{
			UserID:            userID,
			SessionID:         sessionID,
			LastDisconnect:    time.Now(),
			ReconnectAttempts: 0,
			CurrentInterval:   rm.config.InitialInterval,
		}
		rm.sessions[sessionID] = session
	}

	// Check if session belongs to the user
	if session.UserID != userID {
		rm.logger.Warn("Session user mismatch",
			zap.String("session_id", sessionID),
			zap.String("expected_user", session.UserID),
			zap.String("actual_user", userID),
		)
		return nil, nil
	}

	// Update reconnection attempt
	session.ReconnectAttempts++

	// Check if max elapsed time exceeded
	if time.Since(session.LastDisconnect) > rm.config.MaxElapsedTime {
		rm.logger.Info("Session expired",
			zap.String("session_id", sessionID),
			zap.String("user_id", userID),
			zap.Duration("elapsed", time.Since(session.LastDisconnect)),
		)
		delete(rm.sessions, sessionID)
		return nil, nil
	}

	rm.logger.Info("Client reconnected",
		zap.String("user_id", userID),
		zap.String("session_id", sessionID),
		zap.Int("attempts", session.ReconnectAttempts),
		zap.Duration("after", time.Since(session.LastDisconnect)),
	)

	// Successfully reconnected, remove session
	delete(rm.sessions, sessionID)

	return session, nil
}

// CalculateNextInterval calculates the next retry interval with exponential backoff
func (rm *ReconnectManager) CalculateNextInterval(session *ReconnectSession) time.Duration {
	// Apply multiplier
	nextInterval := time.Duration(float64(session.CurrentInterval) * rm.config.Multiplier)

	// Apply randomization
	if rm.config.RandomizationFactor > 0 {
		delta := rm.config.RandomizationFactor * float64(nextInterval)
		minInterval := float64(nextInterval) - delta
		maxInterval := float64(nextInterval) + delta

		// Random value between min and max
		randomFactor := minInterval + (maxInterval-minInterval)*0.5 // Simplified random
		nextInterval = time.Duration(randomFactor)
	}

	// Cap at max interval
	if nextInterval > rm.config.MaxInterval {
		nextInterval = rm.config.MaxInterval
	}

	return nextInterval
}

// GetReconnectInfo returns reconnection information for client
func (rm *ReconnectManager) GetReconnectInfo(sessionID string) map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	session, exists := rm.sessions[sessionID]
	if !exists {
		return map[string]interface{}{
			"should_reconnect": true,
			"interval":        rm.config.InitialInterval.Milliseconds(),
			"max_interval":    rm.config.MaxInterval.Milliseconds(),
			"multiplier":      rm.config.Multiplier,
		}
	}

	nextInterval := rm.CalculateNextInterval(session)

	return map[string]interface{}{
		"should_reconnect": true,
		"session_id":       session.SessionID,
		"interval":         nextInterval.Milliseconds(),
		"max_interval":     rm.config.MaxInterval.Milliseconds(),
		"attempt":         session.ReconnectAttempts + 1,
		"multiplier":      rm.config.Multiplier,
	}
}

// CleanupExpiredSessions removes expired reconnection sessions
func (rm *ReconnectManager) CleanupExpiredSessions() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	expiredSessions := []string{}

	for sessionID, session := range rm.sessions {
		if now.Sub(session.LastDisconnect) > rm.config.MaxElapsedTime {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	for _, sessionID := range expiredSessions {
		delete(rm.sessions, sessionID)
		rm.logger.Debug("Cleaned up expired session",
			zap.String("session_id", sessionID),
		)
	}
}

// StartCleanupWorker starts a background worker to clean up expired sessions
func (rm *ReconnectManager) StartCleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.CleanupExpiredSessions()
		}
	}
}

// HandleReconnectWithOfflineDelivery handles reconnection with offline message delivery
func (rm *ReconnectManager) HandleReconnectWithOfflineDelivery(
	ctx context.Context,
	client *Client,
	sessionID string,
) error {
	// Validate reconnection
	session, err := rm.OnClientReconnect(sessionID, client.userID)
	if err != nil {
		return err
	}

	// Deliver offline messages if valid session
	if session != nil && rm.offlineQueue != nil {
		messages, err := rm.offlineQueue.DeliverMessages(ctx, client.userID)
		if err != nil {
			rm.logger.Error("Failed to deliver offline messages",
				zap.Error(err),
				zap.String("user_id", client.userID),
			)
			return err
		}

		// Send offline messages to client
		for _, msg := range messages {
			if data, err := msg.Marshal(); err == nil {
				select {
				case client.Send <- data:
				default:
					rm.logger.Warn("Failed to send offline message to client",
						zap.String("user_id", client.userID),
					)
				}
			}
		}

		rm.logger.Info("Delivered offline messages",
			zap.String("user_id", client.userID),
			zap.Int("count", len(messages)),
		)
	}

	return nil
}

// ExponentialBackoff implements exponential backoff algorithm
type ExponentialBackoff struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
	current    time.Duration
	mu         sync.Mutex
}

// NewExponentialBackoff creates a new exponential backoff
func NewExponentialBackoff(initial, max time.Duration, multiplier float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		Initial:    initial,
		Max:        max,
		Multiplier: multiplier,
		current:    initial,
	}
}

// NextBackoff returns the next backoff duration
func (b *ExponentialBackoff) NextBackoff() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	current := b.current
	b.current = time.Duration(math.Min(float64(b.Max), float64(b.current)*b.Multiplier))

	return current
}

// Reset resets the backoff to initial value
func (b *ExponentialBackoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current = b.Initial
}
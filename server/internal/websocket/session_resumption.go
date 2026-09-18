package websocket

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// SessionState represents the state that can be resumed
type SessionState struct {
	SessionID      string                 `json:"session_id"`
	UserID         string                 `json:"user_id"`
	LastAckSeq     uint64                 `json:"last_ack_seq"`
	LastMessageID  string                 `json:"last_message_id"`
	Subscriptions  []string               `json:"subscriptions"`
	ConnectionTime time.Time              `json:"connection_time"`
	LastActivity   time.Time              `json:"last_activity"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Snapshot       json.RawMessage        `json:"snapshot,omitempty"`
}

// ReconnectionManager handles advanced reconnection with session resumption
type ReconnectionManager struct {
	redis  *redis.Client
	logger *zap.Logger

	// Configuration
	config ReconnectionConfig

	// Session management
	sessions      map[string]*SessionState
	sessionsMu    sync.RWMutex

	// Reconnection tracking
	reconnectAttempts map[string]*ReconnectAttempt
	attemptsMu        sync.RWMutex

	// Metrics
	totalReconnects   uint64
	successfulResumes uint64
	fullResyncs       uint64

	// Handlers
	snapshotProvider   func(userID string) (json.RawMessage, error)
	sequenceProvider   func(userID string) uint64
	subscriptionLoader func(userID string) []string
}

// ReconnectionConfig contains advanced reconnection settings
type ReconnectionConfig struct {
	// Backoff settings
	BaseInterval        time.Duration
	MaxInterval         time.Duration
	Multiplier          float64
	JitterFactor        float64    // 0.0 to 1.0
	MaxRetries          int

	// Session settings
	SessionTTL          time.Duration
	ResumptionWindow    time.Duration
	SnapshotThreshold   int        // Number of missed messages before full resync

	// Feature flags
	EnableSessionResume bool
	EnableCompression   bool
	EnableMetrics       bool
}

// DefaultReconnectionConfig returns optimized reconnection configuration
func DefaultReconnectionConfig() ReconnectionConfig {
	return ReconnectionConfig{
		BaseInterval:        1 * time.Second,
		MaxInterval:         60 * time.Second,
		Multiplier:          2.0,
		JitterFactor:        0.3,
		MaxRetries:          10,
		SessionTTL:          24 * time.Hour,
		ResumptionWindow:    5 * time.Minute,
		SnapshotThreshold:   100,
		EnableSessionResume: true,
		EnableCompression:   false,
		EnableMetrics:       true,
	}
}

// ReconnectAttempt tracks individual reconnection attempts
type ReconnectAttempt struct {
	SessionID         string
	UserID            string
	Attempt           int
	LastAttempt       time.Time
	NextRetry         time.Time
	CurrentBackoff    time.Duration
	State             string // "backing_off", "reconnecting", "failed", "succeeded"
	LastError         error
	ConsecutiveErrors int
}

// NewReconnectionManager creates an advanced reconnection manager
func NewReconnectionManager(redis *redis.Client, config ReconnectionConfig, logger *zap.Logger) *ReconnectionManager {
	return &ReconnectionManager{
		redis:             redis,
		logger:            logger,
		config:            config,
		sessions:          make(map[string]*SessionState),
		reconnectAttempts: make(map[string]*ReconnectAttempt),
	}
}

// SetSnapshotProvider sets the function to provide snapshots for full resync
func (rm *ReconnectionManager) SetSnapshotProvider(provider func(userID string) (json.RawMessage, error)) {
	rm.snapshotProvider = provider
}

// SetSequenceProvider sets the function to get current sequence number
func (rm *ReconnectionManager) SetSequenceProvider(provider func(userID string) uint64) {
	rm.sequenceProvider = provider
}

// SetSubscriptionLoader sets the function to load user subscriptions
func (rm *ReconnectionManager) SetSubscriptionLoader(loader func(userID string) []string) {
	rm.subscriptionLoader = loader
}

// CreateSession creates a new session for a connected client
func (rm *ReconnectionManager) CreateSession(ctx context.Context, userID string, metadata map[string]interface{}) (*SessionState, error) {
	sessionID := uuid.New().String()

	// Get initial sequence number
	var lastSeq uint64
	if rm.sequenceProvider != nil {
		lastSeq = rm.sequenceProvider(userID)
	}

	// Load subscriptions
	var subscriptions []string
	if rm.subscriptionLoader != nil {
		subscriptions = rm.subscriptionLoader(userID)
	}

	session := &SessionState{
		SessionID:      sessionID,
		UserID:         userID,
		LastAckSeq:     lastSeq,
		Subscriptions:  subscriptions,
		ConnectionTime: time.Now(),
		LastActivity:   time.Now(),
		Metadata:       metadata,
	}

	// Store in memory
	rm.sessionsMu.Lock()
	rm.sessions[sessionID] = session
	rm.sessionsMu.Unlock()

	// Persist to Redis with TTL
	if err := rm.persistSession(ctx, session); err != nil {
		rm.logger.Warn("Failed to persist session",
			zap.Error(err),
			zap.String("session_id", sessionID),
		)
	}

	rm.logger.Info("Session created",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
		zap.Uint64("initial_seq", lastSeq),
	)

	return session, nil
}

// AttemptResume attempts to resume a session after reconnection
func (rm *ReconnectionManager) AttemptResume(ctx context.Context, sessionID string, lastAckSeq uint64) (*SessionState, bool, error) {
	// Track the attempt
	atomic.AddUint64(&rm.totalReconnects, 1)

	// Load session from cache or Redis
	session, err := rm.loadSession(ctx, sessionID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load session: %w", err)
	}

	if session == nil {
		rm.logger.Info("Session not found, requiring full resync",
			zap.String("session_id", sessionID),
		)
		atomic.AddUint64(&rm.fullResyncs, 1)
		return nil, false, nil
	}

	// Check if session is within resumption window
	if time.Since(session.LastActivity) > rm.config.ResumptionWindow {
		rm.logger.Info("Session expired, requiring full resync",
			zap.String("session_id", sessionID),
			zap.Duration("idle_time", time.Since(session.LastActivity)),
		)
		atomic.AddUint64(&rm.fullResyncs, 1)
		return nil, false, nil
	}

	// Check sequence gap
	currentSeq := uint64(0)
	if rm.sequenceProvider != nil {
		currentSeq = rm.sequenceProvider(session.UserID)
	}

	gap := currentSeq - lastAckSeq
	if gap > uint64(rm.config.SnapshotThreshold) {
		rm.logger.Info("Sequence gap too large, requiring full resync",
			zap.String("session_id", sessionID),
			zap.Uint64("gap", gap),
			zap.Uint64("threshold", uint64(rm.config.SnapshotThreshold)),
		)
		atomic.AddUint64(&rm.fullResyncs, 1)
		return nil, false, nil
	}

	// Session can be resumed
	session.LastActivity = time.Now()
	rm.persistSession(ctx, session)

	atomic.AddUint64(&rm.successfulResumes, 1)

	rm.logger.Info("Session resumed successfully",
		zap.String("session_id", sessionID),
		zap.String("user_id", session.UserID),
		zap.Uint64("sequence_gap", gap),
	)

	return session, true, nil
}

// HandleDisconnect handles client disconnection
func (rm *ReconnectionManager) HandleDisconnect(ctx context.Context, sessionID string) error {
	rm.sessionsMu.RLock()
	session, exists := rm.sessions[sessionID]
	rm.sessionsMu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update last activity
	session.LastActivity = time.Now()

	// Persist updated session
	if err := rm.persistSession(ctx, session); err != nil {
		return fmt.Errorf("failed to persist session: %w", err)
	}

	// Create reconnect attempt tracker
	rm.attemptsMu.Lock()
	rm.reconnectAttempts[sessionID] = &ReconnectAttempt{
		SessionID:      sessionID,
		UserID:         session.UserID,
		Attempt:        0,
		LastAttempt:    time.Now(),
		NextRetry:      time.Now().Add(rm.config.BaseInterval),
		CurrentBackoff: rm.config.BaseInterval,
		State:          "backing_off",
	}
	rm.attemptsMu.Unlock()

	rm.logger.Info("Client disconnected, session preserved",
		zap.String("session_id", sessionID),
		zap.String("user_id", session.UserID),
	)

	return nil
}

// GetReconnectStrategy returns the reconnection strategy for a session
func (rm *ReconnectionManager) GetReconnectStrategy(sessionID string) map[string]interface{} {
	rm.attemptsMu.RLock()
	attempt, exists := rm.reconnectAttempts[sessionID]
	rm.attemptsMu.RUnlock()

	if !exists {
		// First reconnection attempt
		return map[string]interface{}{
			"strategy":        "exponential_backoff_with_jitter",
			"initial_delay":   rm.config.BaseInterval.Milliseconds(),
			"max_delay":       rm.config.MaxInterval.Milliseconds(),
			"multiplier":      rm.config.Multiplier,
			"jitter_factor":   rm.config.JitterFactor,
			"max_retries":     rm.config.MaxRetries,
			"can_resume":      rm.config.EnableSessionResume,
		}
	}

	// Calculate next backoff with jitter
	nextBackoff := rm.calculateBackoffWithJitter(attempt)
	attempt.NextRetry = time.Now().Add(nextBackoff)
	attempt.CurrentBackoff = nextBackoff

	return map[string]interface{}{
		"strategy":        "exponential_backoff_with_jitter",
		"attempt":         attempt.Attempt + 1,
		"next_delay":      nextBackoff.Milliseconds(),
		"max_delay":       rm.config.MaxInterval.Milliseconds(),
		"multiplier":      rm.config.Multiplier,
		"jitter_factor":   rm.config.JitterFactor,
		"remaining_tries": rm.config.MaxRetries - attempt.Attempt,
		"can_resume":      rm.config.EnableSessionResume && attempt.Attempt < rm.config.MaxRetries,
		"state":           attempt.State,
	}
}

// calculateBackoffWithJitter calculates exponential backoff with jitter
func (rm *ReconnectionManager) calculateBackoffWithJitter(attempt *ReconnectAttempt) time.Duration {
	// Calculate base exponential backoff
	backoff := float64(rm.config.BaseInterval) * math.Pow(rm.config.Multiplier, float64(attempt.Attempt))

	// Cap at max interval
	if backoff > float64(rm.config.MaxInterval) {
		backoff = float64(rm.config.MaxInterval)
	}

	// Apply jitter
	if rm.config.JitterFactor > 0 {
		// Generate cryptographically secure random jitter
		jitterRange := backoff * rm.config.JitterFactor
		minBackoff := backoff - jitterRange
		maxBackoff := backoff + jitterRange

		// Generate random value in range [0, 1)
		randomBig, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			// Fallback to middle value if random generation fails
			backoff = (minBackoff + maxBackoff) / 2
		} else {
			randomFloat := float64(randomBig.Int64()) / 1000000.0
			backoff = minBackoff + (maxBackoff-minBackoff)*randomFloat
		}
	}

	return time.Duration(backoff)
}

// PerformFullResync performs a full resynchronization
func (rm *ReconnectionManager) PerformFullResync(ctx context.Context, userID string) (*SessionState, error) {
	// Get fresh snapshot
	var snapshot json.RawMessage
	var err error

	if rm.snapshotProvider != nil {
		snapshot, err = rm.snapshotProvider(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get snapshot: %w", err)
		}
	}

	// Create new session with snapshot
	session, err := rm.CreateSession(ctx, userID, map[string]interface{}{
		"resync_reason": "full_resync",
		"timestamp":     time.Now().Unix(),
	})
	if err != nil {
		return nil, err
	}

	session.Snapshot = snapshot

	rm.logger.Info("Full resync performed",
		zap.String("session_id", session.SessionID),
		zap.String("user_id", userID),
		zap.Int("snapshot_size", len(snapshot)),
	)

	return session, nil
}

// UpdateSessionActivity updates the last activity timestamp
func (rm *ReconnectionManager) UpdateSessionActivity(sessionID string, lastAckSeq uint64, lastMessageID string) {
	rm.sessionsMu.RLock()
	session, exists := rm.sessions[sessionID]
	rm.sessionsMu.RUnlock()

	if !exists {
		return
	}

	session.LastActivity = time.Now()
	session.LastAckSeq = lastAckSeq
	if lastMessageID != "" {
		session.LastMessageID = lastMessageID
	}

	// Persist asynchronously
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rm.persistSession(ctx, session)
	}()
}

// persistSession persists session state to Redis
func (rm *ReconnectionManager) persistSession(ctx context.Context, session *SessionState) error {
	key := fmt.Sprintf("session:%s", session.SessionID)

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	return rm.redis.Set(ctx, key, data, rm.config.SessionTTL).Err()
}

// loadSession loads session from cache or Redis
func (rm *ReconnectionManager) loadSession(ctx context.Context, sessionID string) (*SessionState, error) {
	// Check memory cache first
	rm.sessionsMu.RLock()
	if session, exists := rm.sessions[sessionID]; exists {
		rm.sessionsMu.RUnlock()
		return session, nil
	}
	rm.sessionsMu.RUnlock()

	// Load from Redis
	key := fmt.Sprintf("session:%s", sessionID)
	data, err := rm.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var session SessionState
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Cache in memory
	rm.sessionsMu.Lock()
	rm.sessions[sessionID] = &session
	rm.sessionsMu.Unlock()

	return &session, nil
}

// CleanupInactiveSessions removes inactive sessions
func (rm *ReconnectionManager) CleanupInactiveSessions(ctx context.Context) {
	rm.sessionsMu.Lock()
	defer rm.sessionsMu.Unlock()

	now := time.Now()
	toDelete := []string{}

	for sessionID, session := range rm.sessions {
		if now.Sub(session.LastActivity) > rm.config.SessionTTL {
			toDelete = append(toDelete, sessionID)
		}
	}

	for _, sessionID := range toDelete {
		delete(rm.sessions, sessionID)
		// Also remove from Redis
		key := fmt.Sprintf("session:%s", sessionID)
		rm.redis.Del(ctx, key)

		rm.logger.Debug("Cleaned up inactive session",
			zap.String("session_id", sessionID),
		)
	}
}

// GetMetrics returns reconnection metrics
func (rm *ReconnectionManager) GetMetrics() map[string]interface{} {
	rm.sessionsMu.RLock()
	activeSessions := len(rm.sessions)
	rm.sessionsMu.RUnlock()

	rm.attemptsMu.RLock()
	activeAttempts := len(rm.reconnectAttempts)
	rm.attemptsMu.RUnlock()

	totalReconnects := atomic.LoadUint64(&rm.totalReconnects)
	successfulResumes := atomic.LoadUint64(&rm.successfulResumes)
	fullResyncs := atomic.LoadUint64(&rm.fullResyncs)

	resumeRate := float64(0)
	if totalReconnects > 0 {
		resumeRate = float64(successfulResumes) / float64(totalReconnects) * 100.0
	}

	return map[string]interface{}{
		"active_sessions":     activeSessions,
		"active_attempts":     activeAttempts,
		"total_reconnects":    totalReconnects,
		"successful_resumes":  successfulResumes,
		"full_resyncs":        fullResyncs,
		"resume_success_rate": resumeRate,
		"config": map[string]interface{}{
			"base_interval":      rm.config.BaseInterval.Seconds(),
			"max_interval":       rm.config.MaxInterval.Seconds(),
			"multiplier":         rm.config.Multiplier,
			"jitter_factor":      rm.config.JitterFactor,
			"session_ttl":        rm.config.SessionTTL.Hours(),
			"resumption_window":  rm.config.ResumptionWindow.Minutes(),
			"snapshot_threshold": rm.config.SnapshotThreshold,
		},
	}
}

// MonitorReconnectionHealth monitors and logs reconnection health
func (rm *ReconnectionManager) MonitorReconnectionHealth(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := rm.GetMetrics()
			rm.logger.Info("Reconnection health",
				zap.Any("metrics", metrics),
			)

			// Cleanup inactive sessions
			rm.CleanupInactiveSessions(ctx)

			// Check for stuck attempts
			rm.cleanupStuckAttempts()
		}
	}
}

// cleanupStuckAttempts removes stuck reconnection attempts
func (rm *ReconnectionManager) cleanupStuckAttempts() {
	rm.attemptsMu.Lock()
	defer rm.attemptsMu.Unlock()

	now := time.Now()
	toDelete := []string{}

	for sessionID, attempt := range rm.reconnectAttempts {
		// Remove attempts that haven't been updated in a while
		if now.Sub(attempt.LastAttempt) > rm.config.SessionTTL {
			toDelete = append(toDelete, sessionID)
		}
		// Remove attempts that exceeded max retries
		if attempt.Attempt >= rm.config.MaxRetries {
			toDelete = append(toDelete, sessionID)
			attempt.State = "failed"
		}
	}

	for _, sessionID := range toDelete {
		delete(rm.reconnectAttempts, sessionID)
		rm.logger.Debug("Cleaned up stuck reconnection attempt",
			zap.String("session_id", sessionID),
		)
	}
}
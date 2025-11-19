package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// SessionInfo represents session information
type SessionInfo struct {
	SessionID    string    `json:"session_id"`
	UserID       uuid.UUID `json:"user_id"`
	AccessJTI    string    `json:"access_jti"`
	RefreshJTI   string    `json:"refresh_jti"`
	DeviceID     string    `json:"device_id,omitempty"`
	IPAddress    string    `json:"ip_address,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SessionStore manages active sessions using Redis
type SessionStore struct {
	redis  *redis.Client
	logger *zap.Logger
}

// NewSessionStore creates a new session store
func NewSessionStore(redis *redis.Client, logger *zap.Logger) *SessionStore {
	return &SessionStore{
		redis:  redis,
		logger: logger,
	}
}

// CreateSession creates a new session in the store
func (ss *SessionStore) CreateSession(ctx context.Context, session SessionInfo) error {
	// Validate required fields
	if session.SessionID == "" {
		session.SessionID = uuid.New().String()
	}
	if session.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required")
	}
	if session.AccessJTI == "" {
		return fmt.Errorf("access_jti is required")
	}
	if session.RefreshJTI == "" {
		return fmt.Errorf("refresh_jti is required")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.LastActivity.IsZero() {
		session.LastActivity = time.Now()
	}

	// Calculate TTL from expiration time
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session already expired")
	}

	// Store session details as a hash
	detailKey := sessionDetailPrefix + session.UserID.String() + ":" + session.SessionID
	sessionData := map[string]interface{}{
		"session_id":    session.SessionID,
		"user_id":       session.UserID.String(),
		"access_jti":    session.AccessJTI,
		"refresh_jti":   session.RefreshJTI,
		"device_id":     session.DeviceID,
		"ip_address":    session.IPAddress,
		"user_agent":    session.UserAgent,
		"created_at":    session.CreatedAt.Unix(),
		"last_activity": session.LastActivity.Unix(),
		"expires_at":    session.ExpiresAt.Unix(),
	}

	pipe := ss.redis.Pipeline()

	// Store session details
	pipe.HSet(ctx, detailKey, sessionData)
	pipe.Expire(ctx, detailKey, ttl)

	// Add session ID to user's sessions set
	sessionsKey := userSessionsSetPrefix + session.UserID.String()
	pipe.SAdd(ctx, sessionsKey, session.SessionID)
	pipe.Expire(ctx, sessionsKey, ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		ss.logger.Error("Failed to create session",
			zap.String("user_id", session.UserID.String()),
			zap.String("session_id", session.SessionID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create session: %w", err)
	}

	ss.logger.Info("Session created",
		zap.String("user_id", session.UserID.String()),
		zap.String("session_id", session.SessionID),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// GetSession retrieves session information
func (ss *SessionStore) GetSession(ctx context.Context, userID uuid.UUID, sessionID string) (*SessionInfo, error) {
	detailKey := sessionDetailPrefix + userID.String() + ":" + sessionID

	sessionData, err := ss.redis.HGetAll(ctx, detailKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if len(sessionData) == 0 {
		return nil, fmt.Errorf("session not found")
	}

	session := &SessionInfo{
		SessionID:  sessionData["session_id"],
		AccessJTI:  sessionData["access_jti"],
		RefreshJTI: sessionData["refresh_jti"],
		DeviceID:   sessionData["device_id"],
		IPAddress:  sessionData["ip_address"],
		UserAgent:  sessionData["user_agent"],
	}

	// Parse UUID
	if uid, err := uuid.Parse(sessionData["user_id"]); err == nil {
		session.UserID = uid
	}

	// Parse timestamps
	if createdAt, ok := sessionData["created_at"]; ok {
		var timestamp int64
		fmt.Sscanf(createdAt, "%d", &timestamp)
		session.CreatedAt = time.Unix(timestamp, 0)
	}

	if lastActivity, ok := sessionData["last_activity"]; ok {
		var timestamp int64
		fmt.Sscanf(lastActivity, "%d", &timestamp)
		session.LastActivity = time.Unix(timestamp, 0)
	}

	if expiresAt, ok := sessionData["expires_at"]; ok {
		var timestamp int64
		fmt.Sscanf(expiresAt, "%d", &timestamp)
		session.ExpiresAt = time.Unix(timestamp, 0)
	}

	return session, nil
}

// GetUserSessions retrieves all active sessions for a user
func (ss *SessionStore) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]SessionInfo, error) {
	sessionsKey := userSessionsSetPrefix + userID.String()

	sessionIDs, err := ss.redis.SMembers(ctx, sessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	sessions := make([]SessionInfo, 0, len(sessionIDs))

	for _, sessionID := range sessionIDs {
		session, err := ss.GetSession(ctx, userID, sessionID)
		if err != nil {
			ss.logger.Warn("Failed to get session details",
				zap.String("user_id", userID.String()),
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
			continue
		}
		sessions = append(sessions, *session)
	}

	return sessions, nil
}

// UpdateSessionActivity updates the last activity timestamp
func (ss *SessionStore) UpdateSessionActivity(ctx context.Context, userID uuid.UUID, sessionID string) error {
	detailKey := sessionDetailPrefix + userID.String() + ":" + sessionID

	// Check if session exists
	exists, err := ss.redis.Exists(ctx, detailKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check session: %w", err)
	}

	if exists == 0 {
		return fmt.Errorf("session not found")
	}

	// Update last activity
	err = ss.redis.HSet(ctx, detailKey, "last_activity", time.Now().Unix()).Err()
	if err != nil {
		ss.logger.Error("Failed to update session activity",
			zap.String("user_id", userID.String()),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to update session activity: %w", err)
	}

	return nil
}

// DeleteSession deletes a specific session
func (ss *SessionStore) DeleteSession(ctx context.Context, userID uuid.UUID, sessionID string) error {
	detailKey := sessionDetailPrefix + userID.String() + ":" + sessionID
	sessionsKey := userSessionsSetPrefix + userID.String()

	pipe := ss.redis.Pipeline()
	pipe.Del(ctx, detailKey)
	pipe.SRem(ctx, sessionsKey, sessionID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		ss.logger.Error("Failed to delete session",
			zap.String("user_id", userID.String()),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete session: %w", err)
	}

	ss.logger.Info("Session deleted",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID),
	)

	return nil
}

// DeleteUserSessions deletes all sessions for a user
func (ss *SessionStore) DeleteUserSessions(ctx context.Context, userID uuid.UUID) error {
	sessionsKey := userSessionsSetPrefix + userID.String()

	sessionIDs, err := ss.redis.SMembers(ctx, sessionsKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	pipe := ss.redis.Pipeline()

	// Delete all session details
	for _, sessionID := range sessionIDs {
		detailKey := sessionDetailPrefix + userID.String() + ":" + sessionID
		pipe.Del(ctx, detailKey)
	}

	// Delete the sessions set
	pipe.Del(ctx, sessionsKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		ss.logger.Error("Failed to delete user sessions",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	ss.logger.Info("All user sessions deleted",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(sessionIDs)),
	)

	return nil
}

// GetActiveSessionCount returns the number of active sessions for a user
func (ss *SessionStore) GetActiveSessionCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	sessionsKey := userSessionsSetPrefix + userID.String()

	count, err := ss.redis.SCard(ctx, sessionsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to count sessions: %w", err)
	}

	return count, nil
}

// EnforceSessionLimit enforces a maximum number of concurrent sessions per user
// Deletes oldest sessions if limit is exceeded
func (ss *SessionStore) EnforceSessionLimit(ctx context.Context, userID uuid.UUID, maxSessions int) error {
	if maxSessions <= 0 {
		return nil // No limit
	}

	sessions, err := ss.GetUserSessions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(sessions) <= maxSessions {
		return nil // Within limit
	}

	// Sort sessions by creation time (oldest first)
	// Using a simple bubble sort for small arrays
	for i := 0; i < len(sessions)-1; i++ {
		for j := 0; j < len(sessions)-i-1; j++ {
			if sessions[j].CreatedAt.After(sessions[j+1].CreatedAt) {
				sessions[j], sessions[j+1] = sessions[j+1], sessions[j]
			}
		}
	}

	// Delete oldest sessions
	toDelete := len(sessions) - maxSessions
	for i := 0; i < toDelete; i++ {
		err := ss.DeleteSession(ctx, userID, sessions[i].SessionID)
		if err != nil {
			ss.logger.Warn("Failed to delete old session",
				zap.String("user_id", userID.String()),
				zap.String("session_id", sessions[i].SessionID),
				zap.Error(err),
			)
		}
	}

	ss.logger.Info("Enforced session limit",
		zap.String("user_id", userID.String()),
		zap.Int("deleted", toDelete),
		zap.Int("max_sessions", maxSessions),
	)

	return nil
}

// CleanupExpiredSessions removes expired sessions
// Redis automatically expires keys, so this is mainly for cleanup of orphaned entries
func (ss *SessionStore) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	var cleaned int64
	var cursor uint64

	for {
		// Scan for session detail keys
		keys, nextCursor, err := ss.redis.Scan(ctx, cursor, sessionDetailPrefix+"*", 100).Result()
		if err != nil {
			return cleaned, fmt.Errorf("failed to scan sessions: %w", err)
		}

		for _, key := range keys {
			// Check if key exists and get expiration
			ttl, err := ss.redis.TTL(ctx, key).Result()
			if err != nil || ttl < 0 {
				// Key expired or has no TTL, delete it
				ss.redis.Del(ctx, key)
				cleaned++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	ss.logger.Info("Cleaned up expired sessions", zap.Int64("count", cleaned))
	return cleaned, nil
}

// GetSessionStats returns statistics about active sessions
func (ss *SessionStore) GetSessionStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count total sessions
	var totalSessions int64
	cursor := uint64(0)
	for {
		keys, nextCursor, err := ss.redis.Scan(ctx, cursor, sessionDetailPrefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan sessions: %w", err)
		}
		totalSessions += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	stats["total_sessions"] = totalSessions

	// Count users with active sessions
	var activeUsers int64
	cursor = 0
	for {
		keys, nextCursor, err := ss.redis.Scan(ctx, cursor, userSessionsSetPrefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan user sessions: %w", err)
		}
		activeUsers += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	stats["active_users"] = activeUsers

	return stats, nil
}

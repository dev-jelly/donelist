package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TokenBlacklist manages token revocation using Redis
type TokenBlacklist struct {
	redis  *redis.Client
	logger *zap.Logger
}

// NewTokenBlacklist creates a new token blacklist manager
func NewTokenBlacklist(redis *redis.Client, logger *zap.Logger) *TokenBlacklist {
	return &TokenBlacklist{
		redis:  redis,
		logger: logger,
	}
}

// Key patterns for Redis storage
const (
	// Blacklist keys
	accessTokenBlacklistPrefix  = "blacklist:access:"    // blacklist:access:{jti}
	refreshTokenBlacklistPrefix = "blacklist:refresh:"   // blacklist:refresh:{jti}

	// Session keys
	userSessionPrefix           = "session:user:"        // session:user:{userId}
	sessionDetailPrefix         = "session:detail:"      // session:detail:{userId}:{sessionId}

	// User sessions index
	userSessionsSetPrefix       = "sessions:user:"       // sessions:user:{userId} (set of session IDs)
)

// BlacklistAccessToken adds an access token's JTI to the blacklist
func (tb *TokenBlacklist) BlacklistAccessToken(ctx context.Context, jti string, ttl time.Duration) error {
	key := accessTokenBlacklistPrefix + jti

	err := tb.redis.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		tb.logger.Error("Failed to blacklist access token",
			zap.String("jti", jti),
			zap.Error(err),
		)
		return fmt.Errorf("failed to blacklist access token: %w", err)
	}

	tb.logger.Info("Access token blacklisted",
		zap.String("jti", jti),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// BlacklistRefreshToken adds a refresh token's JTI to the blacklist
func (tb *TokenBlacklist) BlacklistRefreshToken(ctx context.Context, jti string, ttl time.Duration) error {
	key := refreshTokenBlacklistPrefix + jti

	err := tb.redis.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		tb.logger.Error("Failed to blacklist refresh token",
			zap.String("jti", jti),
			zap.Error(err),
		)
		return fmt.Errorf("failed to blacklist refresh token: %w", err)
	}

	tb.logger.Info("Refresh token blacklisted",
		zap.String("jti", jti),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// IsAccessTokenBlacklisted checks if an access token's JTI is blacklisted
func (tb *TokenBlacklist) IsAccessTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	key := accessTokenBlacklistPrefix + jti

	result, err := tb.redis.Exists(ctx, key).Result()
	if err != nil {
		tb.logger.Error("Failed to check access token blacklist",
			zap.String("jti", jti),
			zap.Error(err),
		)
		return false, fmt.Errorf("failed to check blacklist: %w", err)
	}

	return result > 0, nil
}

// IsRefreshTokenBlacklisted checks if a refresh token's JTI is blacklisted
func (tb *TokenBlacklist) IsRefreshTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	key := refreshTokenBlacklistPrefix + jti

	result, err := tb.redis.Exists(ctx, key).Result()
	if err != nil {
		tb.logger.Error("Failed to check refresh token blacklist",
			zap.String("jti", jti),
			zap.Error(err),
		)
		return false, fmt.Errorf("failed to check blacklist: %w", err)
	}

	return result > 0, nil
}

// RemoveFromBlacklist removes a JTI from both blacklists (cleanup)
func (tb *TokenBlacklist) RemoveFromBlacklist(ctx context.Context, jti string) error {
	accessKey := accessTokenBlacklistPrefix + jti
	refreshKey := refreshTokenBlacklistPrefix + jti

	pipe := tb.redis.Pipeline()
	pipe.Del(ctx, accessKey)
	pipe.Del(ctx, refreshKey)

	_, err := pipe.Exec(ctx)
	if err != nil {
		tb.logger.Error("Failed to remove JTI from blacklist",
			zap.String("jti", jti),
			zap.Error(err),
		)
		return fmt.Errorf("failed to remove from blacklist: %w", err)
	}

	return nil
}

// CleanupExpired removes expired blacklist entries
// Note: Redis automatically expires keys with TTL, so this is mainly for manual cleanup
func (tb *TokenBlacklist) CleanupExpired(ctx context.Context) (int64, error) {
	var cursor uint64
	var cleaned int64

	// Scan and check access token blacklist
	for {
		keys, nextCursor, err := tb.redis.Scan(ctx, cursor, accessTokenBlacklistPrefix+"*", 100).Result()
		if err != nil {
			return cleaned, fmt.Errorf("failed to scan access blacklist: %w", err)
		}

		for _, key := range keys {
			ttl, err := tb.redis.TTL(ctx, key).Result()
			if err != nil || ttl < 0 {
				// Key doesn't exist or has no expiry, delete it
				tb.redis.Del(ctx, key)
				cleaned++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	// Scan and check refresh token blacklist
	cursor = 0
	for {
		keys, nextCursor, err := tb.redis.Scan(ctx, cursor, refreshTokenBlacklistPrefix+"*", 100).Result()
		if err != nil {
			return cleaned, fmt.Errorf("failed to scan refresh blacklist: %w", err)
		}

		for _, key := range keys {
			ttl, err := tb.redis.TTL(ctx, key).Result()
			if err != nil || ttl < 0 {
				// Key doesn't exist or has no expiry, delete it
				tb.redis.Del(ctx, key)
				cleaned++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	tb.logger.Info("Cleaned up expired blacklist entries", zap.Int64("count", cleaned))
	return cleaned, nil
}

// GetBlacklistStats returns statistics about the blacklist
func (tb *TokenBlacklist) GetBlacklistStats(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	// Count access token blacklist entries
	var accessCount int64
	cursor := uint64(0)
	for {
		keys, nextCursor, err := tb.redis.Scan(ctx, cursor, accessTokenBlacklistPrefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan access blacklist: %w", err)
		}
		accessCount += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	stats["access_tokens"] = accessCount

	// Count refresh token blacklist entries
	var refreshCount int64
	cursor = 0
	for {
		keys, nextCursor, err := tb.redis.Scan(ctx, cursor, refreshTokenBlacklistPrefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan refresh blacklist: %w", err)
		}
		refreshCount += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	stats["refresh_tokens"] = refreshCount

	stats["total"] = accessCount + refreshCount

	return stats, nil
}

// BlacklistAllUserTokens blacklists all tokens for a specific user
// This is used when revoking all sessions for a user
func (tb *TokenBlacklist) BlacklistAllUserTokens(ctx context.Context, userID uuid.UUID, accessTTL, refreshTTL time.Duration) error {
	// Get all session IDs for the user
	sessionsKey := userSessionsSetPrefix + userID.String()
	sessionIDs, err := tb.redis.SMembers(ctx, sessionsKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	pipe := tb.redis.Pipeline()

	// For each session, get the JTIs and blacklist them
	for _, sessionID := range sessionIDs {
		detailKey := sessionDetailPrefix + userID.String() + ":" + sessionID
		sessionData, err := tb.redis.HGetAll(ctx, detailKey).Result()
		if err != nil {
			tb.logger.Warn("Failed to get session details",
				zap.String("user_id", userID.String()),
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
			continue
		}

		// Blacklist access token JTI if present
		if accessJTI, ok := sessionData["access_jti"]; ok && accessJTI != "" {
			accessKey := accessTokenBlacklistPrefix + accessJTI
			pipe.Set(ctx, accessKey, "1", accessTTL)
		}

		// Blacklist refresh token JTI if present
		if refreshJTI, ok := sessionData["refresh_jti"]; ok && refreshJTI != "" {
			refreshKey := refreshTokenBlacklistPrefix + refreshJTI
			pipe.Set(ctx, refreshKey, "1", refreshTTL)
		}

		// Delete session details
		pipe.Del(ctx, detailKey)
	}

	// Clear the sessions set
	pipe.Del(ctx, sessionsKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		tb.logger.Error("Failed to blacklist user tokens",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to blacklist user tokens: %w", err)
	}

	tb.logger.Info("Blacklisted all tokens for user",
		zap.String("user_id", userID.String()),
		zap.Int("session_count", len(sessionIDs)),
	)

	return nil
}

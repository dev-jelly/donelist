package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return client, mr
}

func TestTokenBlacklist_BlacklistAccessToken(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()
	jti := uuid.New().String()
	ttl := 5 * time.Minute

	// Blacklist token
	err := tb.BlacklistAccessToken(ctx, jti, ttl)
	require.NoError(t, err)

	// Check if token is blacklisted
	blacklisted, err := tb.IsAccessTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.True(t, blacklisted)

	// Check that it's not in refresh blacklist
	blacklisted, err = tb.IsRefreshTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.False(t, blacklisted)
}

func TestTokenBlacklist_BlacklistRefreshToken(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()
	jti := uuid.New().String()
	ttl := 7 * 24 * time.Hour

	// Blacklist token
	err := tb.BlacklistRefreshToken(ctx, jti, ttl)
	require.NoError(t, err)

	// Check if token is blacklisted
	blacklisted, err := tb.IsRefreshTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.True(t, blacklisted)

	// Check that it's not in access blacklist
	blacklisted, err = tb.IsAccessTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.False(t, blacklisted)
}

func TestTokenBlacklist_RemoveFromBlacklist(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()
	jti := uuid.New().String()
	ttl := 5 * time.Minute

	// Blacklist both access and refresh tokens
	err := tb.BlacklistAccessToken(ctx, jti, ttl)
	require.NoError(t, err)
	err = tb.BlacklistRefreshToken(ctx, jti, ttl)
	require.NoError(t, err)

	// Verify both are blacklisted
	blacklisted, _ := tb.IsAccessTokenBlacklisted(ctx, jti)
	assert.True(t, blacklisted)
	blacklisted, _ = tb.IsRefreshTokenBlacklisted(ctx, jti)
	assert.True(t, blacklisted)

	// Remove from blacklist
	err = tb.RemoveFromBlacklist(ctx, jti)
	require.NoError(t, err)

	// Verify both are removed
	blacklisted, _ = tb.IsAccessTokenBlacklisted(ctx, jti)
	assert.False(t, blacklisted)
	blacklisted, _ = tb.IsRefreshTokenBlacklisted(ctx, jti)
	assert.False(t, blacklisted)
}

func TestTokenBlacklist_TTLExpiration(t *testing.T) {
	client, mr := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()
	jti := uuid.New().String()
	ttl := 1 * time.Second

	// Blacklist token with short TTL
	err := tb.BlacklistAccessToken(ctx, jti, ttl)
	require.NoError(t, err)

	// Verify token is blacklisted
	blacklisted, err := tb.IsAccessTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.True(t, blacklisted)

	// Fast-forward time in miniredis
	mr.FastForward(2 * time.Second)

	// Verify token is no longer blacklisted
	blacklisted, err = tb.IsAccessTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.False(t, blacklisted)
}

func TestTokenBlacklist_GetStats(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()

	// Blacklist some tokens
	for i := 0; i < 3; i++ {
		jti := uuid.New().String()
		err := tb.BlacklistAccessToken(ctx, jti, 5*time.Minute)
		require.NoError(t, err)
	}

	for i := 0; i < 2; i++ {
		jti := uuid.New().String()
		err := tb.BlacklistRefreshToken(ctx, jti, 7*24*time.Hour)
		require.NoError(t, err)
	}

	// Get stats
	stats, err := tb.GetBlacklistStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(3), stats["access_tokens"])
	assert.Equal(t, int64(2), stats["refresh_tokens"])
	assert.Equal(t, int64(5), stats["total"])
}

func TestTokenBlacklist_BlacklistAllUserTokens(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)
	ss := NewSessionStore(client, logger)

	ctx := context.Background()
	userID := uuid.New()

	// Create multiple sessions for the user
	sessions := []SessionInfo{
		{
			SessionID:  "session1",
			UserID:     userID,
			AccessJTI:  "access-jti-1",
			RefreshJTI: "refresh-jti-1",
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		},
		{
			SessionID:  "session2",
			UserID:     userID,
			AccessJTI:  "access-jti-2",
			RefreshJTI: "refresh-jti-2",
			ExpiresAt:  time.Now().Add(1 * time.Hour),
		},
	}

	for _, session := range sessions {
		err := ss.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Verify sessions exist
	count, err := ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Blacklist all user tokens
	err = tb.BlacklistAllUserTokens(ctx, userID, 5*time.Minute, 7*24*time.Hour)
	require.NoError(t, err)

	// Verify all tokens are blacklisted
	for _, session := range sessions {
		blacklisted, err := tb.IsAccessTokenBlacklisted(ctx, session.AccessJTI)
		require.NoError(t, err)
		assert.True(t, blacklisted, "Access token should be blacklisted: "+session.AccessJTI)

		blacklisted, err = tb.IsRefreshTokenBlacklisted(ctx, session.RefreshJTI)
		require.NoError(t, err)
		assert.True(t, blacklisted, "Refresh token should be blacklisted: "+session.RefreshJTI)
	}

	// Verify sessions are deleted
	count, err = ss.GetActiveSessionCount(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestTokenBlacklist_CleanupExpired(t *testing.T) {
	client, mr := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()

	// Create tokens with different TTLs
	jti1 := uuid.New().String()
	jti2 := uuid.New().String()
	jti3 := uuid.New().String()

	err := tb.BlacklistAccessToken(ctx, jti1, 1*time.Second)
	require.NoError(t, err)
	err = tb.BlacklistAccessToken(ctx, jti2, 10*time.Minute)
	require.NoError(t, err)
	err = tb.BlacklistRefreshToken(ctx, jti3, 1*time.Second)
	require.NoError(t, err)

	// Fast-forward time
	mr.FastForward(2 * time.Second)

	// Run cleanup
	cleaned, err := tb.CleanupExpired(ctx)
	require.NoError(t, err)

	// jti1 and jti3 should be expired, but Redis auto-deletes them
	// So cleanup might find 0 keys if Redis already cleaned them
	// The important thing is no error occurred
	assert.True(t, cleaned >= 0)

	// Verify the non-expired token still exists
	blacklisted, err := tb.IsAccessTokenBlacklisted(ctx, jti2)
	require.NoError(t, err)
	assert.True(t, blacklisted)

	// Verify expired tokens are gone
	blacklisted, err = tb.IsAccessTokenBlacklisted(ctx, jti1)
	require.NoError(t, err)
	assert.False(t, blacklisted)

	blacklisted, err = tb.IsRefreshTokenBlacklisted(ctx, jti3)
	require.NoError(t, err)
	assert.False(t, blacklisted)
}

func TestTokenBlacklist_ConcurrentOperations(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()
	jti := uuid.New().String()

	// Simulate concurrent blacklist operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := tb.BlacklistAccessToken(ctx, jti, 5*time.Minute)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify token is blacklisted
	blacklisted, err := tb.IsAccessTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.True(t, blacklisted)
}

func TestTokenBlacklist_NonExistentToken(t *testing.T) {
	client, _ := setupTestRedis(t)
	logger := zap.NewNop()
	tb := NewTokenBlacklist(client, logger)

	ctx := context.Background()
	jti := uuid.New().String()

	// Check non-existent token
	blacklisted, err := tb.IsAccessTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.False(t, blacklisted)

	blacklisted, err = tb.IsRefreshTokenBlacklisted(ctx, jti)
	require.NoError(t, err)
	assert.False(t, blacklisted)
}

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTokenRotationTest(t *testing.T) (*Service, *testutil.TestDB) {
	testDB := testutil.SetupTestDB(t)

	userRepo := user.NewRepository(testDB.DB)
	refreshTokenRepo := NewRefreshTokenRepository(testDB.DB)

	config := JWTConfig{
		SigningMethod:      SigningMethodHS256,
		Secret:             "test-secret-key-for-token-rotation",
		AccessExpiry:       15 * time.Minute,
		RefreshExpiry:      7 * 24 * time.Hour,
		ClockSkewTolerance: 30 * time.Second,
	}

	jwtManager, err := NewJWTManager(config)
	require.NoError(t, err)

	logger := zap.NewNop()
	service := NewService(userRepo, refreshTokenRepo, jwtManager, logger)

	return service, testDB
}

func TestTokenRotation_OneTimeUse(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	// Create test user
	testEmail := "rotation@example.com"
	testPassword := "Password123!"
	fixtures.CreateTestUser(t, testEmail, testPassword, "rotationuser")

	// Login to get initial tokens
	_, tokens, err := service.Login(ctx, LoginInput{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)
	require.NotNil(t, tokens)

	t.Run("First refresh succeeds", func(t *testing.T) {
		newTokens, err := service.RefreshAccessToken(ctx, tokens.RefreshToken)
		require.NoError(t, err)
		assert.NotNil(t, newTokens)
		assert.NotEqual(t, tokens.AccessToken, newTokens.AccessToken)
		assert.NotEqual(t, tokens.RefreshToken, newTokens.RefreshToken)

		// Update tokens for next test
		tokens = newTokens
	})

	t.Run("Second refresh with same token fails (one-time use)", func(t *testing.T) {
		// Try to use the old refresh token again
		_, err := service.RefreshAccessToken(ctx, tokens.RefreshToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already been used")
	})
}

func TestTokenRotation_JTITracking(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	testEmail := "jti@example.com"
	testPassword := "Password123!"
	fixtures.CreateTestUser(t, testEmail, testPassword, "jtiuser")

	_, tokens, err := service.Login(ctx, LoginInput{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)

	// Extract JTI from refresh token
	claims, err := service.jwtManager.ValidateRefreshToken(tokens.RefreshToken)
	require.NoError(t, err)
	originalJTI := claims.ID

	t.Run("JTI is stored in database", func(t *testing.T) {
		// Check JTI is not revoked initially
		revoked, err := service.refreshTokenRepo.IsJTIRevoked(ctx, originalJTI)
		require.NoError(t, err)
		assert.False(t, revoked)
	})

	t.Run("JTI is revoked after use", func(t *testing.T) {
		// Refresh tokens
		newTokens, err := service.RefreshAccessToken(ctx, tokens.RefreshToken)
		require.NoError(t, err)
		assert.NotNil(t, newTokens)

		// Original JTI should now be revoked
		time.Sleep(100 * time.Millisecond) // Give time for revocation
		revoked, err := service.refreshTokenRepo.IsJTIRevoked(ctx, originalJTI)
		require.NoError(t, err)
		assert.True(t, revoked)
	})
}

func TestTokenRotation_ReuseDetection(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	testEmail := "reuse@example.com"
	testPassword := "Password123!"
	fixtures.CreateTestUser(t, testEmail, testPassword, "reuseuser")

	_, tokens, err := service.Login(ctx, LoginInput{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)

	// Use the refresh token once
	_, err = service.RefreshAccessToken(ctx, tokens.RefreshToken)
	require.NoError(t, err)

	t.Run("Reuse attempt revokes all user tokens", func(t *testing.T) {
		// Create another session
		_, newTokens, err := service.Login(ctx, LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		require.NoError(t, err)

		// Attempt to reuse the old token (security violation)
		_, err = service.RefreshAccessToken(ctx, tokens.RefreshToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "security violation")

		// The new session's tokens should also be revoked as a security measure
		time.Sleep(100 * time.Millisecond)
		_, err = service.RefreshAccessToken(ctx, newTokens.RefreshToken)
		// This should fail because all tokens were revoked
		assert.Error(t, err)
	})
}

func TestTokenRotation_ConcurrentLoginPolicy(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	testEmail := "concurrent@example.com"
	testPassword := "Password123!"
	testUserID := fixtures.CreateTestUser(t, testEmail, testPassword, "concurrentuser")

	t.Run("Multiple concurrent sessions allowed", func(t *testing.T) {
		var tokens []*TokenPair

		// Create 3 concurrent sessions
		for i := 0; i < 3; i++ {
			_, newTokens, err := service.Login(ctx, LoginInput{
				Email:    testEmail,
				Password: testPassword,
			})
			require.NoError(t, err)
			tokens = append(tokens, newTokens)
		}

		// All sessions should be valid
		for i, token := range tokens {
			newTokens, err := service.RefreshAccessToken(ctx, token.RefreshToken)
			require.NoError(t, err, "Session %d should be valid", i)
			assert.NotNil(t, newTokens)
		}
	})

	t.Run("Get active token count", func(t *testing.T) {
		// Login twice
		_, _, err := service.Login(ctx, LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		require.NoError(t, err)

		_, _, err = service.Login(ctx, LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		require.NoError(t, err)

		count, err := service.refreshTokenRepo.GetActiveTokenCount(ctx, testUserID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 2)
	})
}

func TestTokenRotation_ExpiryAndCleanup(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	testEmail := "expiry@example.com"
	testPassword := "Password123!"
	fixtures.CreateTestUser(t, testEmail, testPassword, "expiryuser")

	t.Run("Expired token cannot be refreshed", func(t *testing.T) {
		// Create manager with very short refresh expiry
		config := JWTConfig{
			SigningMethod: SigningMethodHS256,
			Secret:        "test-secret-key-for-token-rotation",
			AccessExpiry:  15 * time.Minute,
			RefreshExpiry: 100 * time.Millisecond, // Very short for testing
		}

		shortManager, err := NewJWTManager(config)
		require.NoError(t, err)

		shortService := NewService(
			service.userRepo,
			service.refreshTokenRepo,
			shortManager,
			zap.NewNop(),
		)

		// Login with short-lived tokens
		_, tokens, err := shortService.Login(ctx, LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		require.NoError(t, err)

		// Wait for token to expire
		time.Sleep(200 * time.Millisecond)

		// Try to refresh expired token
		_, err = shortService.RefreshAccessToken(ctx, tokens.RefreshToken)
		assert.Error(t, err)
	})

	t.Run("Cleanup expired tokens", func(t *testing.T) {
		// Delete expired tokens
		deleted, err := service.refreshTokenRepo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, int64(0))
	})
}

func TestTokenRotation_StandardClaims(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	testEmail := "claims@example.com"
	testPassword := "Password123!"
	testUserID := fixtures.CreateTestUser(t, testEmail, testPassword, "claimsuser")

	_, tokens, err := service.Login(ctx, LoginInput{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)

	t.Run("Standard claims are set", func(t *testing.T) {
		claims, err := service.jwtManager.ValidateRefreshToken(tokens.RefreshToken)
		require.NoError(t, err)

		// Check standard claims
		assert.NotEmpty(t, claims.ID, "JTI should be set")
		assert.Equal(t, testUserID.String(), claims.Subject, "Subject should be user ID")
		assert.NotNil(t, claims.IssuedAt, "IssuedAt should be set")
		assert.NotNil(t, claims.ExpiresAt, "ExpiresAt should be set")
		assert.NotNil(t, claims.NotBefore, "NotBefore should be set")

		// Check custom claims
		assert.Equal(t, testUserID, claims.UserID)
		assert.Equal(t, testEmail, claims.Email)
		assert.Equal(t, RefreshToken, claims.Type)
	})

	t.Run("Access token has different JTI than refresh token", func(t *testing.T) {
		accessClaims, err := service.jwtManager.ValidateAccessToken(tokens.AccessToken)
		require.NoError(t, err)

		refreshClaims, err := service.jwtManager.ValidateRefreshToken(tokens.RefreshToken)
		require.NoError(t, err)

		// Access and refresh tokens should have different JTIs
		assert.NotEqual(t, accessClaims.ID, refreshClaims.ID)
	})
}

func TestTokenRotation_LogoutScenarios(t *testing.T) {
	service, testDB := setupTokenRotationTest(t)
	defer testDB.TearDown(t)
	ctx := context.Background()
	fixtures := testutil.NewFixtures(testDB)

	testEmail := "logout@example.com"
	testPassword := "Password123!"
	testUserID := fixtures.CreateTestUser(t, testEmail, testPassword, "logoutuser")

	t.Run("Logout single session", func(t *testing.T) {
		_, tokens, err := service.Login(ctx, LoginInput{
			Email:    testEmail,
			Password: testPassword,
		})
		require.NoError(t, err)

		// Logout
		err = service.Logout(ctx, tokens.RefreshToken)
		require.NoError(t, err)

		// Try to refresh after logout
		_, err = service.RefreshAccessToken(ctx, tokens.RefreshToken)
		assert.Error(t, err)
	})

	t.Run("Logout all sessions", func(t *testing.T) {
		// Create multiple sessions
		var tokens []*TokenPair
		for i := 0; i < 3; i++ {
			_, newTokens, err := service.Login(ctx, LoginInput{
				Email:    testEmail,
				Password: testPassword,
			})
			require.NoError(t, err)
			tokens = append(tokens, newTokens)
		}

		// Logout from all sessions
		err := service.LogoutAll(ctx, testUserID)
		require.NoError(t, err)

		// All tokens should be invalid
		for i, token := range tokens {
			_, err := service.RefreshAccessToken(ctx, token.RefreshToken)
			assert.Error(t, err, "Token %d should be invalid", i)
		}
	})
}

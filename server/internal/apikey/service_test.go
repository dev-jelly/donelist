package apikey

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dev-jelly/donelist/internal/testutil"
)

func setupServiceTest(t *testing.T) (*Service, *sqlx.DB, func()) {
	testDB := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	repo := NewRepository(testDB.DB)
	service := NewService(repo, logger)

	cleanup := func() {
		testDB.TearDown(t)
	}

	return service, testDB.DB, cleanup
}

func TestService_CreateAPIKey(t *testing.T) {
	service, _, cleanup := setupServiceTest(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("creates API key successfully", func(t *testing.T) {
		input := CreateAPIKeyInput{
			UserID:           userID,
			Name:             "Test API Key",
			Scopes:           []Scope{ScopeCheckinsRead, ScopeCheckinsWrite},
			RateLimitPerDay:  5000,
			RateLimitPerHour: 500,
		}

		result, err := service.CreateAPIKey(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check API key fields
		assert.Equal(t, userID, result.UserID)
		assert.Equal(t, "Test API Key", result.Name)
		assert.Equal(t, 5000, result.RateLimitPerDay)
		assert.Equal(t, 500, result.RateLimitPerHour)
		assert.False(t, result.Revoked)

		// Check plain text key
		assert.NotEmpty(t, result.PlainTextKey)
		assert.True(t, len(result.PlainTextKey) > 20)
	})

	t.Run("validates scopes", func(t *testing.T) {
		input := CreateAPIKeyInput{
			UserID: userID,
			Name:   "Invalid Scopes",
			Scopes: []Scope{"invalid:scope"},
		}

		result, err := service.CreateAPIKey(ctx, input)
		assert.ErrorIs(t, err, ErrInvalidScope)
		assert.Nil(t, result)
	})

	t.Run("sets default rate limits", func(t *testing.T) {
		input := CreateAPIKeyInput{
			UserID: userID,
			Name:   "Default Limits",
			Scopes: []Scope{ScopeCheckinsRead},
		}

		result, err := service.CreateAPIKey(ctx, input)
		require.NoError(t, err)

		assert.Equal(t, 10000, result.RateLimitPerDay)
		assert.Equal(t, 1000, result.RateLimitPerHour)
	})

	t.Run("sets expiry date", func(t *testing.T) {
		expiresAt := time.Now().Add(30 * 24 * time.Hour)
		input := CreateAPIKeyInput{
			UserID:    userID,
			Name:      "Expiring Key",
			Scopes:    []Scope{ScopeCheckinsRead},
			ExpiresAt: &expiresAt,
		}

		result, err := service.CreateAPIKey(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, result.ExpiresAt)

		assert.WithinDuration(t, expiresAt, *result.ExpiresAt, time.Second)
	})
}

func TestService_ValidateAPIKey(t *testing.T) {
	service, _, cleanup := setupServiceTest(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("validates correct key", func(t *testing.T) {
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID: userID,
			Name:   "Valid Key",
			Scopes: []Scope{ScopeCheckinsRead},
		})
		require.NoError(t, err)

		validated, err := service.ValidateAPIKey(ctx, created.PlainTextKey)
		require.NoError(t, err)
		assert.Equal(t, created.ID, validated.ID)
		assert.Equal(t, created.UserID, validated.UserID)
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		validated, err := service.ValidateAPIKey(ctx, "invalid_key")
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)
		assert.Nil(t, validated)
	})

	t.Run("rejects revoked key", func(t *testing.T) {
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID: userID,
			Name:   "To Revoke",
			Scopes: []Scope{ScopeCheckinsRead},
		})
		require.NoError(t, err)

		err = service.RevokeAPIKey(ctx, created.ID, userID, "Testing")
		require.NoError(t, err)

		validated, err := service.ValidateAPIKey(ctx, created.PlainTextKey)
		assert.ErrorIs(t, err, ErrAPIKeyRevoked)
		assert.Nil(t, validated)
	})

	t.Run("rejects expired key", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Hour)
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID:    userID,
			Name:      "Expired Key",
			Scopes:    []Scope{ScopeCheckinsRead},
			ExpiresAt: &past,
		})
		require.NoError(t, err)

		validated, err := service.ValidateAPIKey(ctx, created.PlainTextKey)
		assert.ErrorIs(t, err, ErrAPIKeyExpired)
		assert.Nil(t, validated)
	})
}

func TestService_RateLimit(t *testing.T) {
	service, _, cleanup := setupServiceTest(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("tracks rate limit correctly", func(t *testing.T) {
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID:           userID,
			Name:             "Rate Limited",
			Scopes:           []Scope{ScopeCheckinsRead},
			RateLimitPerDay:  100,
			RateLimitPerHour: 10,
		})
		require.NoError(t, err)

		// Check initial rate limit
		info, err := service.CheckRateLimit(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, info.HourlyUsed)
		assert.Equal(t, 10, info.HourlyLimit)
		assert.Equal(t, 0, info.DailyUsed)
		assert.Equal(t, 100, info.DailyLimit)

		// Record some usage
		for i := 0; i < 5; i++ {
			usage := APIKeyUsage{
				APIKeyID:   created.ID,
				Endpoint:   "/api/v1/checkins",
				Method:     "GET",
				StatusCode: 200,
			}
			err := service.RecordUsage(ctx, usage)
			require.NoError(t, err)
		}

		// Check updated rate limit
		info, err = service.CheckRateLimit(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, 5, info.HourlyUsed)
		assert.Equal(t, 5, info.DailyUsed)
	})

	t.Run("enforces hourly rate limit", func(t *testing.T) {
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID:           userID,
			Name:             "Hourly Limited",
			Scopes:           []Scope{ScopeCheckinsRead},
			RateLimitPerDay:  1000,
			RateLimitPerHour: 3,
		})
		require.NoError(t, err)

		// Use up the hourly limit
		for i := 0; i < 3; i++ {
			usage := APIKeyUsage{
				APIKeyID:   created.ID,
				Endpoint:   "/api/v1/checkins",
				Method:     "GET",
				StatusCode: 200,
			}
			err := service.RecordUsage(ctx, usage)
			require.NoError(t, err)
		}

		// Should now exceed limit
		info, err := service.CheckRateLimit(ctx, created.ID)
		assert.ErrorIs(t, err, ErrRateLimitExceeded)
		assert.NotNil(t, info)
		assert.Equal(t, 3, info.HourlyUsed)
	})

	t.Run("enforces daily rate limit", func(t *testing.T) {
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID:           userID,
			Name:             "Daily Limited",
			Scopes:           []Scope{ScopeCheckinsRead},
			RateLimitPerDay:  2,
			RateLimitPerHour: 100,
		})
		require.NoError(t, err)

		// Use up the daily limit
		for i := 0; i < 2; i++ {
			usage := APIKeyUsage{
				APIKeyID:   created.ID,
				Endpoint:   "/api/v1/checkins",
				Method:     "GET",
				StatusCode: 200,
			}
			err := service.RecordUsage(ctx, usage)
			require.NoError(t, err)
		}

		// Should now exceed limit
		info, err := service.CheckRateLimit(ctx, created.ID)
		assert.ErrorIs(t, err, ErrRateLimitExceeded)
		assert.NotNil(t, info)
		assert.Equal(t, 2, info.DailyUsed)
	})
}

func TestService_ListAPIKeys(t *testing.T) {
	service, _, cleanup := setupServiceTest(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("lists all keys", func(t *testing.T) {
		// Create multiple keys
		for i := 0; i < 3; i++ {
			_, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
				UserID: userID,
				Name:   "Key " + string(rune(i)),
				Scopes: []Scope{ScopeCheckinsRead},
			})
			require.NoError(t, err)
		}

		keys, err := service.ListAPIKeys(ctx, userID, false)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(keys), 3)
	})

	t.Run("excludes revoked keys by default", func(t *testing.T) {
		// Create and revoke a key
		created, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID: userID,
			Name:   "To Revoke",
			Scopes: []Scope{ScopeCheckinsRead},
		})
		require.NoError(t, err)

		err = service.RevokeAPIKey(ctx, created.ID, userID, "Testing")
		require.NoError(t, err)

		// Should not include revoked key
		keys, err := service.ListAPIKeys(ctx, userID, false)
		require.NoError(t, err)
		for _, key := range keys {
			assert.NotEqual(t, created.ID, key.ID)
		}

		// Should include when requested
		keysAll, err := service.ListAPIKeys(ctx, userID, true)
		require.NoError(t, err)
		assert.Greater(t, len(keysAll), len(keys))
	})
}

func TestService_RotateAPIKey(t *testing.T) {
	service, _, cleanup := setupServiceTest(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()

	t.Run("rotates key successfully", func(t *testing.T) {
		original, err := service.CreateAPIKey(ctx, CreateAPIKeyInput{
			UserID:           userID,
			Name:             "Original Key",
			Scopes:           []Scope{ScopeCheckinsRead, ScopeCheckinsWrite},
			RateLimitPerDay:  5000,
			RateLimitPerHour: 500,
		})
		require.NoError(t, err)

		rotated, err := service.RotateAPIKey(ctx, original.ID, userID)
		require.NoError(t, err)
		require.NotNil(t, rotated)

		// New key should have different ID and key value
		assert.NotEqual(t, original.ID, rotated.ID)
		assert.NotEqual(t, original.PlainTextKey, rotated.PlainTextKey)

		// New key should have same settings
		assert.Equal(t, original.RateLimitPerDay, rotated.RateLimitPerDay)
		assert.Equal(t, original.RateLimitPerHour, rotated.RateLimitPerHour)

		// Old key should be revoked
		oldKey, err := service.GetAPIKey(ctx, original.ID, userID)
		require.NoError(t, err)
		assert.True(t, oldKey.Revoked)
	})
}

// Helper function for testing
var timeNow = time.Now

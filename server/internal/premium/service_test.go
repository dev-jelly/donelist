package premium

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_GetUserTierInfo(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	logger := zap.NewNop()

	userRepo := user.NewRepository(sqlxDB)
	subRepo := subscription.NewRepository(sqlxDB)
	service := NewService(sqlxDB, userRepo, subRepo, logger)

	ctx := context.Background()
	userID := uuid.New()

	t.Run("Free tier user", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "free",
				nil, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		info, err := service.GetUserTierInfo(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, TierFree, info.Tier)
		assert.True(t, info.IsActive)
		assert.False(t, info.IsInGrace)
		assert.Nil(t, info.ExpiresAt)
	})

	t.Run("Premium tier user - active", func(t *testing.T) {
		futureTime := time.Now().Add(30 * 24 * time.Hour)

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				futureTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		info, err := service.GetUserTierInfo(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, TierPremium, info.Tier)
		assert.True(t, info.IsActive)
		assert.False(t, info.IsInGrace)
		assert.NotNil(t, info.ExpiresAt)
		assert.True(t, info.ExpiresAt.After(time.Now()))
	})

	t.Run("Premium tier user - in grace period", func(t *testing.T) {
		expiredTime := time.Now().Add(-3 * 24 * time.Hour) // Expired 3 days ago

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				expiredTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		info, err := service.GetUserTierInfo(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, TierPremium, info.Tier)
		assert.False(t, info.IsActive)
		assert.True(t, info.IsInGrace)
		assert.NotNil(t, info.GraceEndsAt)
		assert.True(t, info.GraceEndsAt.After(time.Now()))
	})

	t.Run("Premium tier user - expired", func(t *testing.T) {
		expiredTime := time.Now().Add(-10 * 24 * time.Hour) // Expired 10 days ago (past grace period)

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				expiredTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		info, err := service.GetUserTierInfo(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, TierPremium, info.Tier)
		assert.False(t, info.IsActive)
		assert.False(t, info.IsInGrace)
	})
}

func TestService_CheckFeatureAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	logger := zap.NewNop()

	userRepo := user.NewRepository(sqlxDB)
	subRepo := subscription.NewRepository(sqlxDB)
	service := NewService(sqlxDB, userRepo, subRepo, logger)

	ctx := context.Background()
	userID := uuid.New()

	t.Run("Free tier accessing free feature", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "free",
				nil, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Free tier should not have access to advanced analytics
		err := service.CheckFeatureAccess(ctx, userID, FeatureAdvancedAnalytics)
		assert.Equal(t, ErrFeatureNotAvailable, err)
	})

	t.Run("Premium tier accessing premium feature", func(t *testing.T) {
		futureTime := time.Now().Add(30 * 24 * time.Hour)

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				futureTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Premium tier should have access to advanced analytics
		err := service.CheckFeatureAccess(ctx, userID, FeatureAdvancedAnalytics)
		assert.NoError(t, err)
	})

	t.Run("Premium tier accessing enterprise feature", func(t *testing.T) {
		futureTime := time.Now().Add(30 * 24 * time.Hour)

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				futureTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Premium tier should not have access to SSO
		err := service.CheckFeatureAccess(ctx, userID, FeatureSSO)
		assert.Equal(t, ErrFeatureNotAvailable, err)
	})
}

func TestService_CheckUsageLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	logger := zap.NewNop()

	userRepo := user.NewRepository(sqlxDB)
	subRepo := subscription.NewRepository(sqlxDB)
	service := NewService(sqlxDB, userRepo, subRepo, logger)

	ctx := context.Background()
	userID := uuid.New()

	t.Run("Within limit", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "free",
				nil, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Mock checkins count query
		mock.ExpectQuery("SELECT COUNT.+ FROM checkins WHERE user_id = .+").
			WithArgs(userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

		usage, err := service.CheckUsageLimit(ctx, userID, "checkins_per_day")
		require.NoError(t, err)
		assert.Equal(t, 10, usage.Current)
		assert.Equal(t, 50, usage.Limit) // Free tier limit
		assert.False(t, usage.Unlimited)
	})

	t.Run("Exceeded limit", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "free",
				nil, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Mock checkins count query - exceeding limit
		mock.ExpectQuery("SELECT COUNT.+ FROM checkins WHERE user_id = .+").
			WithArgs(userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(51))

		usage, err := service.CheckUsageLimit(ctx, userID, "checkins_per_day")
		assert.Equal(t, ErrUsageLimitExceeded, err)
		assert.Equal(t, 51, usage.Current)
		assert.Equal(t, 50, usage.Limit)
	})

	t.Run("Unlimited for premium", func(t *testing.T) {
		futureTime := time.Now().Add(30 * 24 * time.Hour)

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				futureTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Mock checkins count query
		mock.ExpectQuery("SELECT COUNT.+ FROM checkins WHERE user_id = .+").
			WithArgs(userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1000))

		usage, err := service.CheckUsageLimit(ctx, userID, "checkins_per_day")
		require.NoError(t, err)
		assert.Equal(t, 1000, usage.Current)
		assert.Equal(t, -1, usage.Limit) // Unlimited
		assert.True(t, usage.Unlimited)
	})
}

func TestService_UpgradeTier(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	logger := zap.NewNop()

	userRepo := user.NewRepository(sqlxDB)
	subRepo := subscription.NewRepository(sqlxDB)
	service := NewService(sqlxDB, userRepo, subRepo, logger)

	ctx := context.Background()
	userID := uuid.New()

	t.Run("Upgrade from free to premium", func(t *testing.T) {
		// Get current tier
		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "free",
				nil, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Update tier - correct parameter order: id, tier, expires_at
		mock.ExpectExec("UPDATE users SET tier = .+, tier_expires_at = .+").
			WithArgs(userID, "premium", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Create event
		mock.ExpectExec("INSERT INTO subscription_events").
			WithArgs(userID, "tier.upgraded", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := service.UpgradeTier(ctx, userID, TierPremium, 30*24*time.Hour)
		assert.NoError(t, err)
	})

	t.Run("Invalid upgrade path", func(t *testing.T) {
		// Get current tier (premium)
		futureTime := time.Now().Add(30 * 24 * time.Hour)

		mock.ExpectQuery("SELECT .+ FROM users WHERE id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "email", "password_hash", "display_name", "tier",
				"tier_expires_at", "mode_preference", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				userID, "test@example.com", "hash", nil, "premium",
				futureTime, "default", time.Now(), time.Now(), nil,
			))

		mock.ExpectQuery("SELECT .+ FROM subscriptions WHERE user_id = .+").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{}))

		// Attempting to "upgrade" to free should fail
		err := service.UpgradeTier(ctx, userID, TierFree, 30*24*time.Hour)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot upgrade")
	})
}
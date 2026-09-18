package premium

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestMiddleware(t *testing.T) (*Middleware, sqlmock.Sqlmock, func()) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	sqlxDB := sqlx.NewDb(db, "postgres")
	logger := zap.NewNop()

	userRepo := user.NewRepository(sqlxDB)
	subRepo := subscription.NewRepository(sqlxDB)
	service := NewService(sqlxDB, userRepo, subRepo, logger)
	middleware := NewMiddleware(service, logger)

	cleanup := func() {
		db.Close()
	}

	return middleware, mock, cleanup
}

func TestMiddleware_RequireTier(t *testing.T) {
	middleware, mock, cleanup := setupTestMiddleware(t)
	defer cleanup()

	userID := uuid.New()

	t.Run("Unauthorized - no claims", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		handler := middleware.RequireTier(TierPremium)
		handler(c)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Authentication required")
		assert.True(t, c.IsAborted())
	})

	t.Run("Valid premium tier access", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query
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

		handler := middleware.RequireTier(TierPremium)
		handler(c)

		// If not aborted, the handler would have been called
		assert.False(t, c.IsAborted())

		// Tier info is only added if GetUserTierInfo succeeds again
		// We would need another mock for that. For now just verify the handler didn't abort
	})

	t.Run("Insufficient tier", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - free tier trying to access premium
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

		// Mock again for getting user tier info
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

		handler := middleware.RequireTier(TierPremium)
		handler(c)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "Insufficient subscription tier")
		assert.True(t, c.IsAborted())
	})

	t.Run("Expired subscription", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - expired premium tier
		expiredTime := time.Now().Add(-10 * 24 * time.Hour) // Expired 10 days ago
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

		handler := middleware.RequireTier(TierPremium)
		handler(c)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "Subscription expired")
		assert.True(t, c.IsAborted())
	})
}

func TestMiddleware_RequireFeature(t *testing.T) {
	middleware, mock, cleanup := setupTestMiddleware(t)
	defer cleanup()

	userID := uuid.New()

	t.Run("Feature available for tier", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - premium tier
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


		handler := middleware.RequireFeature(FeatureAdvancedAnalytics)
		handler(c)

		// If not aborted, the handler would have been called
		assert.False(t, c.IsAborted())
	})

	t.Run("Feature not available", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - free tier
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

		handler := middleware.RequireFeature(FeatureAdvancedAnalytics)
		handler(c)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "Feature not available")
		assert.True(t, c.IsAborted())
	})
}

func TestMiddleware_CheckLimit(t *testing.T) {
	middleware, mock, cleanup := setupTestMiddleware(t)
	defer cleanup()

	userID := uuid.New()

	t.Run("Within limit", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - free tier
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

		// Mock checkins count - within limit
		mock.ExpectQuery("SELECT COUNT.+ FROM checkins WHERE user_id = .+").
			WithArgs(userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))


		handler := middleware.CheckLimit("checkins_per_day")
		handler(c)

		// If not aborted, the handler would have been called
		assert.False(t, c.IsAborted())

		// Check rate limit headers
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
	})

	t.Run("Limit exceeded", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - free tier
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

		// Mock checkins count - exceeded limit
		mock.ExpectQuery("SELECT COUNT.+ FROM checkins WHERE user_id = .+").
			WithArgs(userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(51))

		handler := middleware.CheckLimit("checkins_per_day")
		handler(c)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Contains(t, w.Body.String(), "Usage limit exceeded")
		assert.True(t, c.IsAborted())
	})
}

func TestMiddleware_ConvenienceMethods(t *testing.T) {
	middleware, _, cleanup := setupTestMiddleware(t)
	defer cleanup()

	t.Run("PremiumOnly", func(t *testing.T) {
		handler := middleware.PremiumOnly()
		assert.NotNil(t, handler)
	})

	t.Run("EnterpriseOnly", func(t *testing.T) {
		handler := middleware.EnterpriseOnly()
		assert.NotNil(t, handler)
	})
}

func TestMiddleware_InjectTierInfo(t *testing.T) {
	middleware, mock, cleanup := setupTestMiddleware(t)
	defer cleanup()

	userID := uuid.New()

	t.Run("Inject tier info on success", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query
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

		// Simulate successful response
		c.Writer.WriteHeader(http.StatusOK)

		handler := middleware.InjectTierInfo()
		handler(c)

		// Check headers
		assert.Equal(t, "premium", w.Header().Get("X-User-Tier"))
		assert.NotEmpty(t, w.Header().Get("X-Tier-Expires"))
		assert.NotEmpty(t, w.Header().Get("X-Available-Features"))
	})

	t.Run("Grace period headers", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		// Set claims
		claims := &auth.Claims{
			UserID: userID,
		}
		c.Set("claims", claims)

		// Mock user query - in grace period
		expiredTime := time.Now().Add(-3 * 24 * time.Hour)
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

		// Simulate successful response
		c.Writer.WriteHeader(http.StatusOK)

		handler := middleware.InjectTierInfo()
		handler(c)

		// Check grace period headers
		assert.Equal(t, "true", w.Header().Get("X-Grace-Period"))
		assert.NotEmpty(t, w.Header().Get("X-Grace-Ends"))
	})
}
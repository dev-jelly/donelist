// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/api/routes"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/tag"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/timeline"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/dev-jelly/donelist/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	_ "github.com/lib/pq"
)

type TestServer struct {
	Router *gin.Engine
	DB     *testutil.TestDB
}

func setupTestServer(t *testing.T) *TestServer {
	// Setup test database
	testDB := testutil.SetupTestDB(t)

	// Initialize repositories
	userRepo := user.NewRepository(testDB.DB)
	refreshTokenRepo := auth.NewRefreshTokenRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)
	tagRepo := tag.NewRepository(testDB.DB)

	// Initialize JWT manager
	jwtManager, err := auth.NewJWTManager("test-secret-key-for-testing")
	require.NoError(t, err)

	// Initialize logger
	logger := zap.NewNop()

	// Initialize WebSocket hub
	hub := websocket.NewHub(logger)
	go hub.Run()

	// Initialize services
	authService := auth.NewService(userRepo, refreshTokenRepo, jwtManager, logger)
	userService := user.NewService(userRepo, logger)
	checkinService := checkin.NewService(checkinRepo, categoryRepo, tagRepo, userRepo, hub, logger)
	timelineService := timeline.NewService(checkinRepo, logger)
	categoryService := category.NewService(categoryRepo, logger)
	tagService := tag.NewService(tagRepo, logger)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, logger)
	userHandler := handlers.NewUserHandler(userService, logger)
	checkinHandler := handlers.NewCheckinHandler(checkinService, logger)
	timelineHandler := handlers.NewTimelineHandler(timelineService, logger)
	categoryHandler := handlers.NewCategoryHandler(categoryService, logger)
	tagHandler := handlers.NewTagHandler(tagService, logger)
	wsHandler := handlers.NewWebSocketHandler(hub, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Setup routes
	routes.SetupRoutes(router, authHandler, userHandler, checkinHandler, timelineHandler, categoryHandler, tagHandler, wsHandler, jwtManager, logger)

	return &TestServer{
		Router: router,
		DB:     testDB,
	}
}

func TestAuthFlow_Integration(t *testing.T) {
	server := setupTestServer(t)
	defer server.DB.TearDown(t)

	var accessToken string
	var refreshToken string

	t.Run("Register new user", func(t *testing.T) {
		payload := map[string]string{
			"email":    "integration@example.com",
			"username": "integrationuser",
			"password": "SecurePassword123!",
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response["access_token"])
		assert.NotEmpty(t, response["refresh_token"])
		assert.NotNil(t, response["user"])

		accessToken = response["access_token"].(string)
		refreshToken = response["refresh_token"].(string)
	})

	t.Run("Login with registered user", func(t *testing.T) {
		payload := map[string]string{
			"email":    "integration@example.com",
			"password": "SecurePassword123!",
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response["access_token"])
		assert.NotEmpty(t, response["refresh_token"])

		// Update tokens
		accessToken = response["access_token"].(string)
		refreshToken = response["refresh_token"].(string)
	})

	t.Run("Access protected endpoint with token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		w := httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "integration@example.com", response["email"])
		assert.Equal(t, "integrationuser", response["username"])
	})

	t.Run("Refresh access token", func(t *testing.T) {
		payload := map[string]string{
			"refresh_token": refreshToken,
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response["access_token"])
		assert.NotEmpty(t, response["refresh_token"])

		// Update tokens
		newAccessToken := response["access_token"].(string)
		assert.NotEqual(t, accessToken, newAccessToken) // Should be different
		accessToken = newAccessToken
		refreshToken = response["refresh_token"].(string)
	})

	t.Run("Logout", func(t *testing.T) {
		payload := map[string]string{
			"refresh_token": refreshToken,
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify refresh token is invalidated
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Access protected endpoint without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		w := httptest.NewRecorder()

		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
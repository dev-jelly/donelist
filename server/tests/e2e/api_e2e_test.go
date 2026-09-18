// +build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/dev-jelly/donelist/internal/tag"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/timeline"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/dev-jelly/donelist/internal/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	_ "github.com/lib/pq"
)

// E2ETestClient provides helpers for end-to-end API testing
type E2ETestClient struct {
	Router       *gin.Engine
	DB           *testutil.TestDB
	AccessToken  string
	RefreshToken string
	UserID       string
}

// NewE2ETestClient creates a new E2E test client with full application setup
func NewE2ETestClient(t *testing.T) *E2ETestClient {
	// Setup test database
	testDB := testutil.SetupTestDB(t)

	// Initialize repositories
	userRepo := user.NewRepository(testDB.DB)
	refreshTokenRepo := auth.NewRefreshTokenRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)
	tagRepo := tag.NewRepository(testDB.DB)

	// Initialize JWT manager
	jwtConfig := auth.JWTConfig{
		SigningMethod: auth.SigningMethodHS256,
		Secret:        "test-secret-key-for-e2e-testing-must-be-at-least-32-characters-long",
		AccessExpiry:  15 * time.Minute,
		RefreshExpiry: 24 * time.Hour,
	}
	jwtManager, err := auth.NewJWTManager(jwtConfig)
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
	timelineService := timeline.NewService(checkinRepo, categoryRepo, logger)
	categoryService := category.NewService(categoryRepo, logger)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, logger)
	userHandler := handlers.NewUserHandler(userService, logger)
	checkinHandler := handlers.NewCheckinHandler(checkinService, logger)
	timelineHandler := handlers.NewTimelineHandler(timelineService, logger)
	categoryHandler := handlers.NewCategoryHandler(categoryService, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Setup basic routes manually for E2E testing
	authMiddleware := middleware.AuthMiddleware(jwtManager, logger)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Auth routes (public)
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		// Protected routes
		users := v1.Group("/users")
		users.Use(authMiddleware)
		{
			users.GET("/me", userHandler.GetMe)
			users.PUT("/me", userHandler.UpdateMe)
		}

		checkins := v1.Group("/checkins")
		checkins.Use(authMiddleware)
		{
			checkins.POST("", checkinHandler.Create)
			checkins.GET("", checkinHandler.List)
			checkins.GET("/:id", checkinHandler.GetByID)
			checkins.PUT("/:id", checkinHandler.Update)
			checkins.DELETE("/:id", checkinHandler.Delete)
		}

		categories := v1.Group("/categories")
		categories.Use(authMiddleware)
		{
			categories.POST("", categoryHandler.Create)
			categories.GET("", categoryHandler.List)
			categories.PUT("/:id", categoryHandler.Update)
			categories.DELETE("/:id", categoryHandler.Delete)
		}

		timeline := v1.Group("/timeline")
		timeline.Use(authMiddleware)
		{
			timeline.GET("/daily", timelineHandler.GetDaily)
		}
	}

	// Health endpoints
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ready"})
	})

	return &E2ETestClient{
		Router: router,
		DB:     testDB,
	}
}

// Request executes an HTTP request and returns the response
func (c *E2ETestClient) Request(method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)

	// Set default headers
	req.Header.Set("Content-Type", "application/json")

	// Add authorization if access token is set
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	w := httptest.NewRecorder()
	c.Router.ServeHTTP(w, req)

	return w
}

// Register creates a new user account
func (c *E2ETestClient) Register(email, displayName, password string) (*httptest.ResponseRecorder, error) {
	payload := map[string]interface{}{
		"email":        email,
		"display_name": displayName,
		"password":     password,
	}

	w := c.Request(http.MethodPost, "/api/v1/auth/register", payload, nil)

	if w.Code == http.StatusCreated {
		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			return w, err
		}
		if token, ok := response["access_token"].(string); ok {
			c.AccessToken = token
		}
		if token, ok := response["refresh_token"].(string); ok {
			c.RefreshToken = token
		}
		if user, ok := response["user"].(map[string]interface{}); ok {
			if id, ok := user["id"].(string); ok {
				c.UserID = id
			}
		}
	}

	return w, nil
}

// Login authenticates a user
func (c *E2ETestClient) Login(email, password string) (*httptest.ResponseRecorder, error) {
	payload := map[string]string{
		"email":    email,
		"password": password,
	}

	w := c.Request(http.MethodPost, "/api/v1/auth/login", payload, nil)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			return w, err
		}
		c.AccessToken = response["access_token"].(string)
		c.RefreshToken = response["refresh_token"].(string)
	}

	return w, nil
}

// Cleanup tears down the test environment
func (c *E2ETestClient) Cleanup(t *testing.T) {
	c.DB.TearDown(t)
}

// TestCompleteUserJourney tests the entire user flow from signup to data export
func TestCompleteUserJourney(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	var checkinID string
	var categoryID string

	// Step 1: User Registration
	t.Run("User registers for account", func(t *testing.T) {
		w, err := client.Register("journey@example.com", "journeyuser", "SecurePass123!")
		require.NoError(t, err)

		if w.Code != http.StatusCreated {
			t.Logf("Registration failed with status %d: %s", w.Code, w.Body.String())
		}

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.NotEmpty(t, client.AccessToken, "Access token should not be empty")
		assert.NotEmpty(t, client.RefreshToken, "Refresh token should not be empty")
	})

	// Step 2: User Profile Access
	t.Run("User accesses profile", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/users/me", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "journey@example.com", response["email"])
	})

	// Step 3: Create Category
	t.Run("User creates a category", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "Work",
			"color": "#3498db",
			"icon":  "briefcase",
		}
		w := client.Request(http.MethodPost, "/api/v1/categories", payload, nil)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		categoryID = response["id"].(string)
	})

	// Step 4: Create Multiple Check-ins
	t.Run("User creates check-ins", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			payload := map[string]interface{}{
				"title":       fmt.Sprintf("Task %d", i),
				"description": fmt.Sprintf("Description for task %d", i),
				"category_id": categoryID,
				"tags":        []string{"important", "urgent"},
			}
			w := client.Request(http.MethodPost, "/api/v1/checkins", payload, nil)
			assert.Equal(t, http.StatusCreated, w.Code)

			if i == 1 {
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)
				checkinID = response["id"].(string)
			}
		}
	})

	// Step 5: List Check-ins with Pagination
	t.Run("User lists check-ins with pagination", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/checkins?page=1&limit=10", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		checkins := response["checkins"].([]interface{})
		assert.Len(t, checkins, 3)
	})

	// Step 6: Update Check-in
	t.Run("User updates a check-in", func(t *testing.T) {
		payload := map[string]interface{}{
			"title":       "Updated Task",
			"description": "Updated description",
			"status":      "completed",
		}
		path := fmt.Sprintf("/api/v1/checkins/%s", checkinID)
		w := client.Request(http.MethodPut, path, payload, nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Step 7: Search Check-ins
	t.Run("User searches check-ins", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/checkins/search?q=Updated", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		results := response["results"].([]interface{})
		assert.GreaterOrEqual(t, len(results), 1)
	})

	// Step 8: Get Timeline
	t.Run("User views timeline", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/timeline", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Step 9: Token Refresh
	t.Run("User refreshes access token", func(t *testing.T) {
		payload := map[string]string{
			"refresh_token": client.RefreshToken,
		}
		w := client.Request(http.MethodPost, "/api/v1/auth/refresh", payload, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response["access_token"])
	})

	// Step 10: Logout
	t.Run("User logs out", func(t *testing.T) {
		payload := map[string]string{
			"refresh_token": client.RefreshToken,
		}
		w := client.Request(http.MethodPost, "/api/v1/auth/logout", payload, nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestAuthenticationFlow tests complete authentication scenarios
func TestAuthenticationFlow(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	t.Run("Register with weak password", func(t *testing.T) {
		w, _ := client.Register("weak@example.com", "weakuser", "weak")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Register with duplicate email", func(t *testing.T) {
		client.Register("duplicate@example.com", "user1", "SecurePass123!")
		w, _ := client.Register("duplicate@example.com", "user2", "SecurePass456!")
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Login with invalid credentials", func(t *testing.T) {
		w, _ := client.Login("invalid@example.com", "wrongpassword")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Access protected endpoint without token", func(t *testing.T) {
		client.AccessToken = ""
		w := client.Request(http.MethodGet, "/api/v1/users/me", nil, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Access protected endpoint with invalid token", func(t *testing.T) {
		client.AccessToken = "invalid-token"
		w := client.Request(http.MethodGet, "/api/v1/users/me", nil, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// TestAPIResponseSchemas validates that all API responses follow the expected schema
func TestAPIResponseSchemas(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	// Register and login
	client.Register("schema@example.com", "schemauser", "SecurePass123!")

	t.Run("User profile response schema", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/users/me", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify required fields
		assert.Contains(t, response, "id")
		assert.Contains(t, response, "email")
		assert.Contains(t, response, "username")
		assert.Contains(t, response, "created_at")

		// Verify sensitive fields are not exposed
		assert.NotContains(t, response, "password")
		assert.NotContains(t, response, "password_hash")
	})

	t.Run("Category response schema", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "Test Category",
			"color": "#3498db",
		}
		w := client.Request(http.MethodPost, "/api/v1/categories", payload, nil)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "id")
		assert.Contains(t, response, "name")
		assert.Contains(t, response, "color")
		assert.Contains(t, response, "created_at")
	})

	t.Run("Error response schema", func(t *testing.T) {
		w := client.Request(http.MethodPost, "/api/v1/categories", map[string]interface{}{}, nil)
		assert.NotEqual(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Error responses should have error field
		assert.Contains(t, response, "error")
	})
}

// TestPaginationAndFiltering tests pagination and filtering across endpoints
func TestPaginationAndFiltering(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	client.Register("pagination@example.com", "paginationuser", "SecurePass123!")

	// Create multiple check-ins
	categoryPayload := map[string]interface{}{
		"name":  "Test",
		"color": "#3498db",
	}
	catW := client.Request(http.MethodPost, "/api/v1/categories", categoryPayload, nil)
	var catResponse map[string]interface{}
	json.Unmarshal(catW.Body.Bytes(), &catResponse)
	categoryID := catResponse["id"].(string)

	for i := 0; i < 25; i++ {
		payload := map[string]interface{}{
			"title":       fmt.Sprintf("Checkin %d", i),
			"description": fmt.Sprintf("Description %d", i),
			"category_id": categoryID,
		}
		client.Request(http.MethodPost, "/api/v1/checkins", payload, nil)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	t.Run("Default pagination", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/checkins", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		checkins := response["checkins"].([]interface{})
		assert.LessOrEqual(t, len(checkins), 20) // Default page size
	})

	t.Run("Custom page size", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/api/v1/checkins?limit=5", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		checkins := response["checkins"].([]interface{})
		assert.LessOrEqual(t, len(checkins), 5)
	})

	t.Run("Page navigation", func(t *testing.T) {
		w1 := client.Request(http.MethodGet, "/api/v1/checkins?page=1&limit=10", nil, nil)
		w2 := client.Request(http.MethodGet, "/api/v1/checkins?page=2&limit=10", nil, nil)

		assert.Equal(t, http.StatusOK, w1.Code)
		assert.Equal(t, http.StatusOK, w2.Code)

		var resp1, resp2 map[string]interface{}
		json.Unmarshal(w1.Body.Bytes(), &resp1)
		json.Unmarshal(w2.Body.Bytes(), &resp2)

		checkins1 := resp1["checkins"].([]interface{})
		checkins2 := resp2["checkins"].([]interface{})

		// Ensure different results on different pages
		if len(checkins1) > 0 && len(checkins2) > 0 {
			id1 := checkins1[0].(map[string]interface{})["id"]
			id2 := checkins2[0].(map[string]interface{})["id"]
			assert.NotEqual(t, id1, id2)
		}
	})

	t.Run("Filter by category", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/checkins?category_id=%s", categoryID)
		w := client.Request(http.MethodGet, path, nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		checkins := response["checkins"].([]interface{})

		// All checkins should belong to the specified category
		for _, item := range checkins {
			checkin := item.(map[string]interface{})
			assert.Equal(t, categoryID, checkin["category_id"])
		}
	})
}

// TestConcurrentRequests tests the API's ability to handle concurrent requests
func TestConcurrentRequests(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	client.Register("concurrent@example.com", "concurrentuser", "SecurePass123!")

	t.Run("Concurrent profile reads", func(t *testing.T) {
		results := make(chan int, 10)

		for i := 0; i < 10; i++ {
			go func() {
				w := client.Request(http.MethodGet, "/api/v1/users/me", nil, nil)
				results <- w.Code
			}()
		}

		// Collect results
		for i := 0; i < 10; i++ {
			code := <-results
			assert.Equal(t, http.StatusOK, code)
		}
	})
}

// TestInputValidation tests input validation across all endpoints
func TestInputValidation(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	client.Register("validation@example.com", "validationuser", "SecurePass123!")

	t.Run("Checkin with missing required fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"description": "Missing title",
		}
		w := client.Request(http.MethodPost, "/api/v1/checkins", payload, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Checkin with invalid category ID", func(t *testing.T) {
		payload := map[string]interface{}{
			"title":       "Test",
			"category_id": "invalid-uuid",
		}
		w := client.Request(http.MethodPost, "/api/v1/checkins", payload, nil)
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("Checkin with XSS attempt", func(t *testing.T) {
		payload := map[string]interface{}{
			"title":       "<script>alert('xss')</script>",
			"description": "XSS test",
		}
		w := client.Request(http.MethodPost, "/api/v1/checkins", payload, nil)

		if w.Code == http.StatusCreated {
			var response map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &response)

			// Title should be sanitized
			title := response["title"].(string)
			assert.NotContains(t, title, "<script>")
		}
	})
}

// TestHealthAndMetrics tests health check and metrics endpoints
func TestHealthAndMetrics(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	t.Run("Health check endpoint", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/health", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "healthy", response["status"])
	})

	t.Run("Readiness check endpoint", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/health/ready", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

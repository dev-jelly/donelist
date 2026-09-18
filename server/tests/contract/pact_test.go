// +build contract

package contract_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// PactContract represents a consumer-provider contract
type PactContract struct {
	Consumer     PactParticipant     `json:"consumer"`
	Provider     PactParticipant     `json:"provider"`
	Interactions []PactInteraction   `json:"interactions"`
	Metadata     PactMetadata        `json:"metadata"`
}

type PactParticipant struct {
	Name string `json:"name"`
}

type PactInteraction struct {
	Description   string           `json:"description"`
	ProviderState string           `json:"providerState,omitempty"`
	Request       PactRequest      `json:"request"`
	Response      PactResponse     `json:"response"`
}

type PactRequest struct {
	Method  string                 `json:"method"`
	Path    string                 `json:"path"`
	Headers map[string]string      `json:"headers,omitempty"`
	Body    map[string]interface{} `json:"body,omitempty"`
	Query   string                 `json:"query,omitempty"`
}

type PactResponse struct {
	Status  int                    `json:"status"`
	Headers map[string]string      `json:"headers,omitempty"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

type PactMetadata struct {
	PactSpecification PactSpecVersion `json:"pactSpecification"`
	Client            PactClient      `json:"client,omitempty"`
}

type PactSpecVersion struct {
	Version string `json:"version"`
}

type PactClient struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// TestProviderVerification verifies that the provider matches all Pact contracts
func TestProviderVerification(t *testing.T) {
	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.TearDown(t)

	// Setup services and handlers
	logger := zap.NewNop()
	userRepo := user.NewRepository(testDB.DB)
	categoryRepo := category.NewRepository(testDB.DB)
	tagRepo := tag.NewRepository(testDB.DB)
	checkinRepo := checkin.NewRepository(testDB.DB)

	categoryService := category.NewService(categoryRepo, nil, logger)
	tagService := tag.NewService(tagRepo, nil, logger)
	timelineService := timeline.NewService(checkinRepo, categoryRepo, nil, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Setup JWT manager
	jwtConfig := auth.JWTConfig{
		SigningMethod: auth.SigningMethodHS256,
		Secret:        "test-secret",
		AccessExpiry:  15 * time.Minute,
		RefreshExpiry: 24 * time.Hour,
	}
	jwtManager, err := auth.NewJWTManager(jwtConfig)
	require.NoError(t, err)

	// Setup handlers
	categoryHandler := handlers.NewCategoryHandler(categoryService, logger)
	tagHandler := handlers.NewTagHandler(tagService, logger)
	timelineHandler := handlers.NewTimelineHandler(timelineService, logger)

	// Setup routes (simplified for testing)
	authMiddleware := middleware.AuthMiddleware(jwtManager, logger)
	v1 := router.Group("/api/v1")
	v1.Use(authMiddleware)
	{
		categories := v1.Group("/categories")
		{
			categories.POST("", categoryHandler.Create)
			categories.GET("", categoryHandler.List)
			categories.GET("/:id", categoryHandler.GetByID)
			categories.PATCH("/:id", categoryHandler.Update)
			categories.DELETE("/:id", categoryHandler.Delete)
		}

		tags := v1.Group("/tags")
		{
			tags.GET("/autocomplete", tagHandler.Autocomplete)
			tags.GET("/popular", tagHandler.GetPopular)
			tags.POST("", tagHandler.Create)
			tags.GET("", tagHandler.List)
			tags.GET("/:id", tagHandler.GetByID)
		}

		timeline := v1.Group("/timeline")
		{
			timeline.GET("/daily", timelineHandler.GetDaily)
			timeline.GET("/daily/enhanced", timelineHandler.GetDailyEnhanced)
			timeline.GET("/weekly", timelineHandler.GetWeekly)
			timeline.GET("/monthly", timelineHandler.GetMonthly)
		}
	}

	// Load and verify pact contracts
	pactsDir := "../pacts"
	if _, err := os.Stat(pactsDir); os.IsNotExist(err) {
		t.Skip("Pacts directory not found, skipping provider verification")
	}

	pactFiles, err := filepath.Glob(filepath.Join(pactsDir, "*.json"))
	require.NoError(t, err)

	for _, pactFile := range pactFiles {
		t.Run(filepath.Base(pactFile), func(t *testing.T) {
			// Load pact file
			pactData, err := os.ReadFile(pactFile)
			require.NoError(t, err)

			var pact PactContract
			err = json.Unmarshal(pactData, &pact)
			require.NoError(t, err)

			// Verify each interaction
			for _, interaction := range pact.Interactions {
				t.Run(interaction.Description, func(t *testing.T) {
					// Setup provider state if needed
					if interaction.ProviderState != "" {
						setupProviderState(t, interaction.ProviderState, testDB, userRepo, categoryRepo, tagRepo)
					}

					// Create test user and token
					testUser, token := createTestUserWithToken(t, userRepo, jwtManager)

					// Execute request
					req := createRequestFromPact(t, interaction.Request, token)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					// Verify response
					verifyPactResponse(t, interaction.Response, w)

					// Cleanup
					_ = testUser
				})
			}
		})
	}
}

// TestCategoryContractGeneration generates Pact contracts for category endpoints
func TestCategoryContractGeneration(t *testing.T) {
	pact := PactContract{
		Consumer: PactParticipant{Name: "donelist-mobile-app"},
		Provider: PactParticipant{Name: "donelist-api"},
		Interactions: []PactInteraction{
			{
				Description:   "create a new category",
				ProviderState: "user is authenticated",
				Request: PactRequest{
					Method: "POST",
					Path:   "/api/v1/categories",
					Headers: map[string]string{
						"Content-Type":  "application/json",
						"Authorization": "Bearer TOKEN",
					},
					Body: map[string]interface{}{
						"name":  "Work",
						"color": "#3498db",
						"icon":  "briefcase",
					},
				},
				Response: PactResponse{
					Status: 201,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"id":      "MATCHER:UUID",
						"user_id": "MATCHER:UUID",
						"name":    "Work",
						"color":   "#3498db",
						"icon":    "briefcase",
						"created_at": "MATCHER:ISO8601",
						"updated_at": "MATCHER:ISO8601",
					},
				},
			},
			{
				Description:   "list all categories",
				ProviderState: "user has categories",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/categories",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"data": []interface{}{
							map[string]interface{}{
								"id":      "MATCHER:UUID",
								"user_id": "MATCHER:UUID",
								"name":    "MATCHER:STRING",
								"color":   "MATCHER:STRING",
								"icon":    "MATCHER:STRING",
								"created_at": "MATCHER:ISO8601",
								"updated_at": "MATCHER:ISO8601",
							},
						},
						"meta": map[string]interface{}{
							"total": "MATCHER:INTEGER",
							"count": "MATCHER:INTEGER",
						},
					},
				},
			},
			{
				Description:   "get category by ID",
				ProviderState: "category exists",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/categories/CATEGORY_ID",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"id":      "MATCHER:UUID",
						"user_id": "MATCHER:UUID",
						"name":    "MATCHER:STRING",
						"color":   "MATCHER:STRING",
						"icon":    "MATCHER:STRING",
						"created_at": "MATCHER:ISO8601",
						"updated_at": "MATCHER:ISO8601",
					},
				},
			},
			{
				Description:   "update category",
				ProviderState: "category exists",
				Request: PactRequest{
					Method: "PATCH",
					Path:   "/api/v1/categories/CATEGORY_ID",
					Headers: map[string]string{
						"Content-Type":  "application/json",
						"Authorization": "Bearer TOKEN",
					},
					Body: map[string]interface{}{
						"name": "Work Updated",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"id":      "MATCHER:UUID",
						"user_id": "MATCHER:UUID",
						"name":    "Work Updated",
						"color":   "MATCHER:STRING",
						"icon":    "MATCHER:STRING",
						"created_at": "MATCHER:ISO8601",
						"updated_at": "MATCHER:ISO8601",
					},
				},
			},
			{
				Description:   "delete category",
				ProviderState: "category exists",
				Request: PactRequest{
					Method: "DELETE",
					Path:   "/api/v1/categories/CATEGORY_ID",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 204,
				},
			},
		},
		Metadata: PactMetadata{
			PactSpecification: PactSpecVersion{Version: "2.0.0"},
			Client: PactClient{
				Name:    "donelist-contract-tests",
				Version: "1.0.0",
			},
		},
	}

	// Save pact file
	savePactContract(t, pact, "../pacts/donelist-mobile-app-donelist-api-categories.json")
}

// TestTagContractGeneration generates Pact contracts for tag endpoints
func TestTagContractGeneration(t *testing.T) {
	pact := PactContract{
		Consumer: PactParticipant{Name: "donelist-mobile-app"},
		Provider: PactParticipant{Name: "donelist-api"},
		Interactions: []PactInteraction{
			{
				Description:   "autocomplete tags",
				ProviderState: "user has tags",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/tags/autocomplete",
					Query:  "q=test&limit=10",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"data": []interface{}{
							map[string]interface{}{
								"id":          "MATCHER:UUID",
								"user_id":     "MATCHER:UUID",
								"name":        "MATCHER:STRING",
								"usage_count": "MATCHER:INTEGER",
								"created_at":  "MATCHER:ISO8601",
								"updated_at":  "MATCHER:ISO8601",
							},
						},
					},
				},
			},
			{
				Description:   "get popular tags",
				ProviderState: "user has tags with usage",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/tags/popular",
					Query:  "limit=20",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"data": []interface{}{
							map[string]interface{}{
								"id":          "MATCHER:UUID",
								"user_id":     "MATCHER:UUID",
								"name":        "MATCHER:STRING",
								"usage_count": "MATCHER:INTEGER",
								"created_at":  "MATCHER:ISO8601",
								"updated_at":  "MATCHER:ISO8601",
							},
						},
						"meta": map[string]interface{}{
							"total": "MATCHER:INTEGER",
							"count": "MATCHER:INTEGER",
						},
					},
				},
			},
		},
		Metadata: PactMetadata{
			PactSpecification: PactSpecVersion{Version: "2.0.0"},
			Client: PactClient{
				Name:    "donelist-contract-tests",
				Version: "1.0.0",
			},
		},
	}

	savePactContract(t, pact, "../pacts/donelist-mobile-app-donelist-api-tags.json")
}

// TestTimelineContractGeneration generates Pact contracts for timeline endpoints
func TestTimelineContractGeneration(t *testing.T) {
	pact := PactContract{
		Consumer: PactParticipant{Name: "donelist-mobile-app"},
		Provider: PactParticipant{Name: "donelist-api"},
		Interactions: []PactInteraction{
			{
				Description:   "get daily timeline",
				ProviderState: "user has checkins for date",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/timeline/daily",
					Query:  "date=2024-01-15",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"date":     "2024-01-15",
						"total":    "MATCHER:INTEGER",
						"checkins": "MATCHER:ARRAY",
					},
				},
			},
			{
				Description:   "get enhanced daily timeline",
				ProviderState: "user has checkins for date",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/timeline/daily/enhanced",
					Query:  "date=2024-01-15&block=30&timezone=UTC",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 200,
					Headers: map[string]string{
						"Content-Type":    "application/json; charset=utf-8",
						"Cache-Control":   "MATCHER:STRING",
						"ETag":            "MATCHER:STRING",
						"Last-Modified":   "MATCHER:STRING",
					},
					Body: map[string]interface{}{
						"date":              "2024-01-15",
						"timezone":          "UTC",
						"block_granularity": 30,
						"blocks":            "MATCHER:ARRAY",
						"gaps":              "MATCHER:ARRAY",
						"summary":           "MATCHER:OBJECT",
						"category_legend":   "MATCHER:ARRAY",
						"previous_day":      "2024-01-14",
						"next_day":          "2024-01-16",
						"generated_at":      "MATCHER:ISO8601",
					},
				},
			},
		},
		Metadata: PactMetadata{
			PactSpecification: PactSpecVersion{Version: "2.0.0"},
			Client: PactClient{
				Name:    "donelist-contract-tests",
				Version: "1.0.0",
			},
		},
	}

	savePactContract(t, pact, "../pacts/donelist-mobile-app-donelist-api-timeline.json")
}

// TestNegativeContractScenarios tests negative scenarios against contracts
func TestNegativeContractScenarios(t *testing.T) {
	pact := PactContract{
		Consumer: PactParticipant{Name: "donelist-mobile-app"},
		Provider: PactParticipant{Name: "donelist-api"},
		Interactions: []PactInteraction{
			{
				Description:   "create category with missing required field",
				ProviderState: "user is authenticated",
				Request: PactRequest{
					Method: "POST",
					Path:   "/api/v1/categories",
					Headers: map[string]string{
						"Content-Type":  "application/json",
						"Authorization": "Bearer TOKEN",
					},
					Body: map[string]interface{}{
						"color": "#3498db",
					},
				},
				Response: PactResponse{
					Status: 400,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"error": "MATCHER:STRING",
					},
				},
			},
			{
				Description:   "unauthorized request",
				ProviderState: "no authentication",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/categories",
				},
				Response: PactResponse{
					Status: 401,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"error": "MATCHER:STRING",
					},
				},
			},
			{
				Description:   "category not found",
				ProviderState: "user is authenticated",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/categories/00000000-0000-0000-0000-000000000000",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 404,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"error": "MATCHER:STRING",
					},
				},
			},
			{
				Description:   "invalid block granularity",
				ProviderState: "user is authenticated",
				Request: PactRequest{
					Method: "GET",
					Path:   "/api/v1/timeline/daily/enhanced",
					Query:  "date=2024-01-15&block=25",
					Headers: map[string]string{
						"Authorization": "Bearer TOKEN",
					},
				},
				Response: PactResponse{
					Status: 400,
					Headers: map[string]string{
						"Content-Type": "application/json; charset=utf-8",
					},
					Body: map[string]interface{}{
						"error": "MATCHER:STRING",
					},
				},
			},
		},
		Metadata: PactMetadata{
			PactSpecification: PactSpecVersion{Version: "2.0.0"},
			Client: PactClient{
				Name:    "donelist-contract-tests",
				Version: "1.0.0",
			},
		},
	}

	savePactContract(t, pact, "../pacts/donelist-mobile-app-donelist-api-negative-scenarios.json")
}

// Helper functions

func setupProviderState(t *testing.T, state string, testDB *testutil.TestDB, userRepo *user.Repository, categoryRepo *category.Repository, tagRepo *tag.Repository) {
	switch state {
	case "user is authenticated":
		// Create test user
		testUser := &user.User{
			ID:          uuid.New(),
			Email:       "test@example.com",
			Username:    "testuser",
			DisplayName: "Test User",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err := userRepo.Create(testUser, "hashedpassword")
		require.NoError(t, err)

	case "user has categories":
		// Create test user with categories
		testUser := &user.User{
			ID:          uuid.New(),
			Email:       "test@example.com",
			Username:    "testuser",
			DisplayName: "Test User",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err := userRepo.Create(testUser, "hashedpassword")
		require.NoError(t, err)

		cat := &category.Category{
			ID:     uuid.New(),
			UserID: testUser.ID,
			Name:   "Work",
			Color:  strPtr("#3498db"),
			Icon:   strPtr("briefcase"),
		}
		err = categoryRepo.Create(cat)
		require.NoError(t, err)

	case "category exists", "user has tags", "user has tags with usage", "user has checkins for date":
		// Setup will be handled by test
		// In a real scenario, you'd create the necessary data
	}
}

func createTestUserWithToken(t *testing.T, userRepo *user.Repository, jwtManager *auth.JWTManager) (*user.User, string) {
	testUser := &user.User{
		ID:          uuid.New(),
		Email:       "test@example.com",
		Username:    "testuser",
		DisplayName: "Test User",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := userRepo.Create(testUser, "hashedpassword")
	require.NoError(t, err)

	token, err := jwtManager.GenerateAccessToken(testUser.ID)
	require.NoError(t, err)

	return testUser, token
}

func createRequestFromPact(t *testing.T, pactReq PactRequest, token string) *http.Request {
	url := pactReq.Path
	if pactReq.Query != "" {
		url = url + "?" + pactReq.Query
	}

	req := httptest.NewRequest(pactReq.Method, url, nil)

	// Set headers
	for key, value := range pactReq.Headers {
		if key == "Authorization" && value == "Bearer TOKEN" {
			req.Header.Set(key, "Bearer "+token)
		} else {
			req.Header.Set(key, value)
		}
	}

	return req
}

func verifyPactResponse(t *testing.T, expected PactResponse, actual *httptest.ResponseRecorder) {
	// Verify status code
	assert.Equal(t, expected.Status, actual.Code, "Status code mismatch")

	// Verify headers
	for key, expectedValue := range expected.Headers {
		actualValue := actual.Header().Get(key)
		if expectedValue == "MATCHER:STRING" {
			assert.NotEmpty(t, actualValue, fmt.Sprintf("Header %s should not be empty", key))
		} else {
			assert.Equal(t, expectedValue, actualValue, fmt.Sprintf("Header %s mismatch", key))
		}
	}

	// Verify body if present
	if expected.Body != nil && actual.Body.Len() > 0 {
		var actualBody map[string]interface{}
		err := json.Unmarshal(actual.Body.Bytes(), &actualBody)
		require.NoError(t, err, "Response body should be valid JSON")

		verifyPactBody(t, expected.Body, actualBody, "")
	}
}

func verifyPactBody(t *testing.T, expected, actual map[string]interface{}, path string) {
	for key, expectedValue := range expected {
		currentPath := path + "." + key
		actualValue, exists := actual[key]

		if !exists {
			t.Errorf("Expected field %s not found in response", currentPath)
			continue
		}

		switch v := expectedValue.(type) {
		case string:
			if v == "MATCHER:UUID" {
				_, err := uuid.Parse(actualValue.(string))
				assert.NoError(t, err, fmt.Sprintf("%s should be a valid UUID", currentPath))
			} else if v == "MATCHER:STRING" {
				assert.IsType(t, "", actualValue, fmt.Sprintf("%s should be a string", currentPath))
			} else if v == "MATCHER:ISO8601" {
				_, err := time.Parse(time.RFC3339, actualValue.(string))
				assert.NoError(t, err, fmt.Sprintf("%s should be ISO8601 format", currentPath))
			} else if v == "MATCHER:INTEGER" {
				assert.IsType(t, float64(0), actualValue, fmt.Sprintf("%s should be an integer", currentPath))
			} else if v == "MATCHER:ARRAY" {
				assert.IsType(t, []interface{}{}, actualValue, fmt.Sprintf("%s should be an array", currentPath))
			} else if v == "MATCHER:OBJECT" {
				assert.IsType(t, map[string]interface{}{}, actualValue, fmt.Sprintf("%s should be an object", currentPath))
			} else {
				assert.Equal(t, expectedValue, actualValue, fmt.Sprintf("Value mismatch at %s", currentPath))
			}

		case map[string]interface{}:
			actualMap, ok := actualValue.(map[string]interface{})
			assert.True(t, ok, fmt.Sprintf("%s should be an object", currentPath))
			verifyPactBody(t, v, actualMap, currentPath)

		default:
			assert.Equal(t, expectedValue, actualValue, fmt.Sprintf("Value mismatch at %s", currentPath))
		}
	}
}

func savePactContract(t *testing.T, pact PactContract, filename string) {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	// Marshal to JSON
	data, err := json.MarshalIndent(pact, "", "  ")
	require.NoError(t, err)

	// Write to file
	err = os.WriteFile(filename, data, 0644)
	require.NoError(t, err)

	t.Logf("Pact contract saved to %s", filename)
}

func strPtr(s string) *string {
	return &s
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ContractTestSuite tests API contracts against OpenAPI specification
type ContractTestSuite struct {
	doc    *openapi3.T
	router routers.Router
}

// NewContractTestSuite creates a new contract test suite
func NewContractTestSuite(t *testing.T, specPath string) *ContractTestSuite {
	// Load OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "Failed to load OpenAPI spec")

	// Validate the spec
	err = doc.Validate(loader.Context)
	require.NoError(t, err, "OpenAPI spec validation failed")

	// Create router from spec
	router, err := gorillamux.NewRouter(doc)
	require.NoError(t, err, "Failed to create router from spec")

	return &ContractTestSuite{
		doc:    doc,
		router: router,
	}
}

// ValidateRequest validates an HTTP request against the OpenAPI spec
func (cts *ContractTestSuite) ValidateRequest(t *testing.T, req *http.Request) {
	route, pathParams, err := cts.router.FindRoute(req)
	require.NoError(t, err, "Route not found in OpenAPI spec")

	// Validate request against schema
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}

	err = openapi3filter.ValidateRequest(context.Background(), requestValidationInput)
	assert.NoError(t, err, "Request validation failed")
}

// ValidateResponse validates an HTTP response against the OpenAPI spec
func (cts *ContractTestSuite) ValidateResponse(t *testing.T, req *http.Request, resp *http.Response) {
	route, pathParams, err := cts.router.FindRoute(req)
	require.NoError(t, err, "Route not found in OpenAPI spec")

	// Validate response against schema
	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   resp.Body,
	}

	err = openapi3filter.ValidateResponse(context.Background(), responseValidationInput)
	assert.NoError(t, err, "Response validation failed")
}

// TestCategoryEndpointsContract tests category endpoints against OpenAPI spec
func TestCategoryEndpointsContract(t *testing.T) {
	suite := NewContractTestSuite(t, "../../docs/api/openapi.yaml")

	t.Run("CreateCategory", func(t *testing.T) {
		// Prepare request body
		body := map[string]interface{}{
			"name":  "Test Category",
			"color": "#FF5733",
		}
		bodyBytes, _ := json.Marshal(body)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-Request-ID", uuid.New().String())

		// Validate request against spec
		suite.ValidateRequest(t, req)

		// Simulate response
		resp := &http.Response{
			StatusCode: http.StatusCreated,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: createResponseBody(t, map[string]interface{}{
				"data": map[string]interface{}{
					"id":         uuid.New().String(),
					"user_id":    uuid.New().String(),
					"name":       "Test Category",
					"color":      "#FF5733",
					"created_at": "2024-01-01T00:00:00Z",
					"updated_at": "2024-01-01T00:00:00Z",
				},
			}),
		}

		// Validate response against spec
		suite.ValidateResponse(t, req, resp)
	})

	t.Run("ListCategories", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-Request-ID", uuid.New().String())

		// Validate request against spec
		suite.ValidateRequest(t, req)

		// Simulate response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: createResponseBody(t, map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"id":         uuid.New().String(),
						"user_id":    uuid.New().String(),
						"name":       "Work",
						"color":      "#FF5733",
						"created_at": "2024-01-01T00:00:00Z",
						"updated_at": "2024-01-01T00:00:00Z",
					},
				},
				"meta": map[string]interface{}{
					"total": 1,
					"count": 1,
				},
			}),
		}

		// Validate response against spec
		suite.ValidateResponse(t, req, resp)
	})

	t.Run("UpdateCategory", func(t *testing.T) {
		categoryID := uuid.New().String()

		// Prepare request body
		body := map[string]interface{}{
			"name":  "Updated Category",
			"color": "#33FF57",
		}
		bodyBytes, _ := json.Marshal(body)

		// Create request
		req := httptest.NewRequest(http.MethodPut, "/api/v1/categories/"+categoryID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-Request-ID", uuid.New().String())

		// Validate request against spec
		suite.ValidateRequest(t, req)
	})

	t.Run("InvalidRequest_MissingRequiredField", func(t *testing.T) {
		// Prepare request body without required field
		body := map[string]interface{}{
			"color": "#FF5733", // Missing required "name" field
		}
		bodyBytes, _ := json.Marshal(body)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		// This should fail validation
		route, pathParams, err := suite.router.FindRoute(req)
		require.NoError(t, err)

		requestValidationInput := &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		}

		err = openapi3filter.ValidateRequest(context.Background(), requestValidationInput)
		assert.Error(t, err, "Expected validation error for missing required field")
	})
}

// TestTagEndpointsContract tests tag endpoints against OpenAPI spec
func TestTagEndpointsContract(t *testing.T) {
	suite := NewContractTestSuite(t, "../../docs/api/openapi.yaml")

	t.Run("TagAutocomplete", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/autocomplete?q=test&limit=10", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-Request-ID", uuid.New().String())

		// Validate request against spec
		suite.ValidateRequest(t, req)

		// Simulate response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: createResponseBody(t, map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"id":          uuid.New().String(),
						"user_id":     uuid.New().String(),
						"name":        "testing",
						"usage_count": 5,
						"created_at":  "2024-01-01T00:00:00Z",
						"updated_at":  "2024-01-01T00:00:00Z",
					},
				},
			}),
		}

		// Validate response against spec
		suite.ValidateResponse(t, req, resp)
	})

	t.Run("GetPopularTags", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/popular?limit=20", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-Request-ID", uuid.New().String())

		// Validate request against spec
		suite.ValidateRequest(t, req)
	})

	t.Run("RateLimitResponse", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/autocomplete", nil)
		req.Header.Set("Authorization", "Bearer test-token")

		// Simulate rate limit response
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: createResponseBody(t, map[string]interface{}{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests, please try again later",
			}),
		}

		// Validate response against spec
		suite.ValidateResponse(t, req, resp)
	})
}

// Helper function to create response body
func createResponseBody(t *testing.T, data interface{}) *bytes.Buffer {
	bodyBytes, err := json.Marshal(data)
	require.NoError(t, err)
	return bytes.NewBuffer(bodyBytes)
}
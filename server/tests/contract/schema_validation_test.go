// +build contract

package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SchemaValidator validates HTTP requests and responses against OpenAPI schema
type SchemaValidator struct {
	doc    *openapi3.T
	router routers.Router
}

// NewSchemaValidator creates a new schema validator
func NewSchemaValidator(t *testing.T, specPath string) *SchemaValidator {
	// Load OpenAPI spec
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "Failed to load OpenAPI spec from %s", specPath)

	// Validate the spec
	err = doc.Validate(loader.Context)
	require.NoError(t, err, "OpenAPI spec validation failed")

	// Create router from spec
	router, err := gorillamux.NewRouter(doc)
	require.NoError(t, err, "Failed to create router from spec")

	return &SchemaValidator{
		doc:    doc,
		router: router,
	}
}

// ValidateRequest validates an HTTP request against the OpenAPI schema
func (sv *SchemaValidator) ValidateRequest(t *testing.T, req *http.Request) error {
	route, pathParams, err := sv.router.FindRoute(req)
	if err != nil {
		return err
	}

	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			AuthenticationFunc: func(c context.Context, input *openapi3filter.AuthenticationInput) error {
				// Skip auth validation in tests
				return nil
			},
		},
	}

	return openapi3filter.ValidateRequest(context.Background(), requestValidationInput)
}

// ValidateResponse validates an HTTP response against the OpenAPI schema
func (sv *SchemaValidator) ValidateResponse(t *testing.T, req *http.Request, resp *httptest.ResponseRecorder) error {
	route, pathParams, err := sv.router.FindRoute(req)
	if err != nil {
		return err
	}

	// Create a proper http.Response from ResponseRecorder
	httpResp := &http.Response{
		StatusCode: resp.Code,
		Header:     resp.Header(),
		Body:       io.NopCloser(bytes.NewBuffer(resp.Body.Bytes())),
	}

	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				AuthenticationFunc: func(c context.Context, input *openapi3filter.AuthenticationInput) error {
					return nil
				},
			},
		},
		Status: resp.Code,
		Header: resp.Header(),
	}

	// Important: Set the Body field with the response
	responseValidationInput.SetBodyBytes(resp.Body.Bytes())

	return openapi3filter.ValidateResponse(context.Background(), responseValidationInput)
}

// TestOpenAPISpecValidity tests that the OpenAPI spec itself is valid
func TestOpenAPISpecValidity(t *testing.T) {
	specPath := "../../docs/api/openapi.yaml"

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("OpenAPI spec not found, skipping validation")
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "Failed to load OpenAPI spec")

	err = doc.Validate(loader.Context)
	assert.NoError(t, err, "OpenAPI spec should be valid")

	// Verify spec metadata
	assert.NotEmpty(t, doc.Info.Title, "Spec should have a title")
	assert.NotEmpty(t, doc.Info.Version, "Spec should have a version")
	assert.NotEmpty(t, doc.Servers, "Spec should have at least one server")

	// Verify security schemes
	assert.NotNil(t, doc.Components.SecuritySchemes, "Spec should define security schemes")
	assert.Contains(t, doc.Components.SecuritySchemes, "BearerAuth", "Spec should have BearerAuth security scheme")

	// Verify common schemas are defined
	commonSchemas := []string{"Category", "Tag", "ErrorResponse"}
	for _, schemaName := range commonSchemas {
		assert.Contains(t, doc.Components.Schemas, schemaName, "Spec should define %s schema", schemaName)
	}
}

// TestSchemaSnapshotRegression tests that responses match schema snapshots
func TestSchemaSnapshotRegression(t *testing.T) {
	specPath := "../../docs/api/openapi.yaml"

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("OpenAPI spec not found, skipping validation")
	}

	validator := NewSchemaValidator(t, specPath)

	testCases := []struct {
		name           string
		method         string
		path           string
		requestBody    interface{}
		expectedStatus int
		snapshotFile   string
	}{
		{
			name:           "List Categories Response",
			method:         "GET",
			path:           "/api/v1/categories",
			expectedStatus: 200,
			snapshotFile:   "../snapshots/list_categories_response.json",
		},
		{
			name:   "Create Category Response",
			method: "POST",
			path:   "/api/v1/categories",
			requestBody: map[string]interface{}{
				"name":  "Test Category",
				"color": "#FF0000",
			},
			expectedStatus: 201,
			snapshotFile:   "../snapshots/create_category_response.json",
		},
		{
			name:           "Tag Autocomplete Response",
			method:         "GET",
			path:           "/api/v1/tags/autocomplete?q=test&limit=10",
			expectedStatus: 200,
			snapshotFile:   "../snapshots/tag_autocomplete_response.json",
		},
		{
			name:           "Daily Timeline Response",
			method:         "GET",
			path:           "/api/v1/timeline/daily?date=2024-01-15",
			expectedStatus: 200,
			snapshotFile:   "../snapshots/daily_timeline_response.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create request
			var reqBody io.Reader
			if tc.requestBody != nil {
				bodyBytes, err := json.Marshal(tc.requestBody)
				require.NoError(t, err)
				reqBody = bytes.NewBuffer(bodyBytes)
			}

			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			if tc.requestBody != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Authorization", "Bearer test-token")

			// Validate request against schema
			err := validator.ValidateRequest(t, req)
			assert.NoError(t, err, "Request should validate against schema")

			// Load or create snapshot
			snapshot := loadOrCreateSnapshot(t, tc.snapshotFile, tc.expectedStatus)

			// Validate snapshot against schema
			snapshotResp := httptest.NewRecorder()
			snapshotResp.Code = tc.expectedStatus
			snapshotResp.Header().Set("Content-Type", "application/json")
			snapshotResp.Body = bytes.NewBufferString(string(snapshot))

			err = validator.ValidateResponse(t, req, snapshotResp)
			assert.NoError(t, err, "Snapshot response should validate against schema")
		})
	}
}

// TestResponseHeaderValidation tests that response headers match OpenAPI spec
func TestResponseHeaderValidation(t *testing.T) {
	specPath := "../../docs/api/openapi.yaml"

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("OpenAPI spec not found, skipping validation")
	}

	validator := NewSchemaValidator(t, specPath)

	testCases := []struct {
		name            string
		method          string
		path            string
		expectedStatus  int
		requiredHeaders []string
	}{
		{
			name:           "Content-Type Header",
			method:         "GET",
			path:           "/api/v1/categories",
			expectedStatus: 200,
			requiredHeaders: []string{
				"Content-Type",
			},
		},
		{
			name:           "Rate Limit Headers on 429",
			method:         "GET",
			path:           "/api/v1/categories",
			expectedStatus: 429,
			requiredHeaders: []string{
				"X-RateLimit-Limit",
				"X-RateLimit-Remaining",
				"X-RateLimit-Reset",
			},
		},
		{
			name:           "Cache Headers on Timeline",
			method:         "GET",
			path:           "/api/v1/timeline/daily/enhanced?date=2024-01-15",
			expectedStatus: 200,
			requiredHeaders: []string{
				"Cache-Control",
				"ETag",
				"Last-Modified",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")

			// Create mock response with required headers
			resp := httptest.NewRecorder()
			resp.Code = tc.expectedStatus
			resp.Header().Set("Content-Type", "application/json")

			for _, header := range tc.requiredHeaders {
				switch header {
				case "X-RateLimit-Limit":
					resp.Header().Set(header, "100")
				case "X-RateLimit-Remaining":
					resp.Header().Set(header, "99")
				case "X-RateLimit-Reset":
					resp.Header().Set(header, "1640000000")
				case "Cache-Control":
					resp.Header().Set(header, "public, max-age=3600")
				case "ETag":
					resp.Header().Set(header, `"abc123"`)
				case "Last-Modified":
					resp.Header().Set(header, time.Now().Format(http.TimeFormat))
				}
			}

			// Add minimal valid response body
			var body interface{}
			if tc.expectedStatus == 200 {
				body = map[string]interface{}{
					"data": []interface{}{},
					"meta": map[string]interface{}{
						"total": 0,
						"count": 0,
					},
				}
			} else if tc.expectedStatus == 429 {
				body = map[string]interface{}{
					"error": "rate limit exceeded",
				}
			}

			bodyBytes, _ := json.Marshal(body)
			resp.Body = bytes.NewBuffer(bodyBytes)

			// Validate response
			err := validator.ValidateResponse(t, req, resp)
			assert.NoError(t, err, "Response with required headers should validate")
		})
	}
}

// TestErrorResponseSchemas tests that error responses follow schema
func TestErrorResponseSchemas(t *testing.T) {
	specPath := "../../docs/api/openapi.yaml"

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("OpenAPI spec not found, skipping validation")
	}

	validator := NewSchemaValidator(t, specPath)

	errorCases := []struct {
		name           string
		method         string
		path           string
		requestBody    interface{}
		expectedStatus int
		errorResponse  map[string]interface{}
	}{
		{
			name:           "400 Bad Request - Missing Required Field",
			method:         "POST",
			path:           "/api/v1/categories",
			requestBody:    map[string]interface{}{"color": "#FF0000"},
			expectedStatus: 400,
			errorResponse: map[string]interface{}{
				"error": "invalid request body",
			},
		},
		{
			name:           "401 Unauthorized",
			method:         "GET",
			path:           "/api/v1/categories",
			expectedStatus: 401,
			errorResponse: map[string]interface{}{
				"error": "unauthorized",
			},
		},
		{
			name:           "404 Not Found",
			method:         "GET",
			path:           "/api/v1/categories/00000000-0000-0000-0000-000000000000",
			expectedStatus: 404,
			errorResponse: map[string]interface{}{
				"error": "category not found",
			},
		},
		{
			name:           "429 Rate Limit Exceeded",
			method:         "GET",
			path:           "/api/v1/categories",
			expectedStatus: 429,
			errorResponse: map[string]interface{}{
				"error": "rate limit exceeded",
			},
		},
		{
			name:           "500 Internal Server Error",
			method:         "GET",
			path:           "/api/v1/categories",
			expectedStatus: 500,
			errorResponse: map[string]interface{}{
				"error": "internal server error",
			},
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			var reqBody io.Reader
			if tc.requestBody != nil {
				bodyBytes, err := json.Marshal(tc.requestBody)
				require.NoError(t, err)
				reqBody = bytes.NewBuffer(bodyBytes)
			}

			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			if tc.requestBody != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.expectedStatus != 401 {
				req.Header.Set("Authorization", "Bearer test-token")
			}

			resp := httptest.NewRecorder()
			resp.Code = tc.expectedStatus
			resp.Header().Set("Content-Type", "application/json")

			errorBytes, _ := json.Marshal(tc.errorResponse)
			resp.Body = bytes.NewBuffer(errorBytes)

			err := validator.ValidateResponse(t, req, resp)
			assert.NoError(t, err, "Error response should validate against schema")
		})
	}
}

// TestPaginationResponseSchema tests pagination metadata schemas
func TestPaginationResponseSchema(t *testing.T) {
	specPath := "../../docs/api/openapi.yaml"

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skip("OpenAPI spec not found, skipping validation")
	}

	validator := NewSchemaValidator(t, specPath)

	req := httptest.NewRequest("GET", "/api/v1/categories?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	resp := httptest.NewRecorder()
	resp.Code = 200
	resp.Header().Set("Content-Type", "application/json")

	paginatedResponse := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"id":         "550e8400-e29b-41d4-a716-446655440000",
				"user_id":    "550e8400-e29b-41d4-a716-446655440001",
				"name":       "Work",
				"color":      "#3498db",
				"icon":       "briefcase",
				"created_at": "2024-01-01T00:00:00Z",
				"updated_at": "2024-01-01T00:00:00Z",
			},
		},
		"meta": map[string]interface{}{
			"total":  100,
			"count":  10,
			"limit":  10,
			"offset": 0,
		},
	}

	bodyBytes, _ := json.Marshal(paginatedResponse)
	resp.Body = bytes.NewBuffer(bodyBytes)

	err := validator.ValidateResponse(t, req, resp)
	assert.NoError(t, err, "Paginated response should validate against schema")
}

// Helper functions

func loadOrCreateSnapshot(t *testing.T, snapshotFile string, status int) []byte {
	// Try to load existing snapshot
	if data, err := os.ReadFile(snapshotFile); err == nil {
		return data
	}

	// Create default snapshot based on status
	var snapshot interface{}
	if status == 200 {
		snapshot = map[string]interface{}{
			"data": []interface{}{},
			"meta": map[string]interface{}{
				"total": 0,
				"count": 0,
			},
		}
	} else if status == 201 {
		snapshot = map[string]interface{}{
			"id":         "550e8400-e29b-41d4-a716-446655440000",
			"user_id":    "550e8400-e29b-41d4-a716-446655440001",
			"name":       "Test",
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-01T00:00:00Z",
		}
	} else {
		snapshot = map[string]interface{}{
			"error": "test error",
		}
	}

	data, _ := json.MarshalIndent(snapshot, "", "  ")
	return data
}

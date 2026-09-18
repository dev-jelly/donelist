// +build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHealthEndpoints tests that basic health endpoints are working
func TestHealthEndpoints(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	t.Run("Health check endpoint", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/health", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)
		t.Logf("Health check response: %s", w.Body.String())
	})

	t.Run("Ready check endpoint", func(t *testing.T) {
		w := client.Request(http.MethodGet, "/health/ready", nil, nil)
		assert.Equal(t, http.StatusOK, w.Code)
		t.Logf("Ready check response: %s", w.Body.String())
	})
}

// TestUnauthorizedAccess tests that protected endpoints require authentication
func TestUnauthorizedAccess(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	testCases := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Get user profile without auth", http.MethodGet, "/api/v1/users/me", http.StatusUnauthorized},
		{"List checkins without auth", http.MethodGet, "/api/v1/checkins", http.StatusUnauthorized},
		{"List categories without auth", http.MethodGet, "/api/v1/categories", http.StatusUnauthorized},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Ensure no auth token is set
			client.AccessToken = ""

			w := client.Request(tc.method, tc.path, nil, nil)
			assert.Equal(t, tc.expected, w.Code, "Expected %d for %s %s", tc.expected, tc.method, tc.path)
		})
	}
}

// TestAPIRoutes tests that all API routes are properly configured
func TestAPIRoutes(t *testing.T) {
	client := NewE2ETestClient(t)
	defer client.Cleanup(t)

	t.Run("Auth routes exist", func(t *testing.T) {
		routes := []struct {
			method string
			path   string
		}{
			{http.MethodPost, "/api/v1/auth/register"},
			{http.MethodPost, "/api/v1/auth/login"},
			{http.MethodPost, "/api/v1/auth/refresh"},
			{http.MethodPost, "/api/v1/auth/logout"},
		}

		for _, route := range routes {
			w := client.Request(route.method, route.path, nil, nil)
			// Should not return 404
			assert.NotEqual(t, http.StatusNotFound, w.Code,
				"Route %s %s should exist", route.method, route.path)
		}
	})
}

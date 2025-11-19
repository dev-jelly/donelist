// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-jelly/donelist/internal/api/handlers"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCategoryFlow_Integration(t *testing.T) {
	server := setupTestServer(t)
	defer server.DB.TearDown(t)

	// Register and login to get access token
	accessToken := registerAndLogin(t, server, "category@example.com", "CategoryUser", "SecurePass123!")

	var createdCategoryID string

	t.Run("Create category successfully", func(t *testing.T) {
		color := "#FF5733"
		icon := "work"
		payload := map[string]interface{}{
			"name":  "Work",
			"color": color,
			"icon":  icon,
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "Work", response.Name)
		assert.NotNil(t, response.Color)
		assert.Equal(t, color, *response.Color)
		assert.NotNil(t, response.Icon)
		assert.Equal(t, icon, *response.Icon)
		assert.NotEmpty(t, response.ID)

		createdCategoryID = response.ID.String()
	})

	t.Run("Create category with duplicate name fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "Work",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "already exists")
	})

	t.Run("Create category without name fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"color": "#FF5733",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Create category with invalid color format fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "Invalid Color",
			"color": "FF5733", // Missing #
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "hex format")
	})

	t.Run("Create category with name too long fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "This is a very long category name that exceeds the maximum allowed length of 50 characters",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "less than 50 characters")
	})

	t.Run("List categories returns created category", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string][]category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		categories := response["categories"]
		assert.Len(t, categories, 1)
		assert.Equal(t, "Work", categories[0].Name)
	})

	t.Run("Get category by ID successfully", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories/"+createdCategoryID, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "Work", response.Name)
		assert.Equal(t, createdCategoryID, response.ID.String())
	})

	t.Run("Get category with invalid ID fails", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories/invalid-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "invalid category id")
	})

	t.Run("Get non-existent category fails", func(t *testing.T) {
		nonExistentID := uuid.New().String()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories/"+nonExistentID, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "not found")
	})

	t.Run("Update category successfully", func(t *testing.T) {
		newName := "Updated Work"
		newColor := "#00FF00"
		payload := map[string]interface{}{
			"name":  newName,
			"color": newColor,
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/categories/"+createdCategoryID, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, newName, response.Name)
		assert.NotNil(t, response.Color)
		assert.Equal(t, newColor, *response.Color)
	})

	t.Run("Update category with invalid color fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"color": "invalid",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/categories/"+createdCategoryID, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "hex format")
	})

	t.Run("Update category with empty name fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/categories/"+createdCategoryID, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "cannot be empty")
	})

	t.Run("Delete category successfully", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/categories/"+createdCategoryID, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "deleted successfully")
	})

	t.Run("List categories after deletion returns empty", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string][]category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		categories := response["categories"]
		assert.Len(t, categories, 0)
	})

	t.Run("Delete non-existent category fails", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/categories/"+createdCategoryID, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestCategoryOwnership_Integration(t *testing.T) {
	server := setupTestServer(t)
	defer server.DB.TearDown(t)

	// Register two users
	user1Token := registerAndLogin(t, server, "user1@example.com", "User1", "SecurePass123!")
	user2Token := registerAndLogin(t, server, "user2@example.com", "User2", "SecurePass123!")

	var user1CategoryID string

	t.Run("User1 creates a category", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "User1 Category",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+user1Token)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		user1CategoryID = response.ID.String()
	})

	t.Run("User2 cannot access User1's category", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories/"+user1CategoryID, nil)
		req.Header.Set("Authorization", "Bearer "+user2Token)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("User2 cannot update User1's category", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "Hacked Name",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/categories/"+user1CategoryID, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+user2Token)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("User2 cannot delete User1's category", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/categories/"+user1CategoryID, nil)
		req.Header.Set("Authorization", "Bearer "+user2Token)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("User2 doesn't see User1's categories in list", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories", nil)
		req.Header.Set("Authorization", "Bearer "+user2Token)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string][]category.Category
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		categories := response["categories"]
		assert.Len(t, categories, 0)
	})
}

func TestCategoryUnauthorized_Integration(t *testing.T) {
	server := setupTestServer(t)
	defer server.DB.TearDown(t)

	t.Run("Create category without auth fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "Unauthorized",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("List categories without auth fails", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories", nil)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Get category without auth fails", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/categories/"+uuid.New().String(), nil)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Update category without auth fails", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "Updated",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/categories/"+uuid.New().String(), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Delete category without auth fails", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/categories/"+uuid.New().String(), nil)

		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// Helper function to register and login a user
func registerAndLogin(t *testing.T, server *TestServer, email, username, password string) string {
	// Register
	payload := map[string]string{
		"email":    email,
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	// Login
	loginPayload := map[string]string{
		"email":    email,
		"password": password,
	}
	loginBody, _ := json.Marshal(loginPayload)

	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var loginResponse map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &loginResponse)
	require.NoError(t, err)

	accessToken, ok := loginResponse["access_token"].(string)
	require.True(t, ok, "access_token not found in login response")

	return accessToken
}

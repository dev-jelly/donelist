package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

// MockUserRepository is a mock implementation of user.Repository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestRequireAdmin(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		setupContext   func(*gin.Context)
		mockUser       *user.User
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful admin access",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			mockUser: &user.User{
				ID:    uuid.New(),
				Email: "admin@example.com",
				Role:  string(user.RoleAdmin),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "successful superadmin access",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			mockUser: &user.User{
				ID:    uuid.New(),
				Email: "superadmin@example.com",
				Role:  string(user.RoleSuperAdmin),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "deny regular user access",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			mockUser: &user.User{
				ID:    uuid.New(),
				Email: "user@example.com",
				Role:  string(user.RoleUser),
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "deny unauthenticated access",
			setupContext:   func(c *gin.Context) {},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserRepository)
			router := setupTestRouter()

			// Setup mock expectations
			if tt.mockUser != nil {
				mockRepo.On("GetByID", mock.Anything, mock.Anything).Return(tt.mockUser, tt.mockError)
			}

			// Add middleware and handler
			router.GET("/admin", func(c *gin.Context) {
				// Setup context first
				tt.setupContext(c)

				// Then call the middleware
				RequireAdmin(mockRepo, logger)(c)

				// If not aborted, return OK
				if !c.IsAborted() {
					c.Status(http.StatusOK)
				}
			})

			// Create test request
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.mockUser != nil {
				mockRepo.AssertExpectations(t)
			}
		})
	}
}

func TestRequireSuperAdmin(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		setupContext   func(*gin.Context)
		mockUser       *user.User
		mockError      error
		expectedStatus int
	}{
		{
			name: "successful superadmin access",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			mockUser: &user.User{
				ID:    uuid.New(),
				Email: "superadmin@example.com",
				Role:  string(user.RoleSuperAdmin),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "deny admin access to superadmin endpoint",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			mockUser: &user.User{
				ID:    uuid.New(),
				Email: "admin@example.com",
				Role:  string(user.RoleAdmin),
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "deny regular user access",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			mockUser: &user.User{
				ID:    uuid.New(),
				Email: "user@example.com",
				Role:  string(user.RoleUser),
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserRepository)
			router := setupTestRouter()

			// Setup mock expectations
			if tt.mockUser != nil {
				mockRepo.On("GetByID", mock.Anything, mock.Anything).Return(tt.mockUser, tt.mockError)
			}

			// Add middleware and handler
			router.GET("/superadmin", func(c *gin.Context) {
				// Setup context first
				tt.setupContext(c)

				// Then call the middleware
				RequireSuperAdmin(mockRepo, logger)(c)

				// If not aborted, return OK
				if !c.IsAborted() {
					c.Status(http.StatusOK)
				}
			})

			// Create test request
			req := httptest.NewRequest(http.MethodGet, "/superadmin", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.mockUser != nil {
				mockRepo.AssertExpectations(t)
			}
		})
	}
}

func TestRequireAdminFromToken(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		setupContext   func(*gin.Context)
		expectedStatus int
	}{
		{
			name: "successful admin access from token",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
				c.Set("user_role", string(user.RoleAdmin))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "successful superadmin access from token",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
				c.Set("user_role", string(user.RoleSuperAdmin))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "deny regular user access from token",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
				c.Set("user_role", string(user.RoleUser))
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "deny access without role in token",
			setupContext: func(c *gin.Context) {
				userID := uuid.New()
				c.Set("user_id", userID)
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()

			// Add middleware and handler
			router.GET("/admin", func(c *gin.Context) {
				// Setup context first
				tt.setupContext(c)

				// Then call the middleware
				RequireAdminFromToken(logger)(c)

				// If not aborted, return OK
				if !c.IsAborted() {
					c.Status(http.StatusOK)
				}
			})

			// Create test request
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetAdminUser(t *testing.T) {
	router := setupTestRouter()

	testUser := &user.User{
		ID:    uuid.New(),
		Email: "admin@example.com",
		Role:  string(user.RoleAdmin),
	}

	router.GET("/test", func(c *gin.Context) {
		c.Set("admin_user", testUser)

		adminUser, exists := GetAdminUser(c)
		assert.True(t, exists)
		assert.Equal(t, testUser.ID, adminUser.ID)
		assert.Equal(t, testUser.Email, adminUser.Email)

		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetUserRole(t *testing.T) {
	router := setupTestRouter()

	router.GET("/test", func(c *gin.Context) {
		c.Set("user_role", string(user.RoleAdmin))

		role, exists := GetUserRole(c)
		assert.True(t, exists)
		assert.Equal(t, string(user.RoleAdmin), role)

		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUserIsAdmin(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected bool
	}{
		{
			name:     "admin role is admin",
			role:     string(user.RoleAdmin),
			expected: true,
		},
		{
			name:     "superadmin role is admin",
			role:     string(user.RoleSuperAdmin),
			expected: true,
		},
		{
			name:     "user role is not admin",
			role:     string(user.RoleUser),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &user.User{
				ID:    uuid.New(),
				Email: "test@example.com",
				Role:  tt.role,
			}

			assert.Equal(t, tt.expected, u.IsAdmin())
		})
	}
}

func TestUserIsSuperAdmin(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected bool
	}{
		{
			name:     "superadmin role is superadmin",
			role:     string(user.RoleSuperAdmin),
			expected: true,
		},
		{
			name:     "admin role is not superadmin",
			role:     string(user.RoleAdmin),
			expected: false,
		},
		{
			name:     "user role is not superadmin",
			role:     string(user.RoleUser),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &user.User{
				ID:    uuid.New(),
				Email: "test@example.com",
				Role:  tt.role,
			}

			assert.Equal(t, tt.expected, u.IsSuperAdmin())
		})
	}
}

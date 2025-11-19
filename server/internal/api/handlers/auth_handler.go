package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/auth"
	"go.uber.org/zap"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *auth.Service
	logger      *zap.Logger
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authService *auth.Service, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		logger:      logger,
	}
}

// RegisterRequest represents registration request body
type RegisterRequest struct {
	Email       string  `json:"email" binding:"required,email"`
	Password    string  `json:"password" binding:"required,min=8,max=72"`
	DisplayName *string `json:"display_name" binding:"omitempty,max=100"`
}

// LoginRequest represents login request body
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest represents refresh token request body
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest represents logout request body
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Register handles user registration
// @Summary Register a new user
// @Description Create a new user account with email and password
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Registration details"
// @Success 201 {object} map[string]interface{} "user, access_token, refresh_token, expires_at"
// @Failure 400 {object} map[string]string "Invalid request or password validation error"
// @Failure 409 {object} map[string]string "Email already registered"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid registration request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	user, tokens, err := h.authService.Register(c.Request.Context(), auth.RegisterInput{
		Email:       strings.ToLower(strings.TrimSpace(req.Email)),
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		h.logger.Error("Registration failed", zap.Error(err))

		// Check for duplicate email
		if strings.Contains(err.Error(), "already registered") {
			c.JSON(http.StatusConflict, gin.H{
				"error": "email already registered",
			})
			return
		}

		// Check for password validation errors
		if strings.Contains(err.Error(), "invalid password") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to register user",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user":          user,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt,
	})
}

// Login handles user login
// @Summary Login user
// @Description Authenticate user with email and password, returns access and refresh tokens
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} map[string]interface{} "user, access_token, refresh_token, expires_at"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Invalid email or password"
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid login request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	user, tokens, err := h.authService.Login(c.Request.Context(), auth.LoginInput{
		Email:    strings.ToLower(strings.TrimSpace(req.Email)),
		Password: req.Password,
	})
	if err != nil {
		h.logger.Warn("Login failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid email or password",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":          user,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt,
	})
}

// Refresh handles access token refresh
// @Summary Refresh access token
// @Description Get a new access token using a valid refresh token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body RefreshRequest true "Refresh token"
// @Success 200 {object} map[string]interface{} "access_token, refresh_token, expires_at"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Invalid or expired refresh token"
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid refresh request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	tokens, err := h.authService.RefreshAccessToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.logger.Warn("Token refresh failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid or expired refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt,
	})
}

// Logout handles user logout (revokes refresh token)
// @Summary Logout user
// @Description Revoke the provided refresh token, logging the user out from this device
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body LogoutRequest true "Refresh token to revoke"
// @Success 200 {object} map[string]string "message"
// @Failure 400 {object} map[string]string "Invalid request or logout failed"
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid logout request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	if err := h.authService.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		h.logger.Warn("Logout failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to logout",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "logged out successfully",
	})
}

// LogoutAll handles logout from all devices
// @Summary Logout from all devices
// @Description Revoke all refresh tokens for the current user, logging them out from all devices
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]string "message"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to logout from all devices"
// @Router /auth/logout-all [post]
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	if err := h.authService.LogoutAll(c.Request.Context(), userID); err != nil {
		h.logger.Error("Logout all failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to logout from all devices",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "logged out from all devices successfully",
	})
}

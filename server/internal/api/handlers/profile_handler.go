package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/profile"
	"go.uber.org/zap"
)

// ProfileHandler handles profile and settings endpoints
type ProfileHandler struct {
	profileService *profile.Service
	logger         *zap.Logger
}

// NewProfileHandler creates a new profile handler
func NewProfileHandler(profileService *profile.Service, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		profileService: profileService,
		logger:         logger,
	}
}

// UpdateProfileRequest represents profile update request body
type UpdateProfileRequest struct {
	Bio                  *string `json:"bio" binding:"omitempty,max=1000"`
	AvatarURL            *string `json:"avatar_url" binding:"omitempty,max=500"`
	Timezone             *string `json:"timezone" binding:"omitempty,max=100"`
	TimezoneAutoDetected *bool   `json:"timezone_auto_detected"`
	Locale               *string `json:"locale" binding:"omitempty,max=10"`
	Version              int     `json:"version" binding:"required,min=1"`
}

// UpdateSettingsRequest represents settings update request body
type UpdateSettingsRequest struct {
	Theme        *string `json:"theme" binding:"omitempty,oneof=light dark system"`
	Language     *string `json:"language" binding:"omitempty,max=10"`
	DateFormat   *string `json:"date_format" binding:"omitempty,max=20"`
	TimeFormat   *string `json:"time_format" binding:"omitempty,oneof=12h 24h"`
	WeekStartDay *int    `json:"week_start_day" binding:"omitempty,min=0,max=6"`
	Version      int     `json:"version" binding:"required,min=1"`
}

// UpdateNotificationSettingsRequest represents notification settings update request body
type UpdateNotificationSettingsRequest struct {
	EmailNotifications     *bool      `json:"email_notifications"`
	PushNotifications      *bool      `json:"push_notifications"`
	CheckinReminders       *bool      `json:"checkin_reminders"`
	ReminderIntervalMinutes *int      `json:"reminder_interval_minutes" binding:"omitempty,oneof=15 30 45 60 90 120 180 240"`
	DNDEnabled             *bool      `json:"dnd_enabled"`
	DNDStartTime           *time.Time `json:"dnd_start_time"`
	DNDEndTime             *time.Time `json:"dnd_end_time"`
	DNDDays                *[]int     `json:"dnd_days"`
	Version                int        `json:"version" binding:"required,min=1"`
}

// UpdateDataRetentionSettingsRequest represents data retention settings update request body
type UpdateDataRetentionSettingsRequest struct {
	AutoDeleteEnabled         *bool `json:"auto_delete_enabled"`
	RetentionDays             *int  `json:"retention_days" binding:"omitempty,min=1"`
	DeleteAfterInactivityDays *int  `json:"delete_after_inactivity_days" binding:"omitempty,min=30"`
	Version                   int   `json:"version" binding:"required,min=1"`
}

// GetProfile retrieves the current user's profile
// @Summary Get current user profile
// @Description Retrieve the authenticated user's profile information
// @Tags Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} profile.Profile "User profile"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Profile not found"
// @Router /profile [get]
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userProfile, err := h.profileService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get profile", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	c.JSON(http.StatusOK, userProfile)
}

// UpdateProfile updates the current user's profile
// @Summary Update current user profile
// @Description Update the authenticated user's profile information with optimistic locking
// @Tags Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateProfileRequest true "Profile update details"
// @Success 200 {object} profile.Profile "Updated profile"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]string "Version conflict"
// @Failure 500 {object} map[string]string "Failed to update profile"
// @Router /profile [patch]
func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid profile update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updatedProfile, err := h.profileService.UpdateProfile(c.Request.Context(), userID, profile.UpdateProfileInput{
		Bio:                  req.Bio,
		AvatarURL:            req.AvatarURL,
		Timezone:             req.Timezone,
		TimezoneAutoDetected: req.TimezoneAutoDetected,
		Locale:               req.Locale,
		Version:              req.Version,
	})
	if err != nil {
		if err.Error() == "profile was updated by another process, please refresh and try again" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to update profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, updatedProfile)
}

// GetSettings retrieves the current user's settings
// @Summary Get current user settings
// @Description Retrieve the authenticated user's UI/UX settings
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} profile.Settings "User settings"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Settings not found"
// @Router /settings [get]
func (h *ProfileHandler) GetSettings(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	settings, err := h.profileService.GetSettings(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get settings", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "settings not found"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateSettings updates the current user's settings
// @Summary Update current user settings
// @Description Update the authenticated user's UI/UX settings with optimistic locking
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateSettingsRequest true "Settings update details"
// @Success 200 {object} profile.Settings "Updated settings"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]string "Version conflict"
// @Failure 500 {object} map[string]string "Failed to update settings"
// @Router /settings [patch]
func (h *ProfileHandler) UpdateSettings(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid settings update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updatedSettings, err := h.profileService.UpdateSettings(c.Request.Context(), userID, profile.UpdateSettingsInput{
		Theme:        req.Theme,
		Language:     req.Language,
		DateFormat:   req.DateFormat,
		TimeFormat:   req.TimeFormat,
		WeekStartDay: req.WeekStartDay,
		Version:      req.Version,
	})
	if err != nil {
		if err.Error() == "settings were updated by another process, please refresh and try again" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to update settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	c.JSON(http.StatusOK, updatedSettings)
}

// GetNotificationSettings retrieves the current user's notification settings
// @Summary Get current user notification settings
// @Description Retrieve the authenticated user's notification preferences
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} profile.NotificationSettings "Notification settings"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Notification settings not found"
// @Router /settings/notifications [get]
func (h *ProfileHandler) GetNotificationSettings(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	settings, err := h.profileService.GetNotificationSettings(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get notification settings", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "notification settings not found"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateNotificationSettings updates the current user's notification settings
// @Summary Update current user notification settings
// @Description Update the authenticated user's notification preferences with optimistic locking
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateNotificationSettingsRequest true "Notification settings update details"
// @Success 200 {object} profile.NotificationSettings "Updated notification settings"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]string "Version conflict"
// @Failure 500 {object} map[string]string "Failed to update notification settings"
// @Router /settings/notifications [patch]
func (h *ProfileHandler) UpdateNotificationSettings(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateNotificationSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid notification settings update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updatedSettings, err := h.profileService.UpdateNotificationSettings(c.Request.Context(), userID, profile.UpdateNotificationSettingsInput{
		EmailNotifications:     req.EmailNotifications,
		PushNotifications:      req.PushNotifications,
		CheckinReminders:       req.CheckinReminders,
		ReminderIntervalMinutes: req.ReminderIntervalMinutes,
		DNDEnabled:             req.DNDEnabled,
		DNDStartTime:           req.DNDStartTime,
		DNDEndTime:             req.DNDEndTime,
		DNDDays:                req.DNDDays,
		Version:                req.Version,
	})
	if err != nil {
		if err.Error() == "notification settings were updated by another process, please refresh and try again" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to update notification settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification settings"})
		return
	}

	c.JSON(http.StatusOK, updatedSettings)
}

// GetDataRetentionSettings retrieves the current user's data retention settings
// @Summary Get current user data retention settings
// @Description Retrieve the authenticated user's data retention policies
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} profile.DataRetentionSettings "Data retention settings"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Data retention settings not found"
// @Router /settings/data-retention [get]
func (h *ProfileHandler) GetDataRetentionSettings(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	settings, err := h.profileService.GetDataRetentionSettings(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get data retention settings", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "data retention settings not found"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateDataRetentionSettings updates the current user's data retention settings
// @Summary Update current user data retention settings
// @Description Update the authenticated user's data retention policies with optimistic locking
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateDataRetentionSettingsRequest true "Data retention settings update details"
// @Success 200 {object} profile.DataRetentionSettings "Updated data retention settings"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 409 {object} map[string]string "Version conflict"
// @Failure 500 {object} map[string]string "Failed to update data retention settings"
// @Router /settings/data-retention [patch]
func (h *ProfileHandler) UpdateDataRetentionSettings(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateDataRetentionSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid data retention settings update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updatedSettings, err := h.profileService.UpdateDataRetentionSettings(c.Request.Context(), userID, profile.UpdateDataRetentionSettingsInput{
		AutoDeleteEnabled:         req.AutoDeleteEnabled,
		RetentionDays:             req.RetentionDays,
		DeleteAfterInactivityDays: req.DeleteAfterInactivityDays,
		Version:                   req.Version,
	})
	if err != nil {
		if err.Error() == "data retention settings were updated by another process, please refresh and try again" {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("Failed to update data retention settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update data retention settings"})
		return
	}

	c.JSON(http.StatusOK, updatedSettings)
}

// GetAll retrieves all profile and settings for the current user
// @Summary Get all profile and settings
// @Description Retrieve all profile and settings information for the authenticated user
// @Tags Profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} profile.ProfileWithSettings "Complete profile and settings"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Failed to retrieve profile and settings"
// @Router /profile/all [get]
func (h *ProfileHandler) GetAll(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	result, err := h.profileService.GetAll(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get all profile settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve profile and settings"})
		return
	}

	c.JSON(http.StatusOK, result)
}

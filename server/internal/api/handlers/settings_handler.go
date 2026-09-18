package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/export"
	"github.com/dev-jelly/donelist/internal/profile"
	"go.uber.org/zap"
)

// Context keys for user information
type contextKey string

const (
	userIDContextKey    contextKey = "userID"
	userEmailContextKey contextKey = "userEmail"
)

// getUserIDFromContext extracts user ID from context
func getUserIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(userIDContextKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

// getUserEmailFromContext extracts user email from context
func getUserEmailFromContext(ctx context.Context) string {
	if email, ok := ctx.Value(userEmailContextKey).(string); ok {
		return email
	}
	return ""
}

// respondWithJSON writes a JSON response
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

// respondWithError writes an error response
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

// SettingsHandler handles user settings and account management endpoints
type SettingsHandler struct {
	twoFactorService   *auth.TwoFactorService
	accountService     *auth.AccountManagementService
	profileService     *profile.EnhancedProfileService
	exportService      *export.DataExportService
	logger             *zap.Logger
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(
	twoFactorService *auth.TwoFactorService,
	accountService *auth.AccountManagementService,
	profileService *profile.EnhancedProfileService,
	exportService *export.DataExportService,
	logger *zap.Logger,
) *SettingsHandler {
	return &SettingsHandler{
		twoFactorService: twoFactorService,
		accountService:   accountService,
		profileService:   profileService,
		exportService:    exportService,
		logger:           logger,
	}
}

// RegisterRoutes registers all settings-related routes
func (h *SettingsHandler) RegisterRoutes(r *mux.Router) {
	// 2FA routes
	r.HandleFunc("/settings/2fa/setup", h.Setup2FA).Methods("POST")
	r.HandleFunc("/settings/2fa/verify", h.Verify2FA).Methods("POST")
	r.HandleFunc("/settings/2fa/disable", h.Disable2FA).Methods("POST")
	r.HandleFunc("/settings/2fa/backup-codes", h.RegenerateBackupCodes).Methods("POST")

	// Password management
	r.HandleFunc("/settings/password/change", h.ChangePassword).Methods("POST")
	r.HandleFunc("/auth/password/reset", h.InitiatePasswordReset).Methods("POST")
	r.HandleFunc("/auth/password/reset/complete", h.CompletePasswordReset).Methods("POST")

	// Account management
	r.HandleFunc("/settings/account/recovery-email", h.UpdateRecoveryEmail).Methods("PUT")
	r.HandleFunc("/settings/account/delete", h.DeleteAccount).Methods("POST")
	r.HandleFunc("/settings/account/delete/cancel", h.CancelAccountDeletion).Methods("POST")

	// Data export
	r.HandleFunc("/settings/export", h.RequestDataExport).Methods("POST")
	r.HandleFunc("/settings/export/{id}", h.GetExportStatus).Methods("GET")
	r.HandleFunc("/settings/export/history", h.GetExportHistory).Methods("GET")

	// Profile privacy
	r.HandleFunc("/settings/profile/privacy", h.UpdateProfilePrivacy).Methods("PUT")
	r.HandleFunc("/settings/profile/avatar", h.UploadAvatar).Methods("POST")
	r.HandleFunc("/settings/profile/avatar", h.RemoveAvatar).Methods("DELETE")

	// Security events
	r.HandleFunc("/settings/security/events", h.GetSecurityEvents).Methods("GET")
}

// Setup2FA initiates 2FA setup
// @Summary Setup Two-Factor Authentication
// @Tags Settings
// @Accept json
// @Produce json
// @Success 200 {object} auth.TwoFactorSetup
// @Router /settings/2fa/setup [post]
func (h *SettingsHandler) Setup2FA(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())
	email := getUserEmailFromContext(r.Context())

	setup, err := h.twoFactorService.GenerateTOTPSecret(r.Context(), userID, email)
	if err != nil {
		h.logger.Error("Failed to generate 2FA secret", zap.Error(err))
		respondWithError(w, http.StatusInternalServerError, "Failed to setup 2FA")
		return
	}

	respondWithJSON(w, http.StatusOK, setup)
}

// Verify2FA verifies and enables 2FA
// @Summary Verify and Enable 2FA
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body map[string]string true "Verification code"
// @Success 200 {object} map[string]bool
// @Router /settings/2fa/verify [post]
func (h *SettingsHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := h.twoFactorService.VerifyAndEnableTOTP(r.Context(), userID, req.Code); err != nil {
		h.logger.Warn("Failed to verify 2FA code", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// Disable2FA disables 2FA
// @Summary Disable Two-Factor Authentication
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body map[string]string true "Current password"
// @Success 200 {object} map[string]bool
// @Router /settings/2fa/disable [post]
func (h *SettingsHandler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := h.twoFactorService.DisableTOTP(r.Context(), userID, req.Password); err != nil {
		h.logger.Warn("Failed to disable 2FA", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// RegenerateBackupCodes regenerates 2FA backup codes
// @Summary Regenerate 2FA Backup Codes
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body map[string]string true "Current password"
// @Success 200 {object} map[string][]string
// @Router /settings/2fa/backup-codes [post]
func (h *SettingsHandler) RegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	codes, err := h.twoFactorService.RegenerateBackupCodes(r.Context(), userID, req.Password)
	if err != nil {
		h.logger.Warn("Failed to regenerate backup codes", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string][]string{"backup_codes": codes})
}

// ChangePassword changes user password
// @Summary Change Password
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body auth.ChangePasswordRequest true "Password change request"
// @Success 200 {object} map[string]bool
// @Router /settings/password/change [post]
func (h *SettingsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req auth.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := h.accountService.ChangePassword(r.Context(), userID, req); err != nil {
		h.logger.Warn("Failed to change password", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// InitiatePasswordReset initiates password reset
// @Summary Initiate Password Reset
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.ResetPasswordRequest true "Reset request"
// @Success 200 {object} map[string]string
// @Router /auth/password/reset [post]
func (h *SettingsHandler) InitiatePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req auth.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Always return success to prevent email enumeration
	if err := h.accountService.InitiatePasswordReset(r.Context(), req); err != nil {
		h.logger.Error("Failed to initiate password reset", zap.Error(err))
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "If the email exists, a reset link has been sent",
	})
}

// CompletePasswordReset completes password reset
// @Summary Complete Password Reset
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body map[string]string true "Token and new password"
// @Success 200 {object} map[string]bool
// @Router /auth/password/reset/complete [post]
func (h *SettingsHandler) CompletePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := h.accountService.CompletePasswordReset(r.Context(), req.Token, req.NewPassword); err != nil {
		h.logger.Warn("Failed to complete password reset", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// UpdateRecoveryEmail updates recovery email
// @Summary Update Recovery Email
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body map[string]string true "Email and password"
// @Success 200 {object} map[string]bool
// @Router /settings/account/recovery-email [put]
func (h *SettingsHandler) UpdateRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := h.accountService.UpdateRecoveryEmail(r.Context(), userID, req.Email, req.Password); err != nil {
		h.logger.Warn("Failed to update recovery email", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// DeleteAccount initiates account deletion
// @Summary Delete Account
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body map[string]string true "Password and reason"
// @Success 200 {object} map[string]string
// @Router /settings/account/delete [post]
func (h *SettingsHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req struct {
		Password string `json:"password"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if err := h.accountService.DeleteAccount(r.Context(), userID, req.Password, req.Reason); err != nil {
		h.logger.Warn("Failed to initiate account deletion", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Account deletion scheduled. You have 30 days to cancel this request.",
	})
}

// CancelAccountDeletion cancels account deletion
// @Summary Cancel Account Deletion
// @Tags Settings
// @Accept json
// @Produce json
// @Success 200 {object} map[string]bool
// @Router /settings/account/delete/cancel [post]
func (h *SettingsHandler) CancelAccountDeletion(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	if err := h.accountService.CancelAccountDeletion(r.Context(), userID); err != nil {
		h.logger.Warn("Failed to cancel account deletion", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// RequestDataExport requests a data export
// @Summary Request Data Export
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body map[string]string true "Export format"
// @Success 200 {object} export.DataExportRequest
// @Router /settings/export [post]
func (h *SettingsHandler) RequestDataExport(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req struct {
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	exportReq, err := h.exportService.RequestDataExport(r.Context(), userID, export.ExportFormat(req.Format))
	if err != nil {
		h.logger.Error("Failed to request data export", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, exportReq)
}

// GetExportStatus gets export request status
// @Summary Get Export Status
// @Tags Settings
// @Accept json
// @Produce json
// @Param id path string true "Export request ID"
// @Success 200 {object} export.DataExportRequest
// @Router /settings/export/{id} [get]
func (h *SettingsHandler) GetExportStatus(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())
	vars := mux.Vars(r)
	requestID, err := uuid.Parse(vars["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID")
		return
	}

	exportReq, err := h.exportService.GetExportRequest(r.Context(), requestID, userID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Export request not found")
		return
	}

	respondWithJSON(w, http.StatusOK, exportReq)
}

// GetExportHistory gets export history
// @Summary Get Export History
// @Tags Settings
// @Accept json
// @Produce json
// @Success 200 {object} []export.DataExportRequest
// @Router /settings/export/history [get]
func (h *SettingsHandler) GetExportHistory(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	history, err := h.exportService.GetExportHistory(r.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get export history", zap.Error(err))
		respondWithError(w, http.StatusInternalServerError, "Failed to get export history")
		return
	}

	respondWithJSON(w, http.StatusOK, history)
}

// UpdateProfilePrivacy updates profile privacy settings
// @Summary Update Profile Privacy
// @Tags Settings
// @Accept json
// @Produce json
// @Param request body profile.ProfileUpdateRequest true "Privacy settings"
// @Success 200 {object} profile.EnhancedProfile
// @Router /settings/profile/privacy [put]
func (h *SettingsHandler) UpdateProfilePrivacy(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	var req profile.ProfileUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	updatedProfile, err := h.profileService.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("Failed to update profile privacy", zap.Error(err))
		respondWithError(w, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	respondWithJSON(w, http.StatusOK, updatedProfile)
}

// UploadAvatar handles avatar upload
// @Summary Upload Avatar
// @Tags Settings
// @Accept multipart/form-data
// @Produce json
// @Param avatar formData file true "Avatar image"
// @Success 200 {object} map[string]string
// @Router /settings/profile/avatar [post]
func (h *SettingsHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to get avatar file")
		return
	}
	defer file.Close()

	avatarURL, err := h.profileService.UploadAvatar(r.Context(), userID, file, header)
	if err != nil {
		h.logger.Error("Failed to upload avatar", zap.Error(err))
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"avatar_url": *avatarURL})
}

// RemoveAvatar removes user avatar
// @Summary Remove Avatar
// @Tags Settings
// @Accept json
// @Produce json
// @Success 200 {object} map[string]bool
// @Router /settings/profile/avatar [delete]
func (h *SettingsHandler) RemoveAvatar(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	if err := h.profileService.RemoveAvatar(r.Context(), userID); err != nil {
		h.logger.Error("Failed to remove avatar", zap.Error(err))
		respondWithError(w, http.StatusInternalServerError, "Failed to remove avatar")
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// GetSecurityEvents gets security events
// @Summary Get Security Events
// @Tags Settings
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(20)
// @Success 200 {object} []auth.SecurityEvent
// @Router /settings/security/events [get]
func (h *SettingsHandler) GetSecurityEvents(w http.ResponseWriter, r *http.Request) {
	userID := getUserIDFromContext(r.Context())

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		// Parse limit from query parameter
		// For simplicity, using default if parsing fails
	}

	events, err := h.twoFactorService.GetSecurityEvents(r.Context(), userID, limit)
	if err != nil {
		h.logger.Error("Failed to get security events", zap.Error(err))
		respondWithError(w, http.StatusInternalServerError, "Failed to get security events")
		return
	}

	respondWithJSON(w, http.StatusOK, events)
}
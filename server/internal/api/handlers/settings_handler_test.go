package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/export"
	"github.com/dev-jelly/donelist/internal/profile"
	"go.uber.org/zap"
)

// Mock services for testing
type MockTwoFactorService struct {
	mock.Mock
}

func (m *MockTwoFactorService) GenerateTOTPSecret(ctx context.Context, userID uuid.UUID, email string) (*auth.TwoFactorSetup, error) {
	args := m.Called(ctx, userID, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*auth.TwoFactorSetup), args.Error(1)
}

func (m *MockTwoFactorService) VerifyAndEnableTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	args := m.Called(ctx, userID, code)
	return args.Error(0)
}

func (m *MockTwoFactorService) DisableTOTP(ctx context.Context, userID uuid.UUID, password string) error {
	args := m.Called(ctx, userID, password)
	return args.Error(0)
}

func (m *MockTwoFactorService) GetSecurityEvents(ctx context.Context, userID uuid.UUID, limit int) ([]auth.SecurityEvent, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]auth.SecurityEvent), args.Error(1)
}

type MockAccountManagementService struct {
	mock.Mock
}

func (m *MockAccountManagementService) ChangePassword(ctx context.Context, userID uuid.UUID, req auth.ChangePasswordRequest) error {
	args := m.Called(ctx, userID, req)
	return args.Error(0)
}

type MockProfileService struct {
	mock.Mock
}

func (m *MockProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, req profile.ProfileUpdateRequest) (*profile.EnhancedProfile, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*profile.EnhancedProfile), args.Error(1)
}

func (m *MockProfileService) UploadAvatar(ctx context.Context, userID uuid.UUID, file multipart.File, header *multipart.FileHeader) (*string, error) {
	args := m.Called(ctx, userID, file, header)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*string), args.Error(1)
}

type MockExportService struct {
	mock.Mock
}

func (m *MockExportService) RequestDataExport(ctx context.Context, userID uuid.UUID, format export.ExportFormat) (*export.DataExportRequest, error) {
	args := m.Called(ctx, userID, format)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*export.DataExportRequest), args.Error(1)
}

// Test setup helper
func setupTestHandler() (*SettingsHandler, *MockTwoFactorService, *MockAccountManagementService, *MockProfileService, *MockExportService) {
	logger := zap.NewNop()
	mockTwoFactor := new(MockTwoFactorService)
	mockAccount := new(MockAccountManagementService)
	mockProfile := new(MockProfileService)
	mockExport := new(MockExportService)

	handler := &SettingsHandler{
		twoFactorService: mockTwoFactor,
		accountService:   mockAccount,
		profileService:   mockProfile,
		exportService:    mockExport,
		logger:          logger,
	}

	return handler, mockTwoFactor, mockAccount, mockProfile, mockExport
}

// Helper to create authenticated request
func createAuthenticatedRequest(method, path string, body interface{}) *http.Request {
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// Add user context
	userID := uuid.New()
	ctx := context.WithValue(req.Context(), "userID", userID)
	ctx = context.WithValue(ctx, "userEmail", "test@example.com")
	return req.WithContext(ctx)
}

// Test 2FA Setup
func TestSetup2FA(t *testing.T) {
	handler, mockTwoFactor, _, _, _ := setupTestHandler()

	t.Run("Successful 2FA Setup", func(t *testing.T) {
		expectedSetup := &auth.TwoFactorSetup{
			Secret:      "TESTSECRET123",
			QRCode:      []byte("qr_code_data"),
			BackupCodes: []string{"CODE1", "CODE2", "CODE3"},
			RecoveryURL: "otpauth://totp/DoneList:test@example.com",
		}

		userID := uuid.New()
		mockTwoFactor.On("GenerateTOTPSecret", mock.Anything, userID, "test@example.com").
			Return(expectedSetup, nil)

		req := createAuthenticatedRequest("POST", "/settings/2fa/setup", nil)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.Setup2FA(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response auth.TwoFactorSetup
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, expectedSetup.Secret, response.Secret)
		assert.Equal(t, expectedSetup.BackupCodes, response.BackupCodes)
		mockTwoFactor.AssertExpectations(t)
	})

	t.Run("2FA Already Enabled", func(t *testing.T) {
		userID := uuid.New()
		mockTwoFactor.On("GenerateTOTPSecret", mock.Anything, userID, "test@example.com").
			Return(nil, assert.AnError)

		req := createAuthenticatedRequest("POST", "/settings/2fa/setup", nil)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.Setup2FA(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		mockTwoFactor.AssertExpectations(t)
	})
}

// Test 2FA Verification
func TestVerify2FA(t *testing.T) {
	handler, mockTwoFactor, _, _, _ := setupTestHandler()

	t.Run("Valid Verification Code", func(t *testing.T) {
		userID := uuid.New()
		mockTwoFactor.On("VerifyAndEnableTOTP", mock.Anything, userID, "123456").
			Return(nil)

		body := map[string]string{"code": "123456"}
		req := createAuthenticatedRequest("POST", "/settings/2fa/verify", body)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.Verify2FA(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response map[string]bool
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"])
		mockTwoFactor.AssertExpectations(t)
	})

	t.Run("Invalid Verification Code", func(t *testing.T) {
		userID := uuid.New()
		mockTwoFactor.On("VerifyAndEnableTOTP", mock.Anything, userID, "000000").
			Return(assert.AnError)

		body := map[string]string{"code": "000000"}
		req := createAuthenticatedRequest("POST", "/settings/2fa/verify", body)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.Verify2FA(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		mockTwoFactor.AssertExpectations(t)
	})
}

// Test Password Change
func TestChangePassword(t *testing.T) {
	handler, _, mockAccount, _, _ := setupTestHandler()

	t.Run("Successful Password Change", func(t *testing.T) {
		userID := uuid.New()
		changeReq := auth.ChangePasswordRequest{
			CurrentPassword: "oldPassword123",
			NewPassword:     "newPassword456!",
		}

		mockAccount.On("ChangePassword", mock.Anything, userID, changeReq).
			Return(nil)

		req := createAuthenticatedRequest("POST", "/settings/password/change", changeReq)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ChangePassword(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response map[string]bool
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"])
		mockAccount.AssertExpectations(t)
	})

	t.Run("Incorrect Current Password", func(t *testing.T) {
		userID := uuid.New()
		changeReq := auth.ChangePasswordRequest{
			CurrentPassword: "wrongPassword",
			NewPassword:     "newPassword456!",
		}

		mockAccount.On("ChangePassword", mock.Anything, userID, changeReq).
			Return(assert.AnError)

		req := createAuthenticatedRequest("POST", "/settings/password/change", changeReq)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.ChangePassword(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		mockAccount.AssertExpectations(t)
	})
}

// Test Profile Privacy Update
func TestUpdateProfilePrivacy(t *testing.T) {
	handler, _, _, mockProfile, _ := setupTestHandler()

	t.Run("Update Privacy Settings", func(t *testing.T) {
		userID := uuid.New()
		privacyLevel := profile.PrivacyPublic
		showEmail := false
		showActivity := true
		searchable := true

		updateReq := profile.ProfileUpdateRequest{
			PrivacyLevel: &privacyLevel,
			ShowEmail:    &showEmail,
			ShowActivity: &showActivity,
			Searchable:   &searchable,
		}

		expectedProfile := &profile.EnhancedProfile{
			Profile: profile.Profile{
				ID:        uuid.New(),
				UserID:    userID,
				Timezone:  "UTC",
				Locale:    "en-US",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Version:   2,
			},
			PrivacyLevel: privacyLevel,
			ShowEmail:    showEmail,
			ShowActivity: showActivity,
			Searchable:   searchable,
		}

		mockProfile.On("UpdateProfile", mock.Anything, userID, updateReq).
			Return(expectedProfile, nil)

		req := createAuthenticatedRequest("PUT", "/settings/profile/privacy", updateReq)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.UpdateProfilePrivacy(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response profile.EnhancedProfile
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, expectedProfile.PrivacyLevel, response.PrivacyLevel)
		assert.Equal(t, expectedProfile.ShowEmail, response.ShowEmail)
		mockProfile.AssertExpectations(t)
	})
}

// Test Data Export Request
func TestRequestDataExport(t *testing.T) {
	handler, _, _, _, mockExport := setupTestHandler()

	t.Run("Request JSON Export", func(t *testing.T) {
		userID := uuid.New()
		exportFormat := export.FormatJSON

		expectedRequest := &export.DataExportRequest{
			ID:          uuid.New(),
			UserID:      userID,
			Status:      "pending",
			Format:      exportFormat,
			RequestedAt: time.Now(),
		}

		mockExport.On("RequestDataExport", mock.Anything, userID, exportFormat).
			Return(expectedRequest, nil)

		body := map[string]string{"format": "json"}
		req := createAuthenticatedRequest("POST", "/settings/export", body)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.RequestDataExport(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response export.DataExportRequest
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "pending", response.Status)
		assert.Equal(t, exportFormat, response.Format)
		mockExport.AssertExpectations(t)
	})

	t.Run("Export Already In Progress", func(t *testing.T) {
		userID := uuid.New()
		exportFormat := export.FormatJSON

		mockExport.On("RequestDataExport", mock.Anything, userID, exportFormat).
			Return(nil, assert.AnError)

		body := map[string]string{"format": "json"}
		req := createAuthenticatedRequest("POST", "/settings/export", body)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.RequestDataExport(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		mockExport.AssertExpectations(t)
	})
}

// Test Security Events Retrieval
func TestGetSecurityEvents(t *testing.T) {
	handler, mockTwoFactor, _, _, _ := setupTestHandler()

	t.Run("Get Recent Security Events", func(t *testing.T) {
		userID := uuid.New()
		expectedEvents := []auth.SecurityEvent{
			{
				ID:        uuid.New(),
				UserID:    userID,
				EventType: "login",
				Success:   true,
				CreatedAt: time.Now().Add(-1 * time.Hour),
			},
			{
				ID:        uuid.New(),
				UserID:    userID,
				EventType: "password_changed",
				Success:   true,
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
		}

		mockTwoFactor.On("GetSecurityEvents", mock.Anything, userID, 20).
			Return(expectedEvents, nil)

		req := createAuthenticatedRequest("GET", "/settings/security/events", nil)
		ctx := context.WithValue(req.Context(), "userID", userID)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.GetSecurityEvents(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response []auth.SecurityEvent
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response, 2)
		assert.Equal(t, "login", response[0].EventType)
		assert.Equal(t, "password_changed", response[1].EventType)
		mockTwoFactor.AssertExpectations(t)
	})
}

// Integration test for complete 2FA flow
func TestComplete2FAFlow(t *testing.T) {
	// This would be an integration test with actual services
	// For now, we'll skip this as it requires more setup
	t.Skip("Integration test - requires full service setup")
}

// Test helper functions
func getUserIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value("userID").(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func getUserEmailFromContext(ctx context.Context) string {
	if email, ok := ctx.Value("userEmail").(string); ok {
		return email
	}
	return ""
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}
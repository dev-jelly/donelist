package notification

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// MockDNDOverrideRepository is a mock implementation of DNDOverrideRepository
type MockDNDOverrideRepository struct {
	mock.Mock
}

func (m *MockDNDOverrideRepository) CreateOverride(ctx context.Context, override *DNDOverride) error {
	args := m.Called(ctx, override)
	return args.Error(0)
}

func (m *MockDNDOverrideRepository) GetActiveOverride(ctx context.Context, userID uuid.UUID, checkTime time.Time) (*DNDOverride, error) {
	args := m.Called(ctx, userID, checkTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DNDOverride), args.Error(1)
}

func (m *MockDNDOverrideRepository) GetUserOverrides(ctx context.Context, userID uuid.UUID) ([]*DNDOverride, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*DNDOverride), args.Error(1)
}

func (m *MockDNDOverrideRepository) DeleteOverride(ctx context.Context, overrideID uuid.UUID) error {
	args := m.Called(ctx, overrideID)
	return args.Error(0)
}

func (m *MockDNDOverrideRepository) CleanupExpiredOverrides(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func TestDNDOverrideService_CreateOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	duration := 2 * time.Hour
	reason := OverrideReasonUrgent

	mockRepo.On("CreateOverride", mock.Anything, mock.AnythingOfType("*notification.DNDOverride")).Return(nil)

	override, err := service.CreateOverride(context.Background(), userID, duration, reason)

	require.NoError(t, err)
	assert.NotNil(t, override)
	assert.Equal(t, userID, override.UserID)
	assert.Equal(t, string(reason), override.Reason)
	assert.True(t, override.EndTime.After(override.StartTime))

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_CreateScheduledOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	startTime := time.Now().Add(time.Hour)
	endTime := startTime.Add(2 * time.Hour)
	reason := OverrideReasonManual

	tests := []struct {
		name        string
		startTime   time.Time
		endTime     time.Time
		expectError bool
	}{
		{
			name:        "valid time range",
			startTime:   startTime,
			endTime:     endTime,
			expectError: false,
		},
		{
			name:        "end before start",
			startTime:   endTime,
			endTime:     startTime,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.expectError {
				mockRepo.On("CreateOverride", mock.Anything, mock.AnythingOfType("*notification.DNDOverride")).Return(nil).Once()
			}

			override, err := service.CreateScheduledOverride(context.Background(), userID, tt.startTime, tt.endTime, reason)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, override)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, override)
				assert.Equal(t, userID, override.UserID)
			}
		})
	}

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_IsOverrideActive(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	now := time.Now()

	tests := []struct {
		name           string
		mockOverride   *DNDOverride
		mockError      error
		expectActive   bool
		expectOverride bool
	}{
		{
			name: "active override",
			mockOverride: &DNDOverride{
				ID:        uuid.New(),
				UserID:    userID,
				StartTime: now.Add(-time.Hour),
				EndTime:   now.Add(time.Hour),
				Reason:    string(OverrideReasonUrgent),
			},
			mockError:      nil,
			expectActive:   true,
			expectOverride: true,
		},
		{
			name:           "no override",
			mockOverride:   nil,
			mockError:      nil,
			expectActive:   false,
			expectOverride: false,
		},
		{
			name: "expired override",
			mockOverride: &DNDOverride{
				ID:        uuid.New(),
				UserID:    userID,
				StartTime: now.Add(-2 * time.Hour),
				EndTime:   now.Add(-time.Hour),
				Reason:    string(OverrideReasonUrgent),
			},
			mockError:      nil,
			expectActive:   false,
			expectOverride: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo.On("GetActiveOverride", mock.Anything, userID, mock.AnythingOfType("time.Time")).
				Return(tt.mockOverride, tt.mockError).Once()

			active, override, err := service.IsOverrideActive(context.Background(), userID, now)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectActive, active)

			if tt.expectOverride {
				assert.NotNil(t, override)
			} else {
				assert.Nil(t, override)
			}
		})
	}

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_ShouldSendDuringDND(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	now := time.Now()

	tests := []struct {
		name          string
		notification  *Notification
		mockOverride  *DNDOverride
		expectSend    bool
		expectedReason string
	}{
		{
			name: "active override allows",
			notification: &Notification{
				ID:       uuid.New(),
				UserID:   userID,
				Priority: PriorityNormal,
			},
			mockOverride: &DNDOverride{
				ID:        uuid.New(),
				UserID:    userID,
				StartTime: now.Add(-time.Hour),
				EndTime:   now.Add(time.Hour),
				Reason:    string(OverrideReasonUrgent),
			},
			expectSend:     true,
			expectedReason: "Override active: urgent",
		},
		{
			name: "urgent priority overrides",
			notification: &Notification{
				ID:       uuid.New(),
				UserID:   userID,
				Priority: PriorityUrgent,
			},
			mockOverride:   nil,
			expectSend:     true,
			expectedReason: "Priority override: urgent",
		},
		{
			name: "normal priority blocked",
			notification: &Notification{
				ID:       uuid.New(),
				UserID:   userID,
				Priority: PriorityNormal,
			},
			mockOverride:   nil,
			expectSend:     false,
			expectedReason: "DND active, no override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo.On("GetActiveOverride", mock.Anything, userID, mock.AnythingOfType("time.Time")).
				Return(tt.mockOverride, nil).Once()

			shouldSend, reason, err := service.ShouldSendDuringDND(context.Background(), tt.notification, &NotificationSettings{})

			assert.NoError(t, err)
			assert.Equal(t, tt.expectSend, shouldSend)
			assert.Contains(t, reason, tt.expectedReason)
		})
	}

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_CancelOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	overrideID := uuid.New()

	mockRepo.On("DeleteOverride", mock.Anything, overrideID).Return(nil)

	err := service.CancelOverride(context.Background(), overrideID)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_GetUserOverrides(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	expectedOverrides := []*DNDOverride{
		{
			ID:     uuid.New(),
			UserID: userID,
			Reason: string(OverrideReasonUrgent),
		},
		{
			ID:     uuid.New(),
			UserID: userID,
			Reason: string(OverrideReasonManual),
		},
	}

	mockRepo.On("GetUserOverrides", mock.Anything, userID).Return(expectedOverrides, nil)

	overrides, err := service.GetUserOverrides(context.Background(), userID)

	assert.NoError(t, err)
	assert.Len(t, overrides, 2)
	assert.Equal(t, expectedOverrides, overrides)

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_CleanupExpired(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	expectedCount := 5

	mockRepo.On("CleanupExpiredOverrides", mock.Anything).Return(expectedCount, nil)

	err := service.CleanupExpired(context.Background())

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_CreateTravelOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	duration := 24 * time.Hour

	tests := []struct {
		name        string
		timezone    string
		expectError bool
	}{
		{
			name:        "valid timezone",
			timezone:    "America/New_York",
			expectError: false,
		},
		{
			name:        "invalid timezone",
			timezone:    "Invalid/Timezone",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.expectError {
				mockRepo.On("CreateOverride", mock.Anything, mock.AnythingOfType("*notification.DNDOverride")).Return(nil).Once()
			}

			override, err := service.CreateTravelOverride(context.Background(), userID, tt.timezone, duration)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, override)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, override)
				assert.Equal(t, string(OverrideReasonTravel), override.Reason)
			}
		})
	}

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_GetOverrideStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	now := time.Now()

	activeOverride := &DNDOverride{
		ID:        uuid.New(),
		UserID:    userID,
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
		Reason:    string(OverrideReasonUrgent),
	}

	allOverrides := []*DNDOverride{
		activeOverride,
		{
			ID:        uuid.New(),
			UserID:    userID,
			StartTime: now.Add(2 * time.Hour),
			EndTime:   now.Add(4 * time.Hour),
			Reason:    string(OverrideReasonManual),
		},
	}

	mockRepo.On("GetActiveOverride", mock.Anything, userID, mock.AnythingOfType("time.Time")).
		Return(activeOverride, nil)
	mockRepo.On("GetUserOverrides", mock.Anything, userID).
		Return(allOverrides, nil)

	status, err := service.GetOverrideStatus(context.Background(), userID)

	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.True(t, status["has_active_override"].(bool))
	assert.Equal(t, 2, status["total_overrides"])

	activeInfo := status["active_override"].(map[string]interface{})
	assert.Equal(t, activeOverride.ID.String(), activeInfo["id"])
	assert.Equal(t, string(OverrideReasonUrgent), activeInfo["reason"])

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_CreateEmergencyOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()

	mockRepo.On("CreateOverride", mock.Anything, mock.AnythingOfType("*notification.DNDOverride")).Return(nil)

	override, err := service.CreateEmergencyOverride(context.Background(), userID)

	require.NoError(t, err)
	assert.NotNil(t, override)
	assert.Equal(t, string(OverrideReasonEmergency), override.Reason)

	// Should be 24 hours
	duration := override.EndTime.Sub(override.StartTime)
	assert.InDelta(t, 24*time.Hour, duration, float64(time.Second))

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_CreateVIPOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	userID := uuid.New()
	duration := 4 * time.Hour

	mockRepo.On("CreateOverride", mock.Anything, mock.AnythingOfType("*notification.DNDOverride")).Return(nil)

	override, err := service.CreateVIPOverride(context.Background(), userID, duration)

	require.NoError(t, err)
	assert.NotNil(t, override)
	assert.Equal(t, string(OverrideReasonVIP), override.Reason)

	mockRepo.AssertExpectations(t)
}

func TestDNDOverrideService_ValidateOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	now := time.Now()

	tests := []struct {
		name        string
		override    *DNDOverride
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid override",
			override: &DNDOverride{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				StartTime: now,
				EndTime:   now.Add(2 * time.Hour),
				Reason:    string(OverrideReasonUrgent),
			},
			expectError: false,
		},
		{
			name: "missing user ID",
			override: &DNDOverride{
				ID:        uuid.New(),
				UserID:    uuid.Nil,
				StartTime: now,
				EndTime:   now.Add(2 * time.Hour),
				Reason:    string(OverrideReasonUrgent),
			},
			expectError: true,
			errorMsg:    "user ID is required",
		},
		{
			name: "end before start",
			override: &DNDOverride{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				StartTime: now.Add(2 * time.Hour),
				EndTime:   now,
				Reason:    string(OverrideReasonUrgent),
			},
			expectError: true,
			errorMsg:    "end time must be after start time",
		},
		{
			name: "missing reason",
			override: &DNDOverride{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				StartTime: now,
				EndTime:   now.Add(2 * time.Hour),
				Reason:    "",
			},
			expectError: true,
			errorMsg:    "reason is required",
		},
		{
			name: "duration too long",
			override: &DNDOverride{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				StartTime: now,
				EndTime:   now.Add(8 * 24 * time.Hour), // 8 days
				Reason:    string(OverrideReasonUrgent),
			},
			expectError: true,
			errorMsg:    "cannot exceed 7 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ValidateOverride(tt.override)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDNDOverrideService_shouldOverrideByPriority(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := new(MockDNDOverrideRepository)
	service := NewDNDOverrideService(logger, mockRepo)

	tests := []struct {
		name     string
		priority Priority
		expected bool
	}{
		{
			name:     "urgent priority overrides",
			priority: PriorityUrgent,
			expected: true,
		},
		{
			name:     "high priority does not override",
			priority: PriorityHigh,
			expected: false,
		},
		{
			name:     "normal priority does not override",
			priority: PriorityNormal,
			expected: false,
		},
		{
			name:     "low priority does not override",
			priority: PriorityLow,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.shouldOverrideByPriority(tt.priority)
			assert.Equal(t, tt.expected, result)
		})
	}
}

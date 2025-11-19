package notification

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockSettingsProvider is a mock implementation of SettingsProvider
type MockSettingsProvider struct {
	mock.Mock
}

func (m *MockSettingsProvider) GetUserNotificationSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*NotificationSettings), args.Error(1)
}

func (m *MockSettingsProvider) GetUsersWithRemindersEnabled(ctx context.Context) ([]uuid.UUID, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uuid.UUID), args.Error(1)
}

func (m *MockSettingsProvider) UpdateLastReminderSent(ctx context.Context, userID uuid.UUID, timestamp time.Time) error {
	args := m.Called(ctx, userID, timestamp)
	return args.Error(0)
}

func TestSettingsEngine_ShouldSendNotification(t *testing.T) {
	logger := zap.NewNop()
	userID := uuid.New()
	ctx := context.Background()

	tests := []struct {
		name             string
		notificationType NotificationType
		settings         *NotificationSettings
		scheduledTime    time.Time
		expectedResult   bool
		expectedError    bool
	}{
		{
			name:             "email notification enabled",
			notificationType: NotificationTypeEmail,
			settings: &NotificationSettings{
				UserID:             userID,
				EmailNotifications: true,
				DNDEnabled:         false,
				Timezone:           "UTC",
			},
			scheduledTime:  time.Now(),
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:             "email notification disabled",
			notificationType: NotificationTypeEmail,
			settings: &NotificationSettings{
				UserID:             userID,
				EmailNotifications: false,
				DNDEnabled:         false,
				Timezone:           "UTC",
			},
			scheduledTime:  time.Now(),
			expectedResult: false,
			expectedError:  false,
		},
		{
			name:             "notification blocked by DND",
			notificationType: NotificationTypeEmail,
			settings: &NotificationSettings{
				UserID:             userID,
				EmailNotifications: true,
				DNDEnabled:         true,
				DNDStartTime:       timePtr(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)),
				DNDEndTime:         timePtr(time.Date(0, 1, 1, 23, 59, 0, 0, time.UTC)),
				DNDDays:            []int{0, 1, 2, 3, 4, 5, 6}, // All days
				Timezone:           "UTC",
			},
			scheduledTime:  time.Now(),
			expectedResult: false,
			expectedError:  false,
		},
		{
			name:             "push notification enabled",
			notificationType: NotificationTypePush,
			settings: &NotificationSettings{
				UserID:            userID,
				PushNotifications: true,
				DNDEnabled:        false,
				Timezone:          "UTC",
			},
			scheduledTime:  time.Now(),
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:             "reminder notification enabled",
			notificationType: NotificationTypeReminder,
			settings: &NotificationSettings{
				UserID:           userID,
				CheckinReminders: true,
				DNDEnabled:       false,
				Timezone:         "UTC",
			},
			scheduledTime:  time.Now(),
			expectedResult: true,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := new(MockSettingsProvider)
			mockProvider.On("GetUserNotificationSettings", ctx, userID).Return(tt.settings, nil)

			engine := NewSettingsEngine(logger, mockProvider)

			result, err := engine.ShouldSendNotification(ctx, userID, tt.notificationType, tt.scheduledTime)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestSettingsEngine_GetNextAvailableTime(t *testing.T) {
	logger := zap.NewNop()
	userID := uuid.New()
	ctx := context.Background()

	tests := []struct {
		name          string
		settings      *NotificationSettings
		requestedTime time.Time
		expectedDiff  time.Duration // Expected difference from requested time
	}{
		{
			name: "no DND enabled",
			settings: &NotificationSettings{
				UserID:     userID,
				DNDEnabled: false,
				Timezone:   "UTC",
			},
			requestedTime: time.Now(),
			expectedDiff:  0,
		},
		{
			name: "currently in DND period",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)),
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC)), // DND until 8 AM
				DNDDays:      []int{int(time.Now().Weekday())},
				Timezone:     "UTC",
			},
			requestedTime: time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC), // 2 AM (in DND)
			expectedDiff:  6 * time.Hour,                                // Should return 8 AM
		},
		{
			name: "not in DND period",
			settings: &NotificationSettings{
				UserID:       userID,
				DNDEnabled:   true,
				DNDStartTime: timePtr(time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)), // DND starts at 10 PM
				DNDEndTime:   timePtr(time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)),  // DND ends at 6 AM
				DNDDays:      []int{int(time.Now().Weekday())},
				Timezone:     "UTC",
			},
			requestedTime: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), // Noon (not in DND)
			expectedDiff:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := new(MockSettingsProvider)
			mockProvider.On("GetUserNotificationSettings", ctx, userID).Return(tt.settings, nil)

			engine := NewSettingsEngine(logger, mockProvider)

			result, err := engine.GetNextAvailableTime(ctx, userID, tt.requestedTime)

			assert.NoError(t, err)
			if tt.expectedDiff == 0 {
				assert.Equal(t, tt.requestedTime, result)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestSettingsEngine_ApplyBatchSettings(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	user1ID := uuid.New()
	user2ID := uuid.New()

	notifications := []*Notification{
		{
			ID:           uuid.New(),
			UserID:       user1ID,
			Type:         NotificationTypeEmail,
			ScheduledFor: time.Now(),
		},
		{
			ID:           uuid.New(),
			UserID:       user2ID,
			Type:         NotificationTypePush,
			ScheduledFor: time.Now(),
		},
		{
			ID:           uuid.New(),
			UserID:       user1ID,
			Type:         NotificationTypeReminder,
			ScheduledFor: time.Now(),
		},
	}

	settings1 := &NotificationSettings{
		UserID:             user1ID,
		EmailNotifications: true,
		PushNotifications:  false,
		CheckinReminders:   false,
		DNDEnabled:         false,
		Timezone:           "UTC",
	}

	settings2 := &NotificationSettings{
		UserID:            user2ID,
		EmailNotifications: false,
		PushNotifications:  true,
		CheckinReminders:   true,
		DNDEnabled:         false,
		Timezone:           "UTC",
	}

	mockProvider := new(MockSettingsProvider)
	mockProvider.On("GetUserNotificationSettings", ctx, user1ID).Return(settings1, nil)
	mockProvider.On("GetUserNotificationSettings", ctx, user2ID).Return(settings2, nil)

	engine := NewSettingsEngine(logger, mockProvider)

	filtered, err := engine.ApplyBatchSettings(ctx, notifications)

	assert.NoError(t, err)
	assert.Len(t, filtered, 2) // Only email for user1 and push for user2 should pass

	// Verify the correct notifications passed
	for _, n := range filtered {
		if n.UserID == user1ID {
			assert.Equal(t, NotificationTypeEmail, n.Type)
		} else if n.UserID == user2ID {
			assert.Equal(t, NotificationTypePush, n.Type)
		}
	}

	mockProvider.AssertExpectations(t)
}

func TestSettingsEngine_ProcessReminderSchedule(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	user1ID := uuid.New()
	user2ID := uuid.New()
	users := []uuid.UUID{user1ID, user2ID}

	settings1 := &NotificationSettings{
		UserID:                  user1ID,
		CheckinReminders:        true,
		ReminderIntervalMinutes: 60,
		DNDEnabled:              false,
		Timezone:                "UTC",
	}

	settings2 := &NotificationSettings{
		UserID:                  user2ID,
		CheckinReminders:        true,
		ReminderIntervalMinutes: 120,
		DNDEnabled:              false,
		Timezone:                "UTC",
	}

	mockProvider := new(MockSettingsProvider)
	mockProvider.On("GetUsersWithRemindersEnabled", ctx).Return(users, nil)
	mockProvider.On("GetUserNotificationSettings", ctx, user1ID).Return(settings1, nil)
	mockProvider.On("GetUserNotificationSettings", ctx, user2ID).Return(settings2, nil)
	mockProvider.On("UpdateLastReminderSent", ctx, user1ID, mock.Anything).Return(nil)
	mockProvider.On("UpdateLastReminderSent", ctx, user2ID, mock.Anything).Return(nil)

	engine := NewSettingsEngine(logger, mockProvider)

	err := engine.ProcessReminderSchedule(ctx)

	assert.NoError(t, err)
	mockProvider.AssertExpectations(t)
}

// Helper function to create time pointers
func timePtr(t time.Time) *time.Time {
	return &t
}
package notification

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"github.com/dev-jelly/donelist/internal/checkin"
)

// MockService is a mock implementation of notification.Service for testing
type MockService struct {
	mock.Mock
}

func (m *MockService) SendNotification(ctx context.Context, notification *Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockService) GetUserNotificationSettings(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error) {
	args := m.Called(ctx, userID)
	if settings := args.Get(0); settings != nil {
		return settings.(*NotificationSettings), args.Error(1)
	}
	return nil, args.Error(1)
}

// MockCheckinRepository is a mock implementation of checkin.Repository for testing
type MockCheckinRepository struct {
	mock.Mock
}

func (m *MockCheckinRepository) GetLastCheckin(ctx context.Context, userID uuid.UUID) (*checkin.Checkin, error) {
	args := m.Called(ctx, userID)
	if ch := args.Get(0); ch != nil {
		return ch.(*checkin.Checkin), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestIntervalChecker_GetIntervalRules(t *testing.T) {
	rules := GetIntervalRules()

	assert.Len(t, rules, 4)
	assert.Equal(t, 15*time.Minute, rules[0].Duration)
	assert.Equal(t, 30*time.Minute, rules[1].Duration)
	assert.Equal(t, 45*time.Minute, rules[2].Duration)
	assert.Equal(t, 2*time.Hour, rules[3].Duration)
}

func TestIntervalChecker_shouldSendIntervalNotification(t *testing.T) {
	logger := zap.NewNop()
	db, _, _ := sqlmock.New()
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ic := NewIntervalChecker(sqlxDB, nil, nil, logger)

	tests := []struct {
		name                 string
		timeSinceLastCheckin time.Duration
		targetInterval       time.Duration
		expected             bool
	}{
		{
			name:                 "exactly at interval",
			timeSinceLastCheckin: 15 * time.Minute,
			targetInterval:       15 * time.Minute,
			expected:             true,
		},
		{
			name:                 "within tolerance lower bound",
			timeSinceLastCheckin: 13 * time.Minute,
			targetInterval:       15 * time.Minute,
			expected:             false, // 13 < 15-5 (10)
		},
		{
			name:                 "within tolerance upper bound",
			timeSinceLastCheckin: 19 * time.Minute,
			targetInterval:       15 * time.Minute,
			expected:             true, // 19 < 15+5 (20)
		},
		{
			name:                 "outside tolerance - too early",
			timeSinceLastCheckin: 5 * time.Minute,
			targetInterval:       15 * time.Minute,
			expected:             false,
		},
		{
			name:                 "outside tolerance - too late",
			timeSinceLastCheckin: 25 * time.Minute,
			targetInterval:       15 * time.Minute,
			expected:             false,
		},
		{
			name:                 "2 hour interval - exact",
			timeSinceLastCheckin: 2 * time.Hour,
			targetInterval:       2 * time.Hour,
			expected:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ic.shouldSendIntervalNotification(tt.timeSinceLastCheckin, tt.targetInterval)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIntervalChecker_CheckUserInterval(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	userID := uuid.New()

	t.Run("should send notification for 15 minute interval", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		mockCheckinRepo := new(MockCheckinRepository)
		mockService := new(MockService)

		ic := NewIntervalChecker(sqlxDB, mockCheckinRepo, mockService, logger)

		// Setup mocks
		lastCheckinTime := time.Now().Add(-15 * time.Minute)
		lastCheckin := &checkin.Checkin{
			ID:          uuid.New(),
			UserID:      userID,
			CheckinTime: lastCheckinTime,
		}

		mockCheckinRepo.On("GetLastCheckin", ctx, userID).Return(lastCheckin, nil)

		settings := &NotificationSettings{
			UserID:           userID,
			CheckinReminders: true,
		}
		mockService.On("GetUserNotificationSettings", ctx, userID).Return(settings, nil)

		mockService.On("SendNotification", ctx, mock.MatchedBy(func(n *Notification) bool {
			return n.UserID == userID &&
				   n.Type == NotificationTypeReminder &&
				   n.Title == "Time to Check In"
		})).Return(nil)

		// Execute
		err := ic.CheckUserInterval(ctx, userID)

		// Assert
		assert.NoError(t, err)
		mockCheckinRepo.AssertExpectations(t)
		mockService.AssertExpectations(t)
	})

	t.Run("should skip if reminders disabled", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		mockCheckinRepo := new(MockCheckinRepository)
		mockService := new(MockService)

		ic := NewIntervalChecker(sqlxDB, mockCheckinRepo, mockService, logger)

		// Setup mocks
		lastCheckinTime := time.Now().Add(-15 * time.Minute)
		lastCheckin := &checkin.Checkin{
			ID:          uuid.New(),
			UserID:      userID,
			CheckinTime: lastCheckinTime,
		}

		mockCheckinRepo.On("GetLastCheckin", ctx, userID).Return(lastCheckin, nil)

		settings := &NotificationSettings{
			UserID:           userID,
			CheckinReminders: false, // Disabled
		}
		mockService.On("GetUserNotificationSettings", ctx, userID).Return(settings, nil)

		// Should NOT call SendNotification

		// Execute
		err := ic.CheckUserInterval(ctx, userID)

		// Assert
		assert.NoError(t, err)
		mockCheckinRepo.AssertExpectations(t)
		mockService.AssertExpectations(t)
		// Verify SendNotification was not called
		mockService.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
	})

	t.Run("should skip if no last checkin", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		mockCheckinRepo := new(MockCheckinRepository)
		mockService := new(MockService)

		ic := NewIntervalChecker(sqlxDB, mockCheckinRepo, mockService, logger)

		// Setup mocks - no last checkin
		mockCheckinRepo.On("GetLastCheckin", ctx, userID).Return(nil, nil)

		// Should NOT call GetUserNotificationSettings or SendNotification

		// Execute
		err := ic.CheckUserInterval(ctx, userID)

		// Assert
		assert.NoError(t, err)
		mockCheckinRepo.AssertExpectations(t)
		// Verify other methods were not called
		mockService.AssertNotCalled(t, "GetUserNotificationSettings", mock.Anything, mock.Anything)
		mockService.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
	})

	t.Run("should not send if outside interval window", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		mockCheckinRepo := new(MockCheckinRepository)
		mockService := new(MockService)

		ic := NewIntervalChecker(sqlxDB, mockCheckinRepo, mockService, logger)

		// Setup mocks - last checkin was 5 minutes ago (too recent)
		lastCheckinTime := time.Now().Add(-5 * time.Minute)
		lastCheckin := &checkin.Checkin{
			ID:          uuid.New(),
			UserID:      userID,
			CheckinTime: lastCheckinTime,
		}

		mockCheckinRepo.On("GetLastCheckin", ctx, userID).Return(lastCheckin, nil)

		settings := &NotificationSettings{
			UserID:           userID,
			CheckinReminders: true,
		}
		mockService.On("GetUserNotificationSettings", ctx, userID).Return(settings, nil)

		// Should NOT call SendNotification because 5 minutes is not within any interval window

		// Execute
		err := ic.CheckUserInterval(ctx, userID)

		// Assert
		assert.NoError(t, err)
		mockCheckinRepo.AssertExpectations(t)
		mockService.AssertExpectations(t)
		// Verify SendNotification was not called
		mockService.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
	})
}

func TestIntervalChecker_ProcessIntervalChecks(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("should process active users", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		sqlxDB := sqlx.NewDb(db, "sqlmock")

		mockCheckinRepo := new(MockCheckinRepository)
		mockService := new(MockService)

		ic := NewIntervalChecker(sqlxDB, mockCheckinRepo, mockService, logger)

		// Setup database mock
		userID1 := uuid.New()
		userID2 := uuid.New()

		rows := sqlmock.NewRows([]string{"user_id"}).
			AddRow(userID1).
			AddRow(userID2)

		mock.ExpectQuery("SELECT DISTINCT user_id").
			WillReturnRows(rows)

		// Setup mocks for both users
		// User 1: has recent checkin
		lastCheckin1 := &checkin.Checkin{
			ID:          uuid.New(),
			UserID:      userID1,
			CheckinTime: time.Now().Add(-30 * time.Minute),
		}
		mockCheckinRepo.On("GetLastCheckin", ctx, userID1).Return(lastCheckin1, nil)

		settings1 := &NotificationSettings{
			UserID:           userID1,
			CheckinReminders: true,
		}
		mockService.On("GetUserNotificationSettings", ctx, userID1).Return(settings1, nil)
		mockService.On("SendNotification", ctx, mock.Anything).Return(nil)

		// User 2: no last checkin
		mockCheckinRepo.On("GetLastCheckin", ctx, userID2).Return(nil, nil)

		// Execute
		err = ic.ProcessIntervalChecks(ctx)

		// Assert
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
		mockCheckinRepo.AssertExpectations(t)
	})

	t.Run("should handle query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		sqlxDB := sqlx.NewDb(db, "sqlmock")

		ic := NewIntervalChecker(sqlxDB, nil, nil, logger)

		// Setup database mock to return error
		mock.ExpectQuery("SELECT DISTINCT user_id").
			WillReturnError(assert.AnError)

		// Execute
		err = ic.ProcessIntervalChecks(ctx)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get active users")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
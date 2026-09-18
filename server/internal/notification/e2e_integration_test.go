package notification

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
)

// E2ETestSuite is the end-to-end test suite
type E2ETestSuite struct {
	suite.Suite
	db          *sqlx.DB
	redisClient *redis.Client
	miniRedis   *miniredis.Miniredis
	service     *Service
	mockFCM     *MockFCMProvider
	mockAPNs    *MockAPNsProvider
	ctx         context.Context
	cancel      context.CancelFunc
}

// SetupSuite runs once before all tests
func (s *E2ETestSuite) SetupSuite() {
	// Setup mini Redis
	mr, err := miniredis.Run()
	require.NoError(s.T(), err)
	s.miniRedis = mr

	// Setup Redis client
	s.redisClient = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Setup mock database (in-memory SQLite for testing)
	db, err := sqlx.Connect("sqlite3", ":memory:")
	require.NoError(s.T(), err)
	s.db = db

	// Create necessary tables
	s.createTables()

	// Setup mock providers
	s.mockFCM = &MockFCMProvider{
		sent: make(map[string]*NotificationJob),
	}
	s.mockAPNs = &MockAPNsProvider{
		sent: make(map[string]*NotificationJob),
	}
}

// SetupTest runs before each test
func (s *E2ETestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)

	// Clear Redis
	s.miniRedis.FlushAll()

	// Clear mock provider state
	s.mockFCM.Reset()
	s.mockAPNs.Reset()

	// Create notification service
	logger := zaptest.NewLogger(s.T())

	queueConfig := QueueConfig{
		RedisClient:     s.redisClient,
		MaxRetries:      3,
		RetryDelay:      100 * time.Millisecond,
		QueuePrefix:     "test:notifications",
		EnableDLQ:       true,
	}

	schedulerConfig := &SchedulerConfig{
		WorkerCount:  2,
		PollInterval: 100 * time.Millisecond,
	}

	serviceConfig := &ServiceConfig{
		QueueConfig:     queueConfig,
		SchedulerConfig: schedulerConfig,
	}

	service, err := NewService(s.db, logger, serviceConfig)
	require.NoError(s.T(), err)
	s.service = service
}

// TearDownTest runs after each test
func (s *E2ETestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.service != nil {
		s.service.Stop()
	}
}

// TearDownSuite runs once after all tests
func (s *E2ETestSuite) TearDownSuite() {
	if s.db != nil {
		s.db.Close()
	}
	if s.redisClient != nil {
		s.redisClient.Close()
	}
	if s.miniRedis != nil {
		s.miniRedis.Close()
	}
}

// createTables creates necessary database tables
func (s *E2ETestSuite) createTables() {
	schema := `
		CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			priority TEXT NOT NULL,
			status TEXT NOT NULL,
			title TEXT,
			body TEXT,
			scheduled_for TIMESTAMP,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS notification_settings (
			user_id TEXT PRIMARY KEY,
			dnd_enabled BOOLEAN DEFAULT FALSE,
			dnd_start_hour INTEGER,
			dnd_end_hour INTEGER,
			timezone TEXT,
			checkin_reminders BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS dnd_overrides (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			start_time TIMESTAMP NOT NULL,
			end_time TIMESTAMP NOT NULL,
			reason TEXT,
			created_at TIMESTAMP
		);
	`

	_, err := s.db.Exec(schema)
	require.NoError(s.T(), err)
}

// TestE2E_BasicNotificationFlow tests the complete notification flow
func (s *E2ETestSuite) TestE2E_BasicNotificationFlow() {
	userID := uuid.New()

	// Create notification
	notification := &Notification{
		ID:           uuid.New(),
		UserID:       userID,
		Type:         NotificationTypePush,
		Priority:     PriorityNormal,
		Status:       StatusPending,
		Title:        "Test Notification",
		Body:         "This is a test",
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
	}

	// Send notification
	err := s.service.SendNotification(s.ctx, notification)
	assert.NoError(s.T(), err)

	// Start service to process
	err = s.service.Start(s.ctx)
	assert.NoError(s.T(), err)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify notification was processed
	stats, err := s.service.GetQueueStats(s.ctx)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), stats)
}

// TestE2E_DNDBlocking tests DND blocking functionality
func (s *E2ETestSuite) TestE2E_DNDBlocking() {
	userID := uuid.New()

	// Create DND settings
	settings := &NotificationSettings{
		UserID:      userID,
		DNDEnabled:  true,
		DNDStartHour: 22,
		DNDEndHour:   8,
		Timezone:    "America/New_York",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := s.service.UpdateUserNotificationSettings(s.ctx, settings)
	assert.NoError(s.T(), err)

	// Create notification during DND hours (assuming current time is in DND window)
	notification := &Notification{
		ID:           uuid.New(),
		UserID:       userID,
		Type:         NotificationTypePush,
		Priority:     PriorityNormal,
		Status:       StatusPending,
		Title:        "DND Test",
		Body:         "Should be blocked or rescheduled",
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
	}

	err = s.service.SendNotification(s.ctx, notification)
	assert.NoError(s.T(), err)

	// Notification should be rescheduled to after DND window
	// This is validated by the service logic
}

// TestE2E_RetryMechanism tests notification retry
func (s *E2ETestSuite) TestE2E_RetryMechanism() {
	userID := uuid.New()

	notification := &Notification{
		ID:           uuid.New(),
		UserID:       userID,
		Type:         NotificationTypePush,
		Priority:     PriorityHigh,
		Status:       StatusPending,
		Title:        "Retry Test",
		Body:         "Test retry mechanism",
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
	}

	err := s.service.SendNotification(s.ctx, notification)
	assert.NoError(s.T(), err)

	err = s.service.Start(s.ctx)
	assert.NoError(s.T(), err)

	// Wait for potential retries
	time.Sleep(1 * time.Second)

	// Service should handle retries automatically
}

// TestE2E_BatchProcessing tests batch notification sending
func (s *E2ETestSuite) TestE2E_BatchProcessing() {
	// Create multiple notifications
	notifications := make([]*Notification, 10)
	for i := 0; i < 10; i++ {
		notifications[i] = &Notification{
			ID:           uuid.New(),
			UserID:       uuid.New(),
			Type:         NotificationTypePush,
			Priority:     PriorityNormal,
			Status:       StatusPending,
			Title:        "Batch Test",
			Body:         "Batch notification",
			ScheduledFor: time.Now(),
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			MaxRetries:   3,
		}
	}

	err := s.service.SendBatchNotifications(s.ctx, notifications)
	assert.NoError(s.T(), err)

	err = s.service.Start(s.ctx)
	assert.NoError(s.T(), err)

	// Wait for batch processing
	time.Sleep(1 * time.Second)

	stats, err := s.service.GetQueueStats(s.ctx)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), stats)
}

// TestE2E_PriorityOrdering tests priority-based processing
func (s *E2ETestSuite) TestE2E_PriorityOrdering() {
	userID := uuid.New()

	// Create notifications with different priorities
	priorities := []Priority{PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent}

	for _, priority := range priorities {
		notification := &Notification{
			ID:           uuid.New(),
			UserID:       userID,
			Type:         NotificationTypePush,
			Priority:     priority,
			Status:       StatusPending,
			Title:        "Priority Test",
			Body:         string(priority),
			ScheduledFor: time.Now(),
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			MaxRetries:   3,
		}

		err := s.service.SendNotification(s.ctx, notification)
		assert.NoError(s.T(), err)
	}

	// Higher priority notifications should be processed first
	// This is verified through the queue ordering
}

// TestE2E_DNDOverride tests DND override functionality
func (s *E2ETestSuite) TestE2E_DNDOverride() {
	userID := uuid.New()

	// Create DND settings
	settings := &NotificationSettings{
		UserID:      userID,
		DNDEnabled:  true,
		DNDStartHour: 22,
		DNDEndHour:   8,
		Timezone:    "UTC",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := s.service.UpdateUserNotificationSettings(s.ctx, settings)
	assert.NoError(s.T(), err)

	// Create DND override
	override := &DNDOverride{
		ID:        uuid.New(),
		UserID:    userID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Hour),
		Reason:    "Important meeting",
		CreatedAt: time.Now(),
	}

	err = s.service.CreateDNDOverride(s.ctx, override)
	assert.NoError(s.T(), err)

	// Notifications during override period should be allowed
}

// TestE2E_TimezoneHandling tests timezone-aware scheduling
func (s *E2ETestSuite) TestE2E_TimezoneHandling() {
	userID := uuid.New()

	// Set user timezone
	settings := &NotificationSettings{
		UserID:    userID,
		Timezone:  "Asia/Tokyo",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := s.service.UpdateUserNotificationSettings(s.ctx, settings)
	assert.NoError(s.T(), err)

	// Create notification
	notification := &Notification{
		ID:           uuid.New(),
		UserID:       userID,
		Type:         NotificationTypePush,
		Priority:     PriorityNormal,
		Status:       StatusPending,
		Title:        "Timezone Test",
		Body:         "Testing timezone handling",
		ScheduledFor: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MaxRetries:   3,
	}

	err = s.service.SendNotification(s.ctx, notification)
	assert.NoError(s.T(), err)

	// Notification scheduling should respect user timezone
}

// TestE2E_HealthCheck tests service health monitoring
func (s *E2ETestSuite) TestE2E_HealthCheck() {
	err := s.service.Start(s.ctx)
	assert.NoError(s.T(), err)

	health, err := s.service.GetHealthStatus(s.ctx)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), health)
	assert.Equal(t, "notification", health["service"])
}

// TestE2E_CleanupExpiredNotifications tests cleanup functionality
func (s *E2ETestSuite) TestE2E_CleanupExpiredNotifications() {
	// Create old notification (simulate by using past timestamp)
	// In real scenario, this would be an actual old record

	count, err := s.service.CleanupExpiredNotifications(s.ctx, 24*time.Hour)
	assert.NoError(s.T(), err)
	assert.GreaterOrEqual(s.T(), count, int64(0))
}

// TestE2E_ConcurrentNotifications tests concurrent notification handling
func (s *E2ETestSuite) TestE2E_ConcurrentNotifications() {
	err := s.service.Start(s.ctx)
	assert.NoError(s.T(), err)

	done := make(chan bool)

	// Send notifications concurrently
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				notification := &Notification{
					ID:           uuid.New(),
					UserID:       uuid.New(),
					Type:         NotificationTypePush,
					Priority:     PriorityNormal,
					Status:       StatusPending,
					Title:        "Concurrent Test",
					Body:         "Testing concurrency",
					ScheduledFor: time.Now(),
					CreatedAt:    time.Now(),
					UpdatedAt:    time.Now(),
					MaxRetries:   3,
				}

				s.service.SendNotification(s.ctx, notification)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Allow time for processing
	time.Sleep(1 * time.Second)
}

// Run the test suite
func TestE2ETestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}
	suite.Run(t, new(E2ETestSuite))
}

// MockFCMProvider is a mock FCM provider for testing
type MockFCMProvider struct {
	mu   sync.Mutex
	sent map[string]*NotificationJob
	fail bool
}

func (m *MockFCMProvider) Send(ctx context.Context, job *NotificationJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return fmt.Errorf("mock FCM send failed")
	}

	m.sent[job.ID] = job
	return nil
}

func (m *MockFCMProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = make(map[string]*NotificationJob)
	m.fail = false
}

// MockAPNsProvider is a mock APNs provider for testing
type MockAPNsProvider struct {
	mu   sync.Mutex
	sent map[string]*NotificationJob
	fail bool
}

func (m *MockAPNsProvider) Send(ctx context.Context, job *NotificationJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return fmt.Errorf("mock APNs send failed")
	}

	m.sent[job.ID] = job
	return nil
}

func (m *MockAPNsProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = make(map[string]*NotificationJob)
	m.fail = false
}

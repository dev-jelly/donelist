package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EventType represents the type of analytics event
type EventType string

const (
	// User events
	EventUserSignup       EventType = "user.signup"
	EventUserLogin        EventType = "user.login"
	EventUserLogout       EventType = "user.logout"
	EventUserProfileUpdate EventType = "user.profile_update"
	EventUserDeleted      EventType = "user.deleted"

	// Checkin events
	EventCheckinCreated  EventType = "checkin.created"
	EventCheckinUpdated  EventType = "checkin.updated"
	EventCheckinDeleted  EventType = "checkin.deleted"
	EventCheckinStreakStarted EventType = "checkin.streak_started"
	EventCheckinStreakBroken EventType = "checkin.streak_broken"

	// Category events
	EventCategoryCreated EventType = "category.created"
	EventCategoryUpdated EventType = "category.updated"
	EventCategoryDeleted EventType = "category.deleted"

	// Subscription events
	EventSubscriptionCreated EventType = "subscription.created"
	EventSubscriptionUpgraded EventType = "subscription.upgraded"
	EventSubscriptionDowngraded EventType = "subscription.downgraded"
	EventSubscriptionCanceled EventType = "subscription.canceled"
	EventSubscriptionResumed EventType = "subscription.resumed"

	// Feature usage events
	EventFeatureUsed EventType = "feature.used"
	EventExportCreated EventType = "export.created"
	EventWebhookTriggered EventType = "webhook.triggered"
	EventAPICallMade EventType = "api.call_made"

	// Error events
	EventErrorOccurred EventType = "error.occurred"
	EventRateLimitHit EventType = "rate_limit.hit"
)

// Event represents an analytics event
type Event struct {
	ID         uuid.UUID              `json:"id" gorm:"type:uuid;primaryKey"`
	UserID     *uuid.UUID             `json:"user_id" gorm:"type:uuid;index"`
	EventType  EventType              `json:"event_type" gorm:"index"`
	EventData  map[string]interface{} `json:"event_data" gorm:"type:jsonb"`
	SessionID  string                 `json:"session_id" gorm:"index"`
	IP         string                 `json:"ip"`
	UserAgent  string                 `json:"user_agent"`
	Referrer   string                 `json:"referrer"`
	Timestamp  time.Time              `json:"timestamp" gorm:"index"`
	CreatedAt  time.Time              `json:"created_at"`
}

// TableName returns the table name for Event
func (Event) TableName() string {
	return "analytics_events"
}

// EventService handles analytics event tracking
type EventService struct {
	db     *gorm.DB
	redis  *redis.Client
	logger *zap.Logger
	queue  chan *Event
}

// NewEventService creates a new event service
func NewEventService(db *gorm.DB, redis *redis.Client, logger *zap.Logger) *EventService {
	service := &EventService{
		db:     db,
		redis:  redis,
		logger: logger,
		queue:  make(chan *Event, 1000), // Buffer up to 1000 events
	}

	// Start background worker for processing events
	go service.processEventQueue()

	return service
}

// TrackEvent tracks an analytics event
func (s *EventService) TrackEvent(ctx context.Context, event *Event) error {
	// Set defaults
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.CreatedAt = time.Now()

	// Send to queue for async processing
	select {
	case s.queue <- event:
		s.logger.Debug("Event queued",
			zap.String("event_type", string(event.EventType)),
			zap.String("event_id", event.ID.String()),
		)
	default:
		// Queue is full, process synchronously
		if err := s.saveEvent(ctx, event); err != nil {
			return err
		}
	}

	// Also update real-time metrics in Redis
	s.updateRealTimeMetrics(ctx, event)

	return nil
}

// processEventQueue processes queued events in the background
func (s *EventService) processEventQueue() {
	batch := make([]*Event, 0, 100)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-s.queue:
			batch = append(batch, event)

			// Process batch if it reaches size limit
			if len(batch) >= 100 {
				s.processBatch(batch)
				batch = make([]*Event, 0, 100)
			}

		case <-ticker.C:
			// Process batch periodically
			if len(batch) > 0 {
				s.processBatch(batch)
				batch = make([]*Event, 0, 100)
			}
		}
	}
}

// processBatch saves a batch of events to the database
func (s *EventService) processBatch(events []*Event) {
	if len(events) == 0 {
		return
	}

	ctx := context.Background()
	if err := s.db.WithContext(ctx).CreateInBatches(events, 100).Error; err != nil {
		s.logger.Error("Failed to save event batch",
			zap.Error(err),
			zap.Int("batch_size", len(events)),
		)

		// Try to save individually on batch failure
		for _, event := range events {
			if err := s.saveEvent(ctx, event); err != nil {
				s.logger.Error("Failed to save individual event",
					zap.Error(err),
					zap.String("event_id", event.ID.String()),
				)
			}
		}
	} else {
		s.logger.Debug("Event batch saved",
			zap.Int("batch_size", len(events)),
		)
	}
}

// saveEvent saves a single event to the database
func (s *EventService) saveEvent(ctx context.Context, event *Event) error {
	if err := s.db.WithContext(ctx).Create(event).Error; err != nil {
		s.logger.Error("Failed to save event",
			zap.Error(err),
			zap.String("event_type", string(event.EventType)),
		)
		return fmt.Errorf("failed to save event: %w", err)
	}
	return nil
}

// updateRealTimeMetrics updates real-time metrics in Redis
func (s *EventService) updateRealTimeMetrics(ctx context.Context, event *Event) {
	now := time.Now()

	// Update hourly counter
	hourKey := fmt.Sprintf("analytics:events:%s:hour:%s",
		event.EventType,
		now.Format("2006010215"),
	)
	s.redis.Incr(ctx, hourKey)
	s.redis.Expire(ctx, hourKey, 25*time.Hour) // Keep for a day

	// Update daily counter
	dayKey := fmt.Sprintf("analytics:events:%s:day:%s",
		event.EventType,
		now.Format("20060102"),
	)
	s.redis.Incr(ctx, dayKey)
	s.redis.Expire(ctx, dayKey, 31*24*time.Hour) // Keep for a month

	// Update user-specific metrics if applicable
	if event.UserID != nil {
		userKey := fmt.Sprintf("analytics:user:%s:events:%s",
			event.UserID.String(),
			now.Format("20060102"),
		)
		s.redis.HIncrBy(ctx, userKey, string(event.EventType), 1)
		s.redis.Expire(ctx, userKey, 31*24*time.Hour)
	}

	// Update global counters
	s.redis.HIncrBy(ctx, "analytics:totals", string(event.EventType), 1)
}

// GetEventCount gets the count of events for a time period
func (s *EventService) GetEventCount(ctx context.Context, eventType EventType, start, end time.Time) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&Event{}).
		Where("event_type = ? AND timestamp BETWEEN ? AND ?", eventType, start, end).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to get event count: %w", err)
	}

	return count, nil
}

// GetUserEvents gets events for a specific user
func (s *EventService) GetUserEvents(ctx context.Context, userID uuid.UUID, limit int) ([]*Event, error) {
	var events []*Event
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&events).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get user events: %w", err)
	}

	return events, nil
}

// GetRecentEvents gets recent events of a specific type
func (s *EventService) GetRecentEvents(ctx context.Context, eventType EventType, limit int) ([]*Event, error) {
	var events []*Event
	query := s.db.WithContext(ctx).Order("timestamp DESC").Limit(limit)

	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get recent events: %w", err)
	}

	return events, nil
}

// GetRealTimeMetrics gets real-time metrics from Redis
func (s *EventService) GetRealTimeMetrics(ctx context.Context) (map[string]interface{}, error) {
	metrics := make(map[string]interface{})

	// Get current hour metrics
	now := time.Now()
	hourPattern := fmt.Sprintf("analytics:events:*:hour:%s", now.Format("2006010215"))
	hourKeys, err := s.redis.Keys(ctx, hourPattern).Result()
	if err != nil {
		s.logger.Warn("Failed to get hourly metrics", zap.Error(err))
	}

	hourlyMetrics := make(map[string]int64)
	for _, key := range hourKeys {
		val, _ := s.redis.Get(ctx, key).Int64()
		// Extract event type from key
		eventType := extractEventType(key)
		hourlyMetrics[eventType] += val
	}
	metrics["hourly"] = hourlyMetrics

	// Get daily metrics
	dayPattern := fmt.Sprintf("analytics:events:*:day:%s", now.Format("20060102"))
	dayKeys, err := s.redis.Keys(ctx, dayPattern).Result()
	if err != nil {
		s.logger.Warn("Failed to get daily metrics", zap.Error(err))
	}

	dailyMetrics := make(map[string]int64)
	for _, key := range dayKeys {
		val, _ := s.redis.Get(ctx, key).Int64()
		eventType := extractEventType(key)
		dailyMetrics[eventType] += val
	}
	metrics["daily"] = dailyMetrics

	// Get total metrics
	totals, err := s.redis.HGetAll(ctx, "analytics:totals").Result()
	if err != nil {
		s.logger.Warn("Failed to get total metrics", zap.Error(err))
	}
	metrics["totals"] = totals

	return metrics, nil
}

// extractEventType extracts event type from Redis key
func extractEventType(key string) string {
	// Key format: analytics:events:{eventType}:hour:{timestamp}
	// Extract the eventType part
	parts := []byte(key)
	start := len("analytics:events:")
	end := start
	for i := start; i < len(parts); i++ {
		if parts[i] == ':' {
			end = i
			break
		}
	}
	if end > start {
		return string(parts[start:end])
	}
	return "unknown"
}

// CleanupOldEvents removes events older than retention period
func (s *EventService) CleanupOldEvents(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	result := s.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&Event{})

	if result.Error != nil {
		return fmt.Errorf("failed to cleanup old events: %w", result.Error)
	}

	s.logger.Info("Cleaned up old events",
		zap.Int64("deleted_count", result.RowsAffected),
		zap.Time("cutoff", cutoff),
	)

	return nil
}

// EventBuilder helps build events with common fields
type EventBuilder struct {
	event *Event
}

// NewEventBuilder creates a new event builder
func NewEventBuilder(eventType EventType) *EventBuilder {
	return &EventBuilder{
		event: &Event{
			ID:        uuid.New(),
			EventType: eventType,
			EventData: make(map[string]interface{}),
			Timestamp: time.Now(),
		},
	}
}

// WithUser sets the user ID
func (b *EventBuilder) WithUser(userID uuid.UUID) *EventBuilder {
	b.event.UserID = &userID
	return b
}

// WithSession sets the session ID
func (b *EventBuilder) WithSession(sessionID string) *EventBuilder {
	b.event.SessionID = sessionID
	return b
}

// WithData adds event data
func (b *EventBuilder) WithData(key string, value interface{}) *EventBuilder {
	b.event.EventData[key] = value
	return b
}

// WithIP sets the IP address
func (b *EventBuilder) WithIP(ip string) *EventBuilder {
	b.event.IP = ip
	return b
}

// WithUserAgent sets the user agent
func (b *EventBuilder) WithUserAgent(userAgent string) *EventBuilder {
	b.event.UserAgent = userAgent
	return b
}

// WithReferrer sets the referrer
func (b *EventBuilder) WithReferrer(referrer string) *EventBuilder {
	b.event.Referrer = referrer
	return b
}

// Build returns the built event
func (b *EventBuilder) Build() *Event {
	return b.event
}
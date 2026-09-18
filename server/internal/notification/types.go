package notification

import (
	"time"

	"github.com/google/uuid"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	// NotificationTypeEmail represents email notifications
	NotificationTypeEmail NotificationType = "email"

	// NotificationTypePush represents push notifications
	NotificationTypePush NotificationType = "push"

	// NotificationTypeReminder represents check-in reminders
	NotificationTypeReminder NotificationType = "reminder"

	// NotificationTypeAlert represents system alerts
	NotificationTypeAlert NotificationType = "alert"

	// NotificationTypeDigest represents digest notifications
	NotificationTypeDigest NotificationType = "digest"

	// NotificationTypeSubscription represents subscription-related notifications
	NotificationTypeSubscription NotificationType = "subscription"

	// NotificationTypePayment represents payment-related notifications
	NotificationTypePayment NotificationType = "payment"
)

// Priority represents notification priority level
type Priority string

const (
	// PriorityUrgent represents urgent notifications
	PriorityUrgent Priority = "urgent"

	// PriorityHigh represents high priority notifications
	PriorityHigh Priority = "high"

	// PriorityNormal represents normal priority notifications
	PriorityNormal Priority = "normal"

	// PriorityLow represents low priority notifications
	PriorityLow Priority = "low"
)

// NotificationStatus represents the status of a notification
type NotificationStatus string

const (
	// StatusPending notification is pending
	StatusPending NotificationStatus = "pending"

	// StatusQueued notification is queued for delivery
	StatusQueued NotificationStatus = "queued"

	// StatusSending notification is being sent
	StatusSending NotificationStatus = "sending"

	// StatusSent notification has been sent
	StatusSent NotificationStatus = "sent"

	// StatusFailed notification failed to send
	StatusFailed NotificationStatus = "failed"

	// StatusCancelled notification was cancelled
	StatusCancelled NotificationStatus = "cancelled"

	// StatusDeferred notification was deferred due to DND
	StatusDeferred NotificationStatus = "deferred"
)

// Notification represents a notification to be sent
type Notification struct {
	ID           uuid.UUID          `json:"id" db:"id"`
	UserID       uuid.UUID          `json:"user_id" db:"user_id"`
	Type         NotificationType   `json:"type" db:"type"`
	Priority     Priority           `json:"priority" db:"priority"`
	Status       NotificationStatus `json:"status" db:"status"`
	Title        string             `json:"title" db:"title"`
	Body         string             `json:"body" db:"body"`
	Data         map[string]interface{} `json:"data,omitempty" db:"data"`
	ScheduledFor time.Time         `json:"scheduled_for" db:"scheduled_for"`
	SentAt       *time.Time        `json:"sent_at,omitempty" db:"sent_at"`
	FailedAt     *time.Time        `json:"failed_at,omitempty" db:"failed_at"`
	RetryCount   int               `json:"retry_count" db:"retry_count"`
	MaxRetries   int               `json:"max_retries" db:"max_retries"`
	Error        *string           `json:"error,omitempty" db:"error"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`

	// Channel-specific fields
	EmailTo      *string `json:"email_to,omitempty" db:"email_to"`
	EmailFrom    *string `json:"email_from,omitempty" db:"email_from"`
	EmailReplyTo *string `json:"email_reply_to,omitempty" db:"email_reply_to"`

	// Push notification fields
	DeviceToken  *string `json:"device_token,omitempty" db:"device_token"`
	PushProvider *string `json:"push_provider,omitempty" db:"push_provider"`

	// Tracking
	OpenedAt     *time.Time `json:"opened_at,omitempty" db:"opened_at"`
	ClickedAt    *time.Time `json:"clicked_at,omitempty" db:"clicked_at"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at,omitempty" db:"unsubscribed_at"`
}

// NotificationTemplate represents a reusable notification template
type NotificationTemplate struct {
	ID          uuid.UUID        `json:"id" db:"id"`
	Name        string           `json:"name" db:"name"`
	Type        NotificationType `json:"type" db:"type"`
	Subject     string           `json:"subject" db:"subject"`
	BodyHTML    string           `json:"body_html" db:"body_html"`
	BodyText    string           `json:"body_text" db:"body_text"`
	Variables   []string         `json:"variables" db:"variables"`
	IsActive    bool             `json:"is_active" db:"is_active"`
	CreatedAt   time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at" db:"updated_at"`
}

// NotificationPreference represents user's channel preferences
type NotificationPreference struct {
	UserID          uuid.UUID `json:"user_id" db:"user_id"`
	Channel         string    `json:"channel" db:"channel"` // email, push, sms
	Enabled         bool      `json:"enabled" db:"enabled"`
	FrequencyLimit  int       `json:"frequency_limit" db:"frequency_limit"` // Max notifications per hour
	QuietHoursStart *string   `json:"quiet_hours_start,omitempty" db:"quiet_hours_start"`
	QuietHoursEnd   *string   `json:"quiet_hours_end,omitempty" db:"quiet_hours_end"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// NotificationLog represents a log entry for notification events
type NotificationLog struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	NotificationID uuid.UUID  `json:"notification_id" db:"notification_id"`
	Event          string     `json:"event" db:"event"`
	Details        string     `json:"details" db:"details"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

// DNDOverride represents a temporary override to DND settings
type DNDOverride struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	StartTime time.Time  `json:"start_time" db:"start_time"`
	EndTime   time.Time  `json:"end_time" db:"end_time"`
	Reason    string     `json:"reason" db:"reason"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// BatchNotification represents a batch of notifications to be processed
type BatchNotification struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	Name          string          `json:"name" db:"name"`
	Notifications []*Notification `json:"notifications"`
	TotalCount    int             `json:"total_count" db:"total_count"`
	ProcessedCount int            `json:"processed_count" db:"processed_count"`
	FailedCount   int             `json:"failed_count" db:"failed_count"`
	Status        string          `json:"status" db:"status"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
}

// IsValidNotificationType checks if a notification type is valid
func IsValidNotificationType(t NotificationType) bool {
	switch t {
	case NotificationTypeEmail, NotificationTypePush, NotificationTypeReminder,
	     NotificationTypeAlert, NotificationTypeDigest, NotificationTypeSubscription,
	     NotificationTypePayment:
		return true
	default:
		return false
	}
}

// IsValidPriority checks if a priority is valid
func IsValidPriority(p Priority) bool {
	switch p {
	case PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow:
		return true
	default:
		return false
	}
}

// IsValidStatus checks if a notification status is valid
func IsValidStatus(s NotificationStatus) bool {
	switch s {
	case StatusPending, StatusQueued, StatusSending, StatusSent,
	     StatusFailed, StatusCancelled, StatusDeferred:
		return true
	default:
		return false
	}
}

// ShouldRetry determines if a notification should be retried
func (n *Notification) ShouldRetry() bool {
	return n.Status == StatusFailed && n.RetryCount < n.MaxRetries
}

// CanBeSent determines if a notification can be sent
func (n *Notification) CanBeSent() bool {
	return n.Status == StatusPending || n.Status == StatusQueued || n.Status == StatusDeferred
}

// IsTerminal determines if a notification is in a terminal state
func (n *Notification) IsTerminal() bool {
	return n.Status == StatusSent || n.Status == StatusCancelled ||
		(n.Status == StatusFailed && n.RetryCount >= n.MaxRetries)
}
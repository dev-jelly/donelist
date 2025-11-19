package apikey

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Scope represents an API key permission scope
type Scope string

const (
	// Read scopes
	ScopeCheckinsRead    Scope = "checkins:read"
	ScopeTimelineRead    Scope = "timeline:read"
	ScopeStatisticsRead  Scope = "statistics:read"
	ScopeCategoriesRead  Scope = "categories:read"
	ScopeTagsRead        Scope = "tags:read"
	ScopeSearchRead      Scope = "search:read"
	ScopeCalendarRead    Scope = "calendar:read"

	// Write scopes
	ScopeCheckinsWrite   Scope = "checkins:write"
	ScopeCategoriesWrite Scope = "categories:write"
	ScopeTagsWrite       Scope = "tags:write"

	// Admin scopes
	ScopeAPIKeysManage   Scope = "api_keys:manage"
	ScopeUserRead        Scope = "user:read"
	ScopeUserWrite       Scope = "user:write"
)

// AllScopes returns all available scopes
var AllScopes = []Scope{
	ScopeCheckinsRead, ScopeTimelineRead, ScopeStatisticsRead,
	ScopeCategoriesRead, ScopeTagsRead, ScopeSearchRead, ScopeCalendarRead,
	ScopeCheckinsWrite, ScopeCategoriesWrite, ScopeTagsWrite,
	ScopeAPIKeysManage, ScopeUserRead, ScopeUserWrite,
}

// APIKey represents an API key in the database
type APIKey struct {
	ID               uuid.UUID      `db:"id" json:"id"`
	UserID           uuid.UUID      `db:"user_id" json:"user_id"`
	Name             string         `db:"name" json:"name"`
	KeyPrefix        string         `db:"key_prefix" json:"key_prefix"`
	KeyHash          string         `db:"key_hash" json:"-"`
	Scopes           pq.StringArray `db:"scopes" json:"scopes"`
	RateLimitPerDay  int            `db:"rate_limit_per_day" json:"rate_limit_per_day"`
	RateLimitPerHour int            `db:"rate_limit_per_hour" json:"rate_limit_per_hour"`
	LastUsedAt       *time.Time     `db:"last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt        *time.Time     `db:"expires_at" json:"expires_at,omitempty"`
	Revoked          bool           `db:"revoked" json:"revoked"`
	RevokedAt        *time.Time     `db:"revoked_at" json:"revoked_at,omitempty"`
	RevokedReason    *string        `db:"revoked_reason" json:"revoked_reason,omitempty"`
	CreatedAt        time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at" json:"updated_at"`
}

// CreateAPIKeyInput represents input for creating an API key
type CreateAPIKeyInput struct {
	UserID           uuid.UUID
	Name             string
	Scopes           []Scope
	RateLimitPerDay  int
	RateLimitPerHour int
	ExpiresAt        *time.Time
}

// UpdateAPIKeyInput represents input for updating an API key
type UpdateAPIKeyInput struct {
	Name             *string
	Scopes           []Scope
	RateLimitPerDay  *int
	RateLimitPerHour *int
	ExpiresAt        *time.Time
}

// APIKeyWithPlainText represents an API key with its plain text value
// Only returned when creating a new key
type APIKeyWithPlainText struct {
	APIKey
	PlainTextKey string `json:"plain_text_key"`
}

// APIKeyUsage represents a usage record for an API key
type APIKeyUsage struct {
	ID             uuid.UUID  `db:"id" json:"id"`
	APIKeyID       uuid.UUID  `db:"api_key_id" json:"api_key_id"`
	Endpoint       string     `db:"endpoint" json:"endpoint"`
	Method         string     `db:"method" json:"method"`
	StatusCode     int        `db:"status_code" json:"status_code"`
	ResponseTimeMs *int       `db:"response_time_ms" json:"response_time_ms,omitempty"`
	IPAddress      *string    `db:"ip_address" json:"ip_address,omitempty"`
	UserAgent      *string    `db:"user_agent" json:"user_agent,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
}

// RateLimitInfo represents rate limit information for an API key
type RateLimitInfo struct {
	HourlyUsed     int       `json:"hourly_used"`
	HourlyLimit    int       `json:"hourly_limit"`
	HourlyRemaining int      `json:"hourly_remaining"`
	DailyUsed      int       `json:"daily_used"`
	DailyLimit     int       `json:"daily_limit"`
	DailyRemaining int       `json:"daily_remaining"`
	ResetAt        time.Time `json:"reset_at"`
}

// UsageStatistics represents usage statistics for an API key
type UsageStatistics struct {
	TotalRequests   int64                  `json:"total_requests"`
	RequestsByDay   map[string]int64       `json:"requests_by_day"`
	RequestsByEndpoint map[string]int64    `json:"requests_by_endpoint"`
	AverageResponseTime float64            `json:"average_response_time_ms"`
}

// IsExpired checks if the API key is expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsValid checks if the API key is valid (not revoked and not expired)
func (k *APIKey) IsValid() bool {
	return !k.Revoked && !k.IsExpired()
}

// HasScope checks if the API key has a specific scope
func (k *APIKey) HasScope(scope Scope) bool {
	for _, s := range k.Scopes {
		if Scope(s) == scope {
			return true
		}
	}
	return false
}

// HasAnyScope checks if the API key has any of the specified scopes
func (k *APIKey) HasAnyScope(scopes ...Scope) bool {
	for _, scope := range scopes {
		if k.HasScope(scope) {
			return true
		}
	}
	return false
}

// ValidateScopes validates that all provided scopes are valid
func ValidateScopes(scopes []Scope) error {
	validScopes := make(map[Scope]bool)
	for _, s := range AllScopes {
		validScopes[s] = true
	}

	for _, scope := range scopes {
		if !validScopes[scope] {
			return ErrInvalidScope
		}
	}
	return nil
}

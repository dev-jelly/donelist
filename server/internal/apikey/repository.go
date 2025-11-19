package apikey

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Repository handles API key database operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new API key repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new API key
func (r *Repository) Create(ctx context.Context, input CreateAPIKeyInput, keyPrefix, keyHash string) (*APIKey, error) {
	query := `
		INSERT INTO api_keys (
			user_id, name, key_prefix, key_hash, scopes,
			rate_limit_per_day, rate_limit_per_hour, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, name, key_prefix, key_hash, scopes,
			rate_limit_per_day, rate_limit_per_hour, last_used_at, expires_at,
			revoked, revoked_at, revoked_reason, created_at, updated_at
	`

	var apiKey APIKey
	err := r.db.GetContext(ctx, &apiKey, query,
		input.UserID,
		input.Name,
		keyPrefix,
		keyHash,
		pq.Array(scopesToStrings(input.Scopes)),
		input.RateLimitPerDay,
		input.RateLimitPerHour,
		input.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create api key: %w", err)
	}

	return &apiKey, nil
}

// GetByID retrieves an API key by ID
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_prefix, key_hash, scopes,
			rate_limit_per_day, rate_limit_per_hour, last_used_at, expires_at,
			revoked, revoked_at, revoked_reason, created_at, updated_at
		FROM api_keys
		WHERE id = $1
	`

	var apiKey APIKey
	err := r.db.GetContext(ctx, &apiKey, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to get api key: %w", err)
	}

	return &apiKey, nil
}

// GetByHash retrieves an API key by its hash
func (r *Repository) GetByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_prefix, key_hash, scopes,
			rate_limit_per_day, rate_limit_per_hour, last_used_at, expires_at,
			revoked, revoked_at, revoked_reason, created_at, updated_at
		FROM api_keys
		WHERE key_hash = $1
	`

	var apiKey APIKey
	err := r.db.GetContext(ctx, &apiKey, query, keyHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to get api key by hash: %w", err)
	}

	return &apiKey, nil
}

// ListByUserID retrieves all API keys for a user
func (r *Repository) ListByUserID(ctx context.Context, userID uuid.UUID, includeRevoked bool) ([]APIKey, error) {
	query := `
		SELECT id, user_id, name, key_prefix, key_hash, scopes,
			rate_limit_per_day, rate_limit_per_hour, last_used_at, expires_at,
			revoked, revoked_at, revoked_reason, created_at, updated_at
		FROM api_keys
		WHERE user_id = $1
	`

	if !includeRevoked {
		query += " AND revoked = false"
	}

	query += " ORDER BY created_at DESC"

	var apiKeys []APIKey
	err := r.db.SelectContext(ctx, &apiKeys, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}

	if apiKeys == nil {
		apiKeys = []APIKey{}
	}

	return apiKeys, nil
}

// Update updates an API key
func (r *Repository) Update(ctx context.Context, id uuid.UUID, input UpdateAPIKeyInput) (*APIKey, error) {
	query := `
		UPDATE api_keys
		SET name = COALESCE($2, name),
			scopes = COALESCE($3, scopes),
			rate_limit_per_day = COALESCE($4, rate_limit_per_day),
			rate_limit_per_hour = COALESCE($5, rate_limit_per_hour),
			expires_at = COALESCE($6, expires_at),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING id, user_id, name, key_prefix, key_hash, scopes,
			rate_limit_per_day, rate_limit_per_hour, last_used_at, expires_at,
			revoked, revoked_at, revoked_reason, created_at, updated_at
	`

	var scopesArray interface{}
	if input.Scopes != nil {
		scopesArray = pq.Array(scopesToStrings(input.Scopes))
	}

	var apiKey APIKey
	err := r.db.GetContext(ctx, &apiKey, query,
		id,
		input.Name,
		scopesArray,
		input.RateLimitPerDay,
		input.RateLimitPerHour,
		input.ExpiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to update api key: %w", err)
	}

	return &apiKey, nil
}

// Revoke revokes an API key
func (r *Repository) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
	query := `
		UPDATE api_keys
		SET revoked = true,
			revoked_at = CURRENT_TIMESTAMP,
			revoked_reason = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query, id, reason)
	if err != nil {
		return fmt.Errorf("failed to revoke api key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// Delete permanently deletes an API key
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM api_keys WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete api key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// UpdateLastUsed updates the last_used_at timestamp
func (r *Repository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE api_keys
		SET last_used_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	return nil
}

// RecordUsage records an API key usage
func (r *Repository) RecordUsage(ctx context.Context, usage APIKeyUsage) error {
	query := `
		INSERT INTO api_key_usage (
			api_key_id, endpoint, method, status_code,
			response_time_ms, ip_address, user_agent
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.ExecContext(ctx, query,
		usage.APIKeyID,
		usage.Endpoint,
		usage.Method,
		usage.StatusCode,
		usage.ResponseTimeMs,
		usage.IPAddress,
		usage.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}

	return nil
}

// GetRateLimitUsage retrieves current rate limit usage
func (r *Repository) GetRateLimitUsage(ctx context.Context, apiKeyID uuid.UUID) (*RateLimitInfo, *APIKey, error) {
	// Get API key details for limits
	apiKey, err := r.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	hourWindow := fmt.Sprintf("hour:%s", now.Format("2006-01-02:15"))
	dayWindow := fmt.Sprintf("day:%s", now.Format("2006-01-02"))

	query := `
		SELECT time_window, request_count
		FROM api_key_rate_limits
		WHERE api_key_id = $1 AND time_window IN ($2, $3)
	`

	var records []struct {
		TimeWindow   string `db:"time_window"`
		RequestCount int    `db:"request_count"`
	}

	err = r.db.SelectContext(ctx, &records, query, apiKeyID, hourWindow, dayWindow)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get rate limit usage: %w", err)
	}

	hourlyUsed := 0
	dailyUsed := 0

	for _, record := range records {
		if record.TimeWindow == hourWindow {
			hourlyUsed = record.RequestCount
		} else if record.TimeWindow == dayWindow {
			dailyUsed = record.RequestCount
		}
	}

	resetAt := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())

	info := &RateLimitInfo{
		HourlyUsed:      hourlyUsed,
		HourlyLimit:     apiKey.RateLimitPerHour,
		HourlyRemaining: max(0, apiKey.RateLimitPerHour-hourlyUsed),
		DailyUsed:       dailyUsed,
		DailyLimit:      apiKey.RateLimitPerDay,
		DailyRemaining:  max(0, apiKey.RateLimitPerDay-dailyUsed),
		ResetAt:         resetAt,
	}

	return info, apiKey, nil
}

// IncrementRateLimit increments the rate limit counter
func (r *Repository) IncrementRateLimit(ctx context.Context, apiKeyID uuid.UUID) error {
	now := time.Now()
	hourWindow := fmt.Sprintf("hour:%s", now.Format("2006-01-02:15"))
	dayWindow := fmt.Sprintf("day:%s", now.Format("2006-01-02"))

	query := `
		INSERT INTO api_key_rate_limits (api_key_id, time_window, request_count)
		VALUES ($1, $2, 1)
		ON CONFLICT (api_key_id, time_window)
		DO UPDATE SET
			request_count = api_key_rate_limits.request_count + 1,
			updated_at = CURRENT_TIMESTAMP
	`

	// Increment both hour and day counters
	for _, window := range []string{hourWindow, dayWindow} {
		_, err := r.db.ExecContext(ctx, query, apiKeyID, window)
		if err != nil {
			return fmt.Errorf("failed to increment rate limit: %w", err)
		}
	}

	return nil
}

// GetUsageStatistics retrieves usage statistics for an API key
func (r *Repository) GetUsageStatistics(ctx context.Context, apiKeyID uuid.UUID, days int) (*UsageStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			DATE(created_at) as request_date,
			endpoint,
			AVG(response_time_ms) as avg_response_time
		FROM api_key_usage
		WHERE api_key_id = $1
			AND created_at >= CURRENT_TIMESTAMP - INTERVAL '%d days'
		GROUP BY DATE(created_at), endpoint
	`

	var records []struct {
		TotalRequests   int64          `db:"total_requests"`
		RequestDate     *time.Time     `db:"request_date"`
		Endpoint        string         `db:"endpoint"`
		AvgResponseTime sql.NullFloat64 `db:"avg_response_time"`
	}

	err := r.db.SelectContext(ctx, &records, fmt.Sprintf(query, days), apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage statistics: %w", err)
	}

	stats := &UsageStatistics{
		RequestsByDay:      make(map[string]int64),
		RequestsByEndpoint: make(map[string]int64),
	}

	var totalResponseTime float64
	var responseTimeCount int64

	for _, record := range records {
		stats.TotalRequests += record.TotalRequests

		if record.RequestDate != nil {
			dateStr := record.RequestDate.Format("2006-01-02")
			stats.RequestsByDay[dateStr] += record.TotalRequests
		}

		stats.RequestsByEndpoint[record.Endpoint] += record.TotalRequests

		if record.AvgResponseTime.Valid {
			totalResponseTime += record.AvgResponseTime.Float64 * float64(record.TotalRequests)
			responseTimeCount += record.TotalRequests
		}
	}

	if responseTimeCount > 0 {
		stats.AverageResponseTime = totalResponseTime / float64(responseTimeCount)
	}

	return stats, nil
}

// Helper functions

func scopesToStrings(scopes []Scope) []string {
	strings := make([]string, len(scopes))
	for i, scope := range scopes {
		strings[i] = string(scope)
	}
	return strings
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

package premium

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

const (
	// GracePeriodDays is the number of days after expiration before features are restricted
	GracePeriodDays = 7
)

var (
	// ErrInsufficientTier is returned when user doesn't have required tier
	ErrInsufficientTier = errors.New("insufficient subscription tier")
	// ErrSubscriptionExpired is returned when subscription is expired
	ErrSubscriptionExpired = errors.New("subscription expired")
	// ErrFeatureNotAvailable is returned when feature is not available
	ErrFeatureNotAvailable = errors.New("feature not available")
	// ErrUsageLimitExceeded is returned when usage limit is exceeded
	ErrUsageLimitExceeded = errors.New("usage limit exceeded")
)

// Service handles premium feature checks and subscription management
type Service struct {
	db              *sqlx.DB
	userRepo        *user.Repository
	subscriptionRepo *subscription.Repository
	logger          *zap.Logger
}

// NewService creates a new premium service
func NewService(db *sqlx.DB, userRepo *user.Repository, subscriptionRepo *subscription.Repository, logger *zap.Logger) *Service {
	return &Service{
		db:              db,
		userRepo:        userRepo,
		subscriptionRepo: subscriptionRepo,
		logger:          logger,
	}
}

// TierInfo represents user's tier information
type TierInfo struct {
	Tier          Tier       `json:"tier"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	IsActive      bool       `json:"is_active"`
	IsInGrace     bool       `json:"is_in_grace"`
	GraceEndsAt   *time.Time `json:"grace_ends_at,omitempty"`
	Features      []Feature  `json:"features"`
	Limits        map[string]int `json:"limits"`
}

// GetUserTierInfo gets detailed tier information for a user
func (s *Service) GetUserTierInfo(ctx context.Context, userID uuid.UUID) (*TierInfo, error) {
	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get active subscription if any
	sub, err := s.subscriptionRepo.GetSubscriptionByUserID(ctx, userID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	info := &TierInfo{
		Tier:      TierFree,
		IsActive:  false,
		IsInGrace: false,
	}

	// Determine tier from subscription or user
	if sub != nil && sub.IsActive() {
		info.Tier = Tier(sub.PlanID)
		info.ExpiresAt = &sub.CurrentPeriodEnd
		info.IsActive = true
	} else if user.Tier != "" {
		info.Tier = Tier(user.Tier)
		info.ExpiresAt = user.TierExpiresAt

		// Check if tier is active
		if info.ExpiresAt == nil || info.ExpiresAt.After(time.Now()) {
			info.IsActive = true
		} else {
			// Check grace period
			gracePeriodEnd := info.ExpiresAt.Add(time.Hour * 24 * GracePeriodDays)
			if time.Now().Before(gracePeriodEnd) {
				info.IsInGrace = true
				info.GraceEndsAt = &gracePeriodEnd
			}
		}
	}

	// Get tier configuration
	tiers := DefaultTiers()
	if config, exists := tiers[info.Tier]; exists {
		info.Features = config.Features
		info.Limits = config.Limits
	}

	return info, nil
}

// CheckFeatureAccess checks if a user has access to a feature
func (s *Service) CheckFeatureAccess(ctx context.Context, userID uuid.UUID, feature Feature) error {
	info, err := s.GetUserTierInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get tier info: %w", err)
	}

	// Check if feature is available in tier
	if !HasFeature(info.Tier, feature) {
		return ErrFeatureNotAvailable
	}

	// Check if subscription is active or in grace period
	if !info.IsActive && !info.IsInGrace {
		return ErrSubscriptionExpired
	}

	// If in grace period, log warning
	if info.IsInGrace {
		s.logger.Warn("User accessing feature during grace period",
			zap.String("user_id", userID.String()),
			zap.String("feature", string(feature)),
			zap.Time("grace_ends", *info.GraceEndsAt),
		)
	}

	return nil
}

// CheckTierAccess checks if a user has the minimum required tier
func (s *Service) CheckTierAccess(ctx context.Context, userID uuid.UUID, requiredTier Tier) error {
	info, err := s.GetUserTierInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get tier info: %w", err)
	}

	// Check tier level
	if CompareTiers(info.Tier, requiredTier) < 0 {
		return ErrInsufficientTier
	}

	// Check if subscription is active or in grace period
	if !info.IsActive && !info.IsInGrace {
		return ErrSubscriptionExpired
	}

	return nil
}

// UsageInfo represents current usage information
type UsageInfo struct {
	LimitType string `json:"limit_type"`
	Current   int    `json:"current"`
	Limit     int    `json:"limit"`
	Unlimited bool   `json:"unlimited"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

// CheckUsageLimit checks if user is within usage limit
func (s *Service) CheckUsageLimit(ctx context.Context, userID uuid.UUID, limitType string) (*UsageInfo, error) {
	info, err := s.GetUserTierInfo(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tier info: %w", err)
	}

	// Get limit for tier
	limit := GetLimit(info.Tier, limitType)

	// Get current usage
	current, resetAt, err := s.getCurrentUsage(ctx, userID, limitType)
	if err != nil {
		return nil, fmt.Errorf("failed to get current usage: %w", err)
	}

	usageInfo := &UsageInfo{
		LimitType: limitType,
		Current:   current,
		Limit:     limit,
		Unlimited: limit == -1,
		ResetAt:   resetAt,
	}

	// Check if within limit
	if limit != -1 && current >= limit {
		return usageInfo, ErrUsageLimitExceeded
	}

	return usageInfo, nil
}

// getCurrentUsage gets current usage for a specific limit type
func (s *Service) getCurrentUsage(ctx context.Context, userID uuid.UUID, limitType string) (int, *time.Time, error) {
	var current int
	var resetAt *time.Time
	now := time.Now()

	switch limitType {
	case "checkins_per_day":
		query := `
			SELECT COUNT(*)
			FROM checkins
			WHERE user_id = $1
			AND created_at >= $2
			AND deleted_at IS NULL
		`
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		err := s.db.GetContext(ctx, &current, query, userID, startOfDay)
		if err != nil {
			return 0, nil, err
		}
		nextDay := startOfDay.Add(24 * time.Hour)
		resetAt = &nextDay

	case "checkins_per_month":
		query := `
			SELECT COUNT(*)
			FROM checkins
			WHERE user_id = $1
			AND created_at >= $2
			AND deleted_at IS NULL
		`
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		err := s.db.GetContext(ctx, &current, query, userID, startOfMonth)
		if err != nil {
			return 0, nil, err
		}
		nextMonth := startOfMonth.AddDate(0, 1, 0)
		resetAt = &nextMonth

	case "categories_count":
		query := `
			SELECT COUNT(*)
			FROM categories
			WHERE user_id = $1
			AND deleted_at IS NULL
		`
		err := s.db.GetContext(ctx, &current, query, userID)
		if err != nil {
			return 0, nil, err
		}

	case "webhooks_count":
		query := `
			SELECT COUNT(*)
			FROM webhooks
			WHERE user_id = $1
			AND deleted_at IS NULL
		`
		err := s.db.GetContext(ctx, &current, query, userID)
		if err != nil && err != sql.ErrNoRows {
			return 0, nil, err
		}

	case "api_tokens_count":
		query := `
			SELECT COUNT(*)
			FROM api_keys
			WHERE user_id = $1
			AND deleted_at IS NULL
		`
		err := s.db.GetContext(ctx, &current, query, userID)
		if err != nil && err != sql.ErrNoRows {
			return 0, nil, err
		}

	case "exports_per_month":
		query := `
			SELECT COUNT(*)
			FROM export_logs
			WHERE user_id = $1
			AND created_at >= $2
		`
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		err := s.db.GetContext(ctx, &current, query, userID, startOfMonth)
		if err != nil && err != sql.ErrNoRows {
			return 0, nil, err
		}
		nextMonth := startOfMonth.AddDate(0, 1, 0)
		resetAt = &nextMonth

	case "api_calls_per_hour":
		query := `
			SELECT COUNT(*)
			FROM api_logs
			WHERE user_id = $1
			AND created_at >= $2
		`
		oneHourAgo := now.Add(-time.Hour)
		err := s.db.GetContext(ctx, &current, query, userID, oneHourAgo)
		if err != nil && err != sql.ErrNoRows {
			return 0, nil, err
		}
		nextHour := now.Add(time.Hour)
		resetAt = &nextHour

	default:
		return 0, nil, fmt.Errorf("unknown limit type: %s", limitType)
	}

	return current, resetAt, nil
}

// IncrementUsage increments usage counter for a limit type
func (s *Service) IncrementUsage(ctx context.Context, userID uuid.UUID, limitType string) error {
	// First check if within limit
	_, err := s.CheckUsageLimit(ctx, userID, limitType)
	if err != nil {
		return err
	}

	// Log usage for certain types
	switch limitType {
	case "exports_per_month":
		query := `
			INSERT INTO export_logs (user_id, created_at)
			VALUES ($1, $2)
		`
		_, err = s.db.ExecContext(ctx, query, userID, time.Now())
		if err != nil {
			return fmt.Errorf("failed to log export: %w", err)
		}

	case "api_calls_per_hour":
		query := `
			INSERT INTO api_logs (user_id, endpoint, created_at)
			VALUES ($1, $2, $3)
		`
		_, err = s.db.ExecContext(ctx, query, userID, "api_call", time.Now())
		if err != nil {
			return fmt.Errorf("failed to log API call: %w", err)
		}
	}

	return nil
}

// UpgradeTier upgrades user to a new tier
func (s *Service) UpgradeTier(ctx context.Context, userID uuid.UUID, newTier Tier, duration time.Duration) error {
	// Get current tier info
	info, err := s.GetUserTierInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get current tier: %w", err)
	}

	// Check if upgrade is valid
	if !CanUpgrade(info.Tier, newTier) && info.Tier != newTier {
		return fmt.Errorf("cannot upgrade from %s to %s", info.Tier, newTier)
	}

	// Calculate expiration
	expiresAt := time.Now().Add(duration)

	// Update user tier
	err = s.userRepo.UpdateTier(ctx, userID, string(newTier), &expiresAt)
	if err != nil {
		return fmt.Errorf("failed to update tier: %w", err)
	}

	// Create subscription event
	err = s.createSubscriptionEvent(ctx, userID, "tier.upgraded", map[string]interface{}{
		"from_tier": string(info.Tier),
		"to_tier":   string(newTier),
		"expires_at": expiresAt,
	})
	if err != nil {
		s.logger.Warn("Failed to create subscription event",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
	}

	s.logger.Info("User tier upgraded",
		zap.String("user_id", userID.String()),
		zap.String("from_tier", string(info.Tier)),
		zap.String("to_tier", string(newTier)),
		zap.Time("expires_at", expiresAt),
	)

	return nil
}

// DowngradeTier downgrades user to a lower tier
func (s *Service) DowngradeTier(ctx context.Context, userID uuid.UUID, newTier Tier) error {
	// Get current tier info
	info, err := s.GetUserTierInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get current tier: %w", err)
	}

	// Check if downgrade is valid
	if !CanDowngrade(info.Tier, newTier) {
		return fmt.Errorf("cannot downgrade from %s to %s", info.Tier, newTier)
	}

	// For free tier, remove expiration
	var expiresAt *time.Time
	if newTier != TierFree {
		// Keep current expiration if downgrading to a paid tier
		expiresAt = info.ExpiresAt
	}

	// Update user tier
	err = s.userRepo.UpdateTier(ctx, userID, string(newTier), expiresAt)
	if err != nil {
		return fmt.Errorf("failed to update tier: %w", err)
	}

	// Create subscription event
	err = s.createSubscriptionEvent(ctx, userID, "tier.downgraded", map[string]interface{}{
		"from_tier": string(info.Tier),
		"to_tier":   string(newTier),
	})
	if err != nil {
		s.logger.Warn("Failed to create subscription event",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
	}

	s.logger.Info("User tier downgraded",
		zap.String("user_id", userID.String()),
		zap.String("from_tier", string(info.Tier)),
		zap.String("to_tier", string(newTier)),
	)

	return nil
}

// ExtendSubscription extends the user's subscription
func (s *Service) ExtendSubscription(ctx context.Context, userID uuid.UUID, duration time.Duration) error {
	// Get current tier info
	info, err := s.GetUserTierInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get tier info: %w", err)
	}

	if info.Tier == TierFree {
		return fmt.Errorf("cannot extend free tier")
	}

	// Calculate new expiration
	var newExpiresAt time.Time
	if info.ExpiresAt != nil && info.ExpiresAt.After(time.Now()) {
		// Extend from current expiration
		newExpiresAt = info.ExpiresAt.Add(duration)
	} else {
		// Extend from now if expired
		newExpiresAt = time.Now().Add(duration)
	}

	// Update user tier
	err = s.userRepo.UpdateTier(ctx, userID, string(info.Tier), &newExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to update tier: %w", err)
	}

	// Create subscription event
	err = s.createSubscriptionEvent(ctx, userID, "subscription.extended", map[string]interface{}{
		"tier":           string(info.Tier),
		"old_expires_at": info.ExpiresAt,
		"new_expires_at": newExpiresAt,
		"duration_days":  int(duration.Hours() / 24),
	})
	if err != nil {
		s.logger.Warn("Failed to create subscription event",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
	}

	s.logger.Info("Subscription extended",
		zap.String("user_id", userID.String()),
		zap.String("tier", string(info.Tier)),
		zap.Time("new_expires_at", newExpiresAt),
	)

	return nil
}

// createSubscriptionEvent creates a subscription event for audit logging
func (s *Service) createSubscriptionEvent(ctx context.Context, userID uuid.UUID, eventType string, data map[string]interface{}) error {
	query := `
		INSERT INTO subscription_events (user_id, event_type, data, created_at)
		VALUES ($1, $2, $3, $4)
	`

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, userID, eventType, dataJSON, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create subscription event: %w", err)
	}

	return nil
}

// GetSubscriptionEvents gets subscription events for a user
func (s *Service) GetSubscriptionEvents(ctx context.Context, userID uuid.UUID, limit int) ([]subscription.SubscriptionEvent, error) {
	query := `
		SELECT id, user_id, event_type, data, created_at
		FROM subscription_events
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	var events []subscription.SubscriptionEvent
	err := s.db.SelectContext(ctx, &events, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription events: %w", err)
	}

	return events, nil
}
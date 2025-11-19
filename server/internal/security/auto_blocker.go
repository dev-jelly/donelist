package security

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BlockReason represents the reason for blocking
type BlockReason string

const (
	BlockReasonRapidRequests     BlockReason = "rapid_requests"
	BlockReasonHighFailureRate   BlockReason = "high_failure_rate"
	BlockReasonBruteForce        BlockReason = "brute_force"
	BlockReasonGeographicAnomaly BlockReason = "geographic_anomaly"
	BlockReasonMultipleAnomalies BlockReason = "multiple_anomalies"
	BlockReasonManual            BlockReason = "manual"
)

// BlockStatus represents the status of a block
type BlockStatus string

const (
	BlockStatusActive   BlockStatus = "active"
	BlockStatusExpired  BlockStatus = "expired"
	BlockStatusReleased BlockStatus = "released"
)

// BlockEntry represents a blocked identifier
type BlockEntry struct {
	ID          uuid.UUID              `json:"id" gorm:"type:uuid;primaryKey"`
	Identifier  string                 `json:"identifier" gorm:"index"`
	IPAddress   string                 `json:"ip_address"`
	Reason      BlockReason            `json:"reason"`
	Description string                 `json:"description"`
	Severity    AnomalySeverity        `json:"severity"`
	Metadata    map[string]interface{} `json:"metadata" gorm:"type:jsonb"`
	Status      BlockStatus            `json:"status"`
	BlockedAt   time.Time              `json:"blocked_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
	ReleasedAt  *time.Time             `json:"released_at,omitempty"`
	ReleasedBy  *uuid.UUID             `json:"released_by,omitempty" gorm:"type:uuid"`
	Notes       string                 `json:"notes,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TableName returns the table name for BlockEntry
func (BlockEntry) TableName() string {
	return "security_blocks"
}

// AutoBlockerConfig holds configuration for auto blocker
type AutoBlockerConfig struct {
	DB          *gorm.DB
	RedisClient *redis.Client
	Logger      *zap.Logger

	// Auto-blocking rules
	EnableAutoBlock         bool
	AutoBlockOnCritical     bool          // Auto-block on critical severity
	AutoBlockOnHighCount    int           // Number of high severity anomalies to trigger block
	AutoBlockWindow         time.Duration // Time window to count anomalies

	// Block duration by severity
	CriticalBlockDuration time.Duration
	HighBlockDuration     time.Duration
	MediumBlockDuration   time.Duration
	LowBlockDuration      time.Duration

	// Whitelist
	WhitelistedIdentifiers []string
	WhitelistedIPs         []string
}

// AutoBlocker manages automatic blocking of suspicious identifiers
type AutoBlocker struct {
	config AutoBlockerConfig
	db     *gorm.DB
	redis  *redis.Client
	logger *zap.Logger
}

// NewAutoBlocker creates a new auto blocker
func NewAutoBlocker(config AutoBlockerConfig) *AutoBlocker {
	// Set defaults
	if config.AutoBlockOnHighCount == 0 {
		config.AutoBlockOnHighCount = 3
	}
	if config.AutoBlockWindow == 0 {
		config.AutoBlockWindow = 10 * time.Minute
	}
	if config.CriticalBlockDuration == 0 {
		config.CriticalBlockDuration = 24 * time.Hour
	}
	if config.HighBlockDuration == 0 {
		config.HighBlockDuration = 2 * time.Hour
	}
	if config.MediumBlockDuration == 0 {
		config.MediumBlockDuration = 30 * time.Minute
	}
	if config.LowBlockDuration == 0 {
		config.LowBlockDuration = 5 * time.Minute
	}

	return &AutoBlocker{
		config: config,
		db:     config.DB,
		redis:  config.RedisClient,
		logger: config.Logger,
	}
}

// EvaluateBlock evaluates if an identifier should be blocked based on anomaly event
func (b *AutoBlocker) EvaluateBlock(ctx context.Context, event *AnomalyEvent) (bool, error) {
	if !b.config.EnableAutoBlock {
		return false, nil
	}

	// Check whitelist
	if b.isWhitelisted(event.Identifier, event.IPAddress) {
		b.logger.Info("Identifier is whitelisted, skipping auto-block",
			zap.String("identifier", event.Identifier),
		)
		return false, nil
	}

	// Check if already blocked
	isBlocked, err := b.IsBlocked(ctx, event.Identifier)
	if err != nil {
		return false, err
	}
	if isBlocked {
		b.logger.Info("Identifier is already blocked",
			zap.String("identifier", event.Identifier),
		)
		return true, nil
	}

	// Auto-block on critical severity
	if b.config.AutoBlockOnCritical && event.Severity == SeverityCritical {
		return true, b.BlockIdentifier(ctx, event.Identifier, event.IPAddress, BlockReasonFromAnomaly(event.Type), event.Description, event.Severity, event.Metadata)
	}

	// Count recent high severity anomalies
	if event.Severity == SeverityHigh || event.Severity == SeverityCritical {
		count, err := b.countRecentAnomalies(ctx, event.Identifier)
		if err != nil {
			return false, err
		}

		if count >= b.config.AutoBlockOnHighCount {
			reason := BlockReasonMultipleAnomalies
			description := fmt.Sprintf("Multiple anomalies detected (%d in %v)", count, b.config.AutoBlockWindow)
			return true, b.BlockIdentifier(ctx, event.Identifier, event.IPAddress, reason, description, event.Severity, event.Metadata)
		}
	}

	return false, nil
}

// BlockIdentifier blocks an identifier
func (b *AutoBlocker) BlockIdentifier(ctx context.Context, identifier, ipAddress string, reason BlockReason, description string, severity AnomalySeverity, metadata map[string]interface{}) error {
	// Calculate expiration based on severity
	duration := b.getBlockDuration(severity)
	expiresAt := time.Now().Add(duration)

	// Create block entry
	entry := &BlockEntry{
		ID:          uuid.New(),
		Identifier:  identifier,
		IPAddress:   ipAddress,
		Reason:      reason,
		Description: description,
		Severity:    severity,
		Metadata:    metadata,
		Status:      BlockStatusActive,
		BlockedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save to database
	if err := b.db.WithContext(ctx).Create(entry).Error; err != nil {
		return fmt.Errorf("failed to create block entry: %w", err)
	}

	// Add to Redis for fast lookup
	blockKey := fmt.Sprintf("blocked:%s", identifier)
	err := b.redis.Set(ctx, blockKey, entry.ID.String(), duration).Err()
	if err != nil {
		b.logger.Error("Failed to set block in Redis", zap.Error(err))
		// Continue anyway, database is source of truth
	}

	b.logger.Warn("Identifier blocked",
		zap.String("identifier", identifier),
		zap.String("reason", string(reason)),
		zap.String("severity", string(severity)),
		zap.Time("expires_at", expiresAt),
	)

	return nil
}

// IsBlocked checks if an identifier is currently blocked
func (b *AutoBlocker) IsBlocked(ctx context.Context, identifier string) (bool, error) {
	// Fast check in Redis first
	blockKey := fmt.Sprintf("blocked:%s", identifier)
	exists, err := b.redis.Exists(ctx, blockKey).Result()
	if err != nil {
		b.logger.Error("Failed to check Redis block", zap.Error(err))
		// Fall through to database check
	} else if exists > 0 {
		return true, nil
	}

	// Check database for active blocks
	var count int64
	err = b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Where("identifier = ? AND status = ? AND expires_at > ?",
			identifier, BlockStatusActive, time.Now()).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check block status: %w", err)
	}

	return count > 0, nil
}

// GetBlockInfo gets block information for an identifier
func (b *AutoBlocker) GetBlockInfo(ctx context.Context, identifier string) (*BlockEntry, error) {
	var entry BlockEntry
	err := b.db.WithContext(ctx).
		Where("identifier = ? AND status = ? AND expires_at > ?",
			identifier, BlockStatusActive, time.Now()).
		Order("created_at DESC").
		First(&entry).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &entry, nil
}

// UnblockIdentifier manually unblocks an identifier
func (b *AutoBlocker) UnblockIdentifier(ctx context.Context, identifier string, releasedBy *uuid.UUID, notes string) error {
	now := time.Now()

	// Update database
	err := b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Where("identifier = ? AND status = ?", identifier, BlockStatusActive).
		Updates(map[string]interface{}{
			"status":      BlockStatusReleased,
			"released_at": now,
			"released_by": releasedBy,
			"notes":       notes,
			"updated_at":  now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to unblock identifier: %w", err)
	}

	// Remove from Redis
	blockKey := fmt.Sprintf("blocked:%s", identifier)
	b.redis.Del(ctx, blockKey)

	b.logger.Info("Identifier unblocked",
		zap.String("identifier", identifier),
		zap.String("notes", notes),
	)

	return nil
}

// AddToWhitelist adds an identifier to the whitelist
func (b *AutoBlocker) AddToWhitelist(ctx context.Context, identifier string) error {
	// Add to Redis set
	key := "security:whitelist:identifiers"
	err := b.redis.SAdd(ctx, key, identifier).Err()
	if err != nil {
		return fmt.Errorf("failed to add to whitelist: %w", err)
	}

	b.logger.Info("Identifier added to whitelist", zap.String("identifier", identifier))
	return nil
}

// RemoveFromWhitelist removes an identifier from the whitelist
func (b *AutoBlocker) RemoveFromWhitelist(ctx context.Context, identifier string) error {
	key := "security:whitelist:identifiers"
	err := b.redis.SRem(ctx, key, identifier).Err()
	if err != nil {
		return fmt.Errorf("failed to remove from whitelist: %w", err)
	}

	b.logger.Info("Identifier removed from whitelist", zap.String("identifier", identifier))
	return nil
}

// isWhitelisted checks if identifier or IP is whitelisted
func (b *AutoBlocker) isWhitelisted(identifier, ipAddress string) bool {
	// Check config whitelist
	for _, wl := range b.config.WhitelistedIdentifiers {
		if wl == identifier {
			return true
		}
	}
	for _, wl := range b.config.WhitelistedIPs {
		if wl == ipAddress {
			return true
		}
	}

	// Check Redis whitelist
	ctx := context.Background()
	key := "security:whitelist:identifiers"
	isMember, err := b.redis.SIsMember(ctx, key, identifier).Result()
	if err == nil && isMember {
		return true
	}

	key = "security:whitelist:ips"
	isMember, err = b.redis.SIsMember(ctx, key, ipAddress).Result()
	if err == nil && isMember {
		return true
	}

	return false
}

// countRecentAnomalies counts recent anomalies for an identifier
func (b *AutoBlocker) countRecentAnomalies(ctx context.Context, identifier string) (int, error) {
	key := fmt.Sprintf("anomaly:count:%s", identifier)

	// Increment counter
	count, err := b.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// Set expiry on first increment
	if count == 1 {
		b.redis.Expire(ctx, key, b.config.AutoBlockWindow)
	}

	return int(count), nil
}

// getBlockDuration returns block duration based on severity
func (b *AutoBlocker) getBlockDuration(severity AnomalySeverity) time.Duration {
	switch severity {
	case SeverityCritical:
		return b.config.CriticalBlockDuration
	case SeverityHigh:
		return b.config.HighBlockDuration
	case SeverityMedium:
		return b.config.MediumBlockDuration
	case SeverityLow:
		return b.config.LowBlockDuration
	default:
		return b.config.LowBlockDuration
	}
}

// BlockReasonFromAnomaly converts anomaly type to block reason
func BlockReasonFromAnomaly(anomalyType AnomalyType) BlockReason {
	switch anomalyType {
	case AnomalyTypeRapidRequests:
		return BlockReasonRapidRequests
	case AnomalyTypeHighFailureRate:
		return BlockReasonHighFailureRate
	case AnomalyTypeBruteForce:
		return BlockReasonBruteForce
	case AnomalyTypeGeographicAnomaly:
		return BlockReasonGeographicAnomaly
	default:
		return BlockReasonMultipleAnomalies
	}
}

// ListBlocks lists block entries with filtering
func (b *AutoBlocker) ListBlocks(ctx context.Context, opts ListBlocksOptions) ([]*BlockEntry, error) {
	query := b.db.WithContext(ctx).Model(&BlockEntry{})

	if opts.Identifier != nil {
		query = query.Where("identifier = ?", *opts.Identifier)
	}
	if opts.Status != nil {
		query = query.Where("status = ?", *opts.Status)
	}
	if opts.Reason != nil {
		query = query.Where("reason = ?", *opts.Reason)
	}
	if opts.ActiveOnly {
		query = query.Where("status = ? AND expires_at > ?", BlockStatusActive, time.Now())
	}
	if opts.StartDate != nil {
		query = query.Where("blocked_at >= ?", *opts.StartDate)
	}
	if opts.EndDate != nil {
		query = query.Where("blocked_at <= ?", *opts.EndDate)
	}

	if opts.Limit == 0 {
		opts.Limit = 100
	}

	var blocks []*BlockEntry
	err := query.
		Order("blocked_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset).
		Find(&blocks).Error

	return blocks, err
}

// ListBlocksOptions represents options for listing blocks
type ListBlocksOptions struct {
	Identifier *string
	Status     *BlockStatus
	Reason     *BlockReason
	ActiveOnly bool
	StartDate  *time.Time
	EndDate    *time.Time
	Limit      int
	Offset     int
}

// GetBlockStatistics returns statistics about blocks
func (b *AutoBlocker) GetBlockStatistics(ctx context.Context, since time.Time) (*BlockStatistics, error) {
	stats := &BlockStatistics{
		ByReason:   make(map[BlockReason]int64),
		BySeverity: make(map[AnomalySeverity]int64),
		ByStatus:   make(map[BlockStatus]int64),
	}

	// Count total blocks
	b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Where("blocked_at >= ?", since).
		Count(&stats.TotalBlocks)

	// Count active blocks
	b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Where("status = ? AND expires_at > ?", BlockStatusActive, time.Now()).
		Count(&stats.ActiveBlocks)

	// Count by reason
	var reasonCounts []struct {
		Reason BlockReason
		Count  int64
	}
	b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Select("reason, COUNT(*) as count").
		Where("blocked_at >= ?", since).
		Group("reason").
		Scan(&reasonCounts)

	for _, rc := range reasonCounts {
		stats.ByReason[rc.Reason] = rc.Count
	}

	// Count by severity
	var severityCounts []struct {
		Severity AnomalySeverity
		Count    int64
	}
	b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Select("severity, COUNT(*) as count").
		Where("blocked_at >= ?", since).
		Group("severity").
		Scan(&severityCounts)

	for _, sc := range severityCounts {
		stats.BySeverity[sc.Severity] = sc.Count
	}

	// Count by status
	var statusCounts []struct {
		Status BlockStatus
		Count  int64
	}
	b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Select("status, COUNT(*) as count").
		Where("blocked_at >= ?", since).
		Group("status").
		Scan(&statusCounts)

	for _, sc := range statusCounts {
		stats.ByStatus[sc.Status] = sc.Count
	}

	return stats, nil
}

// BlockStatistics represents block statistics
type BlockStatistics struct {
	TotalBlocks  int64                       `json:"total_blocks"`
	ActiveBlocks int64                       `json:"active_blocks"`
	ByReason     map[BlockReason]int64       `json:"by_reason"`
	BySeverity   map[AnomalySeverity]int64   `json:"by_severity"`
	ByStatus     map[BlockStatus]int64       `json:"by_status"`
}

// CleanupExpiredBlocks updates expired blocks
func (b *AutoBlocker) CleanupExpiredBlocks(ctx context.Context) error {
	now := time.Now()
	err := b.db.WithContext(ctx).
		Model(&BlockEntry{}).
		Where("status = ? AND expires_at <= ?", BlockStatusActive, now).
		Updates(map[string]interface{}{
			"status":     BlockStatusExpired,
			"updated_at": now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to cleanup expired blocks: %w", err)
	}

	return nil
}

// StartCleanupWorker starts a background worker to cleanup expired blocks
func (b *AutoBlocker) StartCleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.CleanupExpiredBlocks(ctx); err != nil {
				b.logger.Error("Failed to cleanup expired blocks", zap.Error(err))
			}
		}
	}
}

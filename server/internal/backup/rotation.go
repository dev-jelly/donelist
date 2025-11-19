package backup

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// RetentionPolicy defines backup retention rules
type RetentionPolicy struct {
	DailyRetentionDays    int
	WeeklyRetentionWeeks  int
	MonthlyRetentionMonths int
}

// RotationManager handles backup rotation and retention
type RotationManager struct {
	config  *Config
	storage *S3Storage
	logger  *zap.Logger
}

// NewRotationManager creates a new rotation manager
func NewRotationManager(config *Config, storage *S3Storage, logger *zap.Logger) *RotationManager {
	return &RotationManager{
		config:  config,
		storage: storage,
		logger:  logger,
	}
}

// BackupClassification represents how a backup is classified
type BackupClassification struct {
	Object       types.Object
	Age          time.Duration
	Category     string // "daily", "weekly", "monthly"
	ShouldKeep   bool
	KeepReason   string
	ShouldDelete bool
	DeleteReason string
}

// ApplyRetentionPolicy applies the retention policy to backups
func (rm *RotationManager) ApplyRetentionPolicy(ctx context.Context) error {
	rm.logger.Info("Applying retention policy",
		zap.Int("daily_retention_days", rm.config.RetentionDays),
		zap.Int("weekly_retention_weeks", rm.config.RetentionWeeks),
		zap.Int("monthly_retention_months", rm.config.RetentionMonths),
	)

	// Get all backups grouped by age
	backupsByAge, err := rm.storage.GetBackupsByAge(ctx)
	if err != nil {
		return fmt.Errorf("failed to get backups by age: %w", err)
	}

	// Classify backups
	classifications := rm.classifyBackups(backupsByAge)

	// Delete backups that should be removed
	var toDelete []string
	for _, classification := range classifications {
		if classification.ShouldDelete {
			toDelete = append(toDelete, *classification.Object.Key)
			rm.logger.Info("Marking backup for deletion",
				zap.String("key", *classification.Object.Key),
				zap.String("reason", classification.DeleteReason),
				zap.Duration("age", classification.Age),
			)
		}
	}

	if len(toDelete) > 0 {
		if err := rm.storage.DeleteMultipleBackups(ctx, toDelete); err != nil {
			return fmt.Errorf("failed to delete backups: %w", err)
		}
		rm.logger.Info("Deleted backups according to retention policy",
			zap.Int("count", len(toDelete)),
		)
	} else {
		rm.logger.Info("No backups to delete")
	}

	// Log retention summary
	rm.logRetentionSummary(classifications)

	return nil
}

// classifyBackups classifies backups based on retention policy
func (rm *RotationManager) classifyBackups(backupsByAge map[string][]types.Object) []BackupClassification {
	var classifications []BackupClassification
	now := time.Now()

	// Process daily backups (keep for RetentionDays days)
	dailyBackups := backupsByAge["daily"]
	for i, obj := range dailyBackups {
		age := now.Sub(*obj.LastModified)
		classification := BackupClassification{
			Object:   obj,
			Age:      age,
			Category: "daily",
		}

		// Keep the most recent backup of each day
		if i < rm.config.RetentionDays {
			classification.ShouldKeep = true
			classification.KeepReason = "within daily retention period"
		} else {
			classification.ShouldDelete = true
			classification.DeleteReason = "exceeds daily retention period"
		}

		classifications = append(classifications, classification)
	}

	// Process weekly backups (keep one per week for RetentionWeeks weeks)
	weeklyBackups := backupsByAge["weekly"]
	weeklyKeep := rm.selectWeeklyBackups(weeklyBackups, rm.config.RetentionWeeks)
	for _, obj := range weeklyBackups {
		age := now.Sub(*obj.LastModified)
		classification := BackupClassification{
			Object:   obj,
			Age:      age,
			Category: "weekly",
		}

		if weeklyKeep[*obj.Key] {
			classification.ShouldKeep = true
			classification.KeepReason = "selected as weekly backup"
		} else {
			classification.ShouldDelete = true
			classification.DeleteReason = "not selected as weekly backup"
		}

		classifications = append(classifications, classification)
	}

	// Process monthly backups (keep one per month for RetentionMonths months)
	monthlyBackups := backupsByAge["monthly"]
	monthlyKeep := rm.selectMonthlyBackups(monthlyBackups, rm.config.RetentionMonths)
	for _, obj := range monthlyBackups {
		age := now.Sub(*obj.LastModified)
		classification := BackupClassification{
			Object:   obj,
			Age:      age,
			Category: "monthly",
		}

		if monthlyKeep[*obj.Key] {
			classification.ShouldKeep = true
			classification.KeepReason = "selected as monthly backup"
		} else {
			classification.ShouldDelete = true
			classification.DeleteReason = "not selected as monthly backup"
		}

		classifications = append(classifications, classification)
	}

	// Process old backups (delete all)
	oldBackups := backupsByAge["old"]
	for _, obj := range oldBackups {
		age := now.Sub(*obj.LastModified)
		classifications = append(classifications, BackupClassification{
			Object:       obj,
			Age:          age,
			Category:     "old",
			ShouldDelete: true,
			DeleteReason: "exceeds maximum retention period",
		})
	}

	return classifications
}

// selectWeeklyBackups selects one backup per week to keep
func (rm *RotationManager) selectWeeklyBackups(backups []types.Object, weeksToKeep int) map[string]bool {
	keep := make(map[string]bool)

	// Group backups by week
	weekMap := make(map[string]types.Object)
	for _, obj := range backups {
		if obj.LastModified == nil {
			continue
		}

		// Get week identifier (year + week number)
		year, week := obj.LastModified.ISOWeek()
		weekKey := fmt.Sprintf("%d-W%02d", year, week)

		// Keep the newest backup for each week
		if existing, ok := weekMap[weekKey]; !ok || obj.LastModified.After(*existing.LastModified) {
			weekMap[weekKey] = obj
		}
	}

	// Sort weeks and keep only the most recent weeksToKeep weeks
	var weeks []string
	for week := range weekMap {
		weeks = append(weeks, week)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(weeks)))

	for i, week := range weeks {
		if i < weeksToKeep {
			keep[*weekMap[week].Key] = true
		}
	}

	return keep
}

// selectMonthlyBackups selects one backup per month to keep
func (rm *RotationManager) selectMonthlyBackups(backups []types.Object, monthsToKeep int) map[string]bool {
	keep := make(map[string]bool)

	// Group backups by month
	monthMap := make(map[string]types.Object)
	for _, obj := range backups {
		if obj.LastModified == nil {
			continue
		}

		// Get month identifier (year-month)
		monthKey := obj.LastModified.Format("2006-01")

		// Keep the newest backup for each month
		if existing, ok := monthMap[monthKey]; !ok || obj.LastModified.After(*existing.LastModified) {
			monthMap[monthKey] = obj
		}
	}

	// Sort months and keep only the most recent monthsToKeep months
	var months []string
	for month := range monthMap {
		months = append(months, month)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(months)))

	for i, month := range months {
		if i < monthsToKeep {
			keep[*monthMap[month].Key] = true
		}
	}

	return keep
}

// logRetentionSummary logs a summary of the retention policy application
func (rm *RotationManager) logRetentionSummary(classifications []BackupClassification) {
	summary := make(map[string]struct {
		total  int
		kept   int
		deleted int
	})

	for _, c := range classifications {
		s := summary[c.Category]
		s.total++
		if c.ShouldKeep {
			s.kept++
		}
		if c.ShouldDelete {
			s.deleted++
		}
		summary[c.Category] = s
	}

	rm.logger.Info("Retention policy summary")
	for category, s := range summary {
		rm.logger.Info("Category stats",
			zap.String("category", category),
			zap.Int("total", s.total),
			zap.Int("kept", s.kept),
			zap.Int("deleted", s.deleted),
		)
	}
}

// GetRetentionStats returns statistics about current backups and retention
func (rm *RotationManager) GetRetentionStats(ctx context.Context) (map[string]interface{}, error) {
	backupsByAge, err := rm.storage.GetBackupsByAge(ctx)
	if err != nil {
		return nil, err
	}

	classifications := rm.classifyBackups(backupsByAge)

	stats := map[string]interface{}{
		"total_backups": len(classifications),
		"by_category": map[string]int{
			"daily":   len(backupsByAge["daily"]),
			"weekly":  len(backupsByAge["weekly"]),
			"monthly": len(backupsByAge["monthly"]),
			"old":     len(backupsByAge["old"]),
		},
		"retention_policy": map[string]int{
			"daily_retention_days":     rm.config.RetentionDays,
			"weekly_retention_weeks":   rm.config.RetentionWeeks,
			"monthly_retention_months": rm.config.RetentionMonths,
		},
	}

	var totalSize int64
	var toDeleteCount int
	var toDeleteSize int64

	for _, c := range classifications {
		if c.Object.Size != nil {
			totalSize += *c.Object.Size
		}
		if c.ShouldDelete {
			toDeleteCount++
			if c.Object.Size != nil {
				toDeleteSize += *c.Object.Size
			}
		}
	}

	stats["total_size_bytes"] = totalSize
	stats["to_delete_count"] = toDeleteCount
	stats["to_delete_size_bytes"] = toDeleteSize

	return stats, nil
}

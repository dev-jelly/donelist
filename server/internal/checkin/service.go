package checkin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/premium"
	"github.com/dev-jelly/donelist/internal/tag"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/dev-jelly/donelist/internal/validation"
	"github.com/dev-jelly/donelist/internal/websocket"
	"go.uber.org/zap"
)

// SearchEventHandler handles search indexing events
type SearchEventHandler interface {
	OnCheckinCreated(ctx context.Context, checkinID, userID uuid.UUID, data map[string]interface{}) error
	OnCheckinUpdated(ctx context.Context, checkinID, userID uuid.UUID, data map[string]interface{}) error
	OnCheckinDeleted(ctx context.Context, checkinID, userID uuid.UUID) error
}

// NoOpSearchEventHandler is a no-op implementation
type NoOpSearchEventHandler struct{}

func (n *NoOpSearchEventHandler) OnCheckinCreated(ctx context.Context, checkinID, userID uuid.UUID, data map[string]interface{}) error {
	return nil
}

func (n *NoOpSearchEventHandler) OnCheckinUpdated(ctx context.Context, checkinID, userID uuid.UUID, data map[string]interface{}) error {
	return nil
}

func (n *NoOpSearchEventHandler) OnCheckinDeleted(ctx context.Context, checkinID, userID uuid.UUID) error {
	return nil
}

// Service handles check-in business logic
type Service struct {
	checkinRepo      *Repository
	categoryRepo     *category.Repository
	tagRepo          *tag.Repository
	userRepo         *user.Repository
	hub              *websocket.Hub
	cacheInvalidator CacheInvalidator
	searchHandler    SearchEventHandler
	logger           *zap.Logger
}

// NewService creates a new check-in service
func NewService(
	checkinRepo *Repository,
	categoryRepo *category.Repository,
	tagRepo *tag.Repository,
	userRepo *user.Repository,
	hub *websocket.Hub,
	logger *zap.Logger,
) *Service {
	return &Service{
		checkinRepo:      checkinRepo,
		categoryRepo:     categoryRepo,
		tagRepo:          tagRepo,
		userRepo:         userRepo,
		hub:              hub,
		cacheInvalidator: &NoOpCacheInvalidator{}, // Default to no-op
		searchHandler:    &NoOpSearchEventHandler{}, // Default to no-op
		logger:           logger,
	}
}

// SetCacheInvalidator sets the cache invalidator for this service
func (s *Service) SetCacheInvalidator(invalidator CacheInvalidator) {
	if invalidator != nil {
		s.cacheInvalidator = invalidator
	}
}

// SetSearchEventHandler sets the search event handler for this service
func (s *Service) SetSearchEventHandler(handler SearchEventHandler) {
	if handler != nil {
		s.searchHandler = handler
	}
}

// CreateInput represents service-level create input
type CreateInput struct {
	UserID          uuid.UUID
	CategoryID      *uuid.UUID
	Content         string
	CheckinTime     time.Time
	DurationMinutes int
	TagNames        []string
}

// UpdateInput represents service-level update input
type UpdateInput struct {
	Content    *string
	CategoryID *uuid.UUID
	TagNames   []string
	EditReason *string // Optional reason for the edit
	Version    int     // Current version for optimistic locking
}

// Create creates a new check-in
func (s *Service) Create(ctx context.Context, input CreateInput) (*Checkin, error) {
	// Validate content using validation package
	if err := validation.ValidateContent(input.Content); err != nil {
		return nil, err
	}

	// Normalize and validate tags
	normalizedTags := validation.NormalizeTags(input.TagNames)
	if err := validation.ValidateTags(normalizedTags); err != nil {
		return nil, err
	}
	input.TagNames = normalizedTags

	// Validate duration
	validDurations := map[int]bool{15: true, 30: true, 45: true, 120: true}
	if !validDurations[input.DurationMinutes] {
		return nil, fmt.Errorf("duration must be 15, 30, 45, or 120 minutes")
	}

	// Check for duplicate check-in at the same time
	isDuplicate, err := s.checkinRepo.CheckDuplicateAtTime(ctx, input.UserID, input.CheckinTime)
	if err != nil {
		s.logger.Error("Failed to check duplicate", zap.Error(err))
		return nil, fmt.Errorf("failed to validate check-in")
	}
	if isDuplicate {
		s.logger.Warn("Duplicate check-in attempt",
			zap.String("user_id", input.UserID.String()),
			zap.Time("checkin_time", input.CheckinTime),
		)
		return nil, NewDuplicateCheckinError(input.CheckinTime)
	}

	// Check check-in interval rules
	lastCheckin, err := s.checkinRepo.GetLastCheckin(ctx, input.UserID)
	if err != nil {
		s.logger.Error("Failed to get last check-in", zap.Error(err))
		return nil, fmt.Errorf("failed to validate check-in interval")
	}

	// If there's a previous check-in, validate the interval
	if lastCheckin != nil {
		requestedInterval := time.Duration(input.DurationMinutes) * time.Minute

		// Check if the user can check-in with the requested interval
		if !CanCheckinWithInterval(lastCheckin.CheckinTime, requestedInterval) {
			s.logger.Warn("Check-in interval violation",
				zap.String("user_id", input.UserID.String()),
				zap.Time("last_checkin_time", lastCheckin.CheckinTime),
				zap.Int("requested_interval_minutes", input.DurationMinutes),
			)

			// Return detailed interval error
			return nil, NewCheckinIntervalError(lastCheckin.CheckinTime, requestedInterval)
		}

		// Additional check: ensure minimum time has passed
		timeUntilEligible := GetTimeUntilNextCheckin(lastCheckin.CheckinTime, requestedInterval)
		if timeUntilEligible > 0 {
			s.logger.Warn("Check-in too soon",
				zap.String("user_id", input.UserID.String()),
				zap.Duration("time_until_eligible", timeUntilEligible),
			)

			return nil, NewCheckinIntervalError(lastCheckin.CheckinTime, requestedInterval)
		}
	}

	// Verify category belongs to user if provided
	if input.CategoryID != nil {
		if _, err := s.categoryRepo.GetByID(ctx, *input.CategoryID, input.UserID); err != nil {
			return nil, fmt.Errorf("category not found")
		}
	}

	// Get or create tags
	var tagIDs []uuid.UUID
	if len(input.TagNames) > 0 {
		tags, err := s.tagRepo.GetByNames(ctx, input.UserID, input.TagNames)
		if err != nil {
			s.logger.Error("Failed to get/create tags", zap.Error(err))
			return nil, fmt.Errorf("failed to process tags")
		}

		for _, t := range tags {
			tagIDs = append(tagIDs, t.ID)
		}
	}

	// Create check-in
	checkin, err := s.checkinRepo.Create(ctx, CreateCheckinInput{
		UserID:          input.UserID,
		CategoryID:      input.CategoryID,
		Content:         input.Content,
		CheckinTime:     input.CheckinTime,
		DurationMinutes: input.DurationMinutes,
		TagIDs:          tagIDs,
	})
	if err != nil {
		s.logger.Error("Failed to create check-in", zap.Error(err))
		return nil, fmt.Errorf("failed to create check-in")
	}

	s.logger.Info("Check-in created",
		zap.String("checkin_id", checkin.ID.String()),
		zap.String("user_id", input.UserID.String()),
	)

	// Invalidate timeline cache for this user and date
	if s.cacheInvalidator != nil {
		dateStr := checkin.CheckinTime.Format("2006-01-02")
		if err := s.cacheInvalidator.InvalidateDateCache(ctx, input.UserID, dateStr); err != nil {
			s.logger.Warn("Failed to invalidate timeline cache",
				zap.String("user_id", input.UserID.String()),
				zap.String("date", dateStr),
				zap.Error(err))
			// Don't fail the request if cache invalidation fails
		}
	}

	// Trigger search indexing
	if s.searchHandler != nil {
		searchData := map[string]interface{}{
			"id":               checkin.ID,
			"user_id":          checkin.UserID,
			"category_id":      checkin.CategoryID,
			"content":          checkin.Content,
			"checkin_time":     checkin.CheckinTime,
			"duration_minutes": checkin.DurationMinutes,
			"is_edited":        checkin.IsEdited,
			"edit_count":       checkin.EditCount,
			"created_at":       checkin.CreatedAt,
			"updated_at":       checkin.UpdatedAt,
		}
		if err := s.searchHandler.OnCheckinCreated(ctx, checkin.ID, checkin.UserID, searchData); err != nil {
			s.logger.Warn("Failed to trigger search indexing",
				zap.String("checkin_id", checkin.ID.String()),
				zap.Error(err))
			// Don't fail the request if indexing fails
		}
	}

	// Broadcast WebSocket event
	if s.hub != nil {
		msg := websocket.NewMessage(websocket.MessageTypeCheckinCreated, websocket.CheckinEventData{
			CheckinID:  checkin.ID,
			UserID:     checkin.UserID,
			Action:     "created",
			Title:      checkin.Content,
			Duration:   checkin.DurationMinutes,
			CategoryID: checkin.CategoryID,
		})
		s.hub.BroadcastToUser(input.UserID.String(), msg)
	}

	return checkin, nil
}

// GetByID retrieves a check-in by ID
func (s *Service) GetByID(ctx context.Context, id, userID uuid.UUID) (*Checkin, error) {
	checkin, err := s.checkinRepo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, fmt.Errorf("check-in not found")
	}

	return checkin, nil
}

// List retrieves check-ins with filtering
func (s *Service) List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, categoryID *uuid.UUID, limit, offset int) ([]*Checkin, int, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	checkins, total, err := s.checkinRepo.List(ctx, ListOptions{
		UserID:     userID,
		StartDate:  startDate,
		EndDate:    endDate,
		CategoryID: categoryID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		s.logger.Error("Failed to list check-ins", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to list check-ins")
	}

	return checkins, total, nil
}

// Update updates a check-in
func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*Checkin, error) {
	// Get existing check-in
	existing, err := s.checkinRepo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, fmt.Errorf("check-in not found")
	}

	// Check if edit is allowed based on user tier and checkin age
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Convert string tier to premium.Tier type
	userTier := premium.Tier(u.Tier)

	// Check edit permission using our utility function
	if !CanEditCheckin(existing.CheckinTime, userTier) {
		// Log the denied edit attempt for analytics
		s.logger.Warn("Edit permission denied",
			zap.String("checkin_id", id.String()),
			zap.String("user_id", userID.String()),
			zap.String("user_tier", string(userTier)),
			zap.Duration("checkin_age", time.Since(existing.CheckinTime)),
		)

		// Return detailed error with upgrade information
		return nil, NewEditPermissionError(id.String(), existing.CheckinTime, userTier)
	}

	// Validate content if provided
	if input.Content != nil {
		if err := validation.ValidateContent(*input.Content); err != nil {
			return nil, err
		}
	}

	// Normalize and validate tags if provided
	if input.TagNames != nil {
		normalizedTags := validation.NormalizeTags(input.TagNames)
		if err := validation.ValidateTags(normalizedTags); err != nil {
			return nil, err
		}
		input.TagNames = normalizedTags
	}

	// Verify category if provided
	if input.CategoryID != nil {
		if _, err := s.categoryRepo.GetByID(ctx, *input.CategoryID, userID); err != nil {
			return nil, fmt.Errorf("category not found")
		}
	}

	// Process tags
	var tagIDs []uuid.UUID
	if input.TagNames != nil {
		tags, err := s.tagRepo.GetByNames(ctx, userID, input.TagNames)
		if err != nil {
			s.logger.Error("Failed to get/create tags", zap.Error(err))
			return nil, fmt.Errorf("failed to process tags")
		}

		for _, t := range tags {
			tagIDs = append(tagIDs, t.ID)
		}
	}

	// Update check-in with edit reason and version for optimistic locking
	updated, err := s.checkinRepo.Update(ctx, id, userID, UpdateCheckinInput{
		Content:    input.Content,
		CategoryID: input.CategoryID,
		TagIDs:     tagIDs,
		EditReason: input.EditReason,
		Version:    input.Version,
	})
	if err != nil {
		// Check if it's a concurrent modification error
		if err.Error() == "concurrent edit detected: check-in was modified by another request" {
			s.logger.Warn("Concurrent edit attempt",
				zap.String("checkin_id", id.String()),
				zap.String("user_id", userID.String()),
				zap.Int("attempted_version", input.Version),
			)
			return nil, fmt.Errorf("concurrent edit detected: please refresh and try again")
		}
		s.logger.Error("Failed to update check-in", zap.Error(err))
		return nil, fmt.Errorf("failed to update check-in")
	}

	s.logger.Info("Check-in updated",
		zap.String("checkin_id", id.String()),
		zap.String("user_id", userID.String()),
	)

	// Invalidate timeline cache for this user and date
	if s.cacheInvalidator != nil {
		dateStr := updated.CheckinTime.Format("2006-01-02")
		if err := s.cacheInvalidator.InvalidateDateCache(ctx, userID, dateStr); err != nil {
			s.logger.Warn("Failed to invalidate timeline cache",
				zap.String("user_id", userID.String()),
				zap.String("date", dateStr),
				zap.Error(err))
			// Don't fail the request if cache invalidation fails
		}
	}

	// Trigger search indexing
	if s.searchHandler != nil {
		searchData := map[string]interface{}{
			"id":               updated.ID,
			"user_id":          updated.UserID,
			"category_id":      updated.CategoryID,
			"content":          updated.Content,
			"checkin_time":     updated.CheckinTime,
			"duration_minutes": updated.DurationMinutes,
			"is_edited":        updated.IsEdited,
			"edit_count":       updated.EditCount,
			"created_at":       updated.CreatedAt,
			"updated_at":       updated.UpdatedAt,
		}
		if err := s.searchHandler.OnCheckinUpdated(ctx, updated.ID, updated.UserID, searchData); err != nil {
			s.logger.Warn("Failed to trigger search indexing",
				zap.String("checkin_id", updated.ID.String()),
				zap.Error(err))
			// Don't fail the request if indexing fails
		}
	}

	// Broadcast WebSocket event
	if s.hub != nil {
		msg := websocket.NewMessage(websocket.MessageTypeCheckinUpdated, websocket.CheckinEventData{
			CheckinID:  updated.ID,
			UserID:     updated.UserID,
			Action:     "updated",
			Title:      updated.Content,
			Duration:   updated.DurationMinutes,
			CategoryID: updated.CategoryID,
		})
		s.hub.BroadcastToUser(userID.String(), msg)
	}

	return updated, nil
}

// Delete deletes a check-in
func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	// Get the check-in first to know which date to invalidate
	checkin, err := s.checkinRepo.GetByID(ctx, id, userID)
	if err != nil {
		s.logger.Error("Failed to get check-in for deletion", zap.Error(err))
		return fmt.Errorf("failed to delete check-in")
	}

	if err := s.checkinRepo.Delete(ctx, id, userID); err != nil {
		s.logger.Error("Failed to delete check-in", zap.Error(err))
		return fmt.Errorf("failed to delete check-in")
	}

	s.logger.Info("Check-in deleted",
		zap.String("checkin_id", id.String()),
		zap.String("user_id", userID.String()),
	)

	// Invalidate timeline cache for this user and date
	if s.cacheInvalidator != nil && checkin != nil {
		dateStr := checkin.CheckinTime.Format("2006-01-02")
		if err := s.cacheInvalidator.InvalidateDateCache(ctx, userID, dateStr); err != nil {
			s.logger.Warn("Failed to invalidate timeline cache",
				zap.String("user_id", userID.String()),
				zap.String("date", dateStr),
				zap.Error(err))
			// Don't fail the request if cache invalidation fails
		}
	}

	// Trigger search indexing
	if s.searchHandler != nil {
		if err := s.searchHandler.OnCheckinDeleted(ctx, id, userID); err != nil {
			s.logger.Warn("Failed to trigger search indexing",
				zap.String("checkin_id", id.String()),
				zap.Error(err))
			// Don't fail the request if indexing fails
		}
	}

	// Broadcast WebSocket event
	if s.hub != nil {
		msg := websocket.NewMessage(websocket.MessageTypeCheckinDeleted, websocket.CheckinEventData{
			CheckinID: id,
			UserID:    userID,
			Action:    "deleted",
		})
		s.hub.BroadcastToUser(userID.String(), msg)
	}

	return nil
}

// GetEditHistory retrieves edit history for a check-in (Premium feature)
func (s *Service) GetEditHistory(ctx context.Context, checkinID, userID uuid.UUID) ([]EditHistory, error) {
	// Check user tier
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Convert tier and check for premium/enterprise access
	userTier := premium.Tier(u.Tier)
	if userTier != premium.TierPremium && userTier != premium.TierEnterprise {
		return nil, fmt.Errorf("premium subscription required to view edit history")
	}

	history, err := s.checkinRepo.GetEditHistory(ctx, checkinID, userID)
	if err != nil {
		s.logger.Error("Failed to get edit history", zap.Error(err))
		return nil, fmt.Errorf("failed to get edit history")
	}

	return history, nil
}

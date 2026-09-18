package profile

import (
	"context"
	"database/sql"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// PrivacyLevel represents profile visibility settings
type PrivacyLevel string

const (
	PrivacyPublic  PrivacyLevel = "public"
	PrivacyPrivate PrivacyLevel = "private"
	PrivacyFriends PrivacyLevel = "friends"
)

// EnhancedProfileService handles advanced profile operations
type EnhancedProfileService struct {
	db           *sqlx.DB
	storageService StorageService // Interface for file storage (S3, local, etc.)
	logger       *zap.Logger
}

// StorageService interface for file storage operations
type StorageService interface {
	UploadFile(ctx context.Context, file multipart.File, filename string, contentType string) (string, error)
	DeleteFile(ctx context.Context, fileURL string) error
	GetPresignedURL(ctx context.Context, fileURL string, duration time.Duration) (string, error)
}

// NewEnhancedProfileService creates a new enhanced profile service
func NewEnhancedProfileService(db *sqlx.DB, storage StorageService, logger *zap.Logger) *EnhancedProfileService {
	return &EnhancedProfileService{
		db:             db,
		storageService: storage,
		logger:         logger,
	}
}

// EnhancedProfile represents a user profile with privacy settings
type EnhancedProfile struct {
	Profile
	PrivacyLevel  PrivacyLevel `db:"privacy_level" json:"privacy_level"`
	ShowEmail     bool         `db:"show_email" json:"show_email"`
	ShowActivity  bool         `db:"show_activity" json:"show_activity"`
	Searchable    bool         `db:"searchable" json:"searchable"`
	ViewCount     int          `db:"view_count" json:"view_count,omitempty"`
	LastViewedAt  *time.Time   `db:"last_viewed_at" json:"last_viewed_at,omitempty"`
}

// ProfileUpdateRequest represents a profile update request
type ProfileUpdateRequest struct {
	Bio           *string       `json:"bio,omitempty"`
	Timezone      *string       `json:"timezone,omitempty"`
	Locale        *string       `json:"locale,omitempty"`
	PrivacyLevel  *PrivacyLevel `json:"privacy_level,omitempty"`
	ShowEmail     *bool         `json:"show_email,omitempty"`
	ShowActivity  *bool         `json:"show_activity,omitempty"`
	Searchable    *bool         `json:"searchable,omitempty"`
}

// GetProfile retrieves a user's profile with privacy checks
func (s *EnhancedProfileService) GetProfile(ctx context.Context, profileUserID, viewerUserID uuid.UUID) (*EnhancedProfile, error) {
	var profile EnhancedProfile

	// Get profile with privacy settings
	err := s.db.GetContext(ctx, &profile,
		`SELECT p.*,
		        COALESCE(p.privacy_level, 'private') as privacy_level,
		        COALESCE(p.show_email, false) as show_email,
		        COALESCE(p.show_activity, false) as show_activity,
		        COALESCE(p.searchable, true) as searchable
		 FROM user_profiles p
		 WHERE p.user_id = $1`,
		profileUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("profile not found")
		}
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Check if viewer has permission to see this profile
	canView := s.canViewProfile(ctx, profile.PrivacyLevel, profileUserID, viewerUserID)
	if !canView {
		// Return limited information for private profiles
		return &EnhancedProfile{
			Profile: Profile{
				UserID:    profile.UserID,
				CreatedAt: profile.CreatedAt,
			},
			PrivacyLevel: profile.PrivacyLevel,
			Searchable:   false,
		}, nil
	}

	// Filter sensitive information based on privacy settings
	if !profile.ShowEmail && profileUserID != viewerUserID {
		// Don't show email to other users if disabled
		// This would typically be handled at the API response level
	}

	// Track profile view if viewer is different from profile owner
	if profileUserID != viewerUserID {
		go s.trackProfileView(context.Background(), profileUserID, viewerUserID)
	}

	return &profile, nil
}

// UpdateProfile updates a user's profile
func (s *EnhancedProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, req ProfileUpdateRequest) (*EnhancedProfile, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{userID}
	argCount := 1

	if req.Bio != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("bio = $%d", argCount))
		args = append(args, *req.Bio)
	}

	if req.Timezone != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("timezone = $%d", argCount))
		args = append(args, *req.Timezone)
		// Reset auto-detection when manually set
		updates = append(updates, "timezone_auto_detected = false")
	}

	if req.Locale != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("locale = $%d", argCount))
		args = append(args, *req.Locale)
	}

	if req.PrivacyLevel != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("privacy_level = $%d", argCount))
		args = append(args, *req.PrivacyLevel)
	}

	if req.ShowEmail != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("show_email = $%d", argCount))
		args = append(args, *req.ShowEmail)
	}

	if req.ShowActivity != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("show_activity = $%d", argCount))
		args = append(args, *req.ShowActivity)
	}

	if req.Searchable != nil {
		argCount++
		updates = append(updates, fmt.Sprintf("searchable = $%d", argCount))
		args = append(args, *req.Searchable)
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no updates provided")
	}

	// Add updated_at
	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")

	query := fmt.Sprintf(`
		UPDATE user_profiles
		SET %s
		WHERE user_id = $1
		RETURNING *
	`, strings.Join(updates, ", "))

	var profile EnhancedProfile
	err = tx.GetContext(ctx, &profile, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Log privacy changes in audit log
	if req.PrivacyLevel != nil || req.ShowEmail != nil || req.ShowActivity != nil || req.Searchable != nil {
		s.logSettingChange(ctx, tx, userID, "profile_privacy", "privacy_update", nil)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &profile, nil
}

// UploadAvatar handles avatar upload and updates profile
func (s *EnhancedProfileService) UploadAvatar(ctx context.Context, userID uuid.UUID, file multipart.File, header *multipart.FileHeader) (*string, error) {
	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}
	if !allowedExts[ext] {
		return nil, fmt.Errorf("invalid file type: %s", ext)
	}

	// Validate file size (max 5MB)
	if header.Size > 5*1024*1024 {
		return nil, fmt.Errorf("file size exceeds 5MB limit")
	}

	// Generate unique filename
	filename := fmt.Sprintf("avatars/%s/%s%s", userID.String(), uuid.New().String(), ext)

	// Get content type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg" // Default
	}

	// Upload to storage
	avatarURL, err := s.storageService.UploadFile(ctx, file, filename, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to upload avatar: %w", err)
	}

	// Get current avatar URL to delete old one
	var oldAvatarURL sql.NullString
	err = s.db.GetContext(ctx, &oldAvatarURL,
		"SELECT avatar_url FROM user_profiles WHERE user_id = $1",
		userID)
	if err != nil && err != sql.ErrNoRows {
		s.logger.Warn("Failed to get old avatar URL", zap.Error(err))
	}

	// Update profile with new avatar URL
	_, err = s.db.ExecContext(ctx,
		`UPDATE user_profiles
		 SET avatar_url = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = $2`,
		avatarURL, userID)
	if err != nil {
		// Try to delete uploaded file
		if delErr := s.storageService.DeleteFile(ctx, avatarURL); delErr != nil {
			s.logger.Error("Failed to delete uploaded file after DB error", zap.Error(delErr))
		}
		return nil, fmt.Errorf("failed to update avatar URL: %w", err)
	}

	// Delete old avatar if exists
	if oldAvatarURL.Valid && oldAvatarURL.String != "" {
		go func() {
			if err := s.storageService.DeleteFile(context.Background(), oldAvatarURL.String); err != nil {
				s.logger.Error("Failed to delete old avatar", zap.Error(err))
			}
		}()
	}

	return &avatarURL, nil
}

// RemoveAvatar removes a user's avatar
func (s *EnhancedProfileService) RemoveAvatar(ctx context.Context, userID uuid.UUID) error {
	// Get current avatar URL
	var avatarURL sql.NullString
	err := s.db.GetContext(ctx, &avatarURL,
		"SELECT avatar_url FROM user_profiles WHERE user_id = $1",
		userID)
	if err != nil {
		return fmt.Errorf("failed to get avatar URL: %w", err)
	}

	// Update profile to remove avatar
	_, err = s.db.ExecContext(ctx,
		`UPDATE user_profiles
		 SET avatar_url = NULL, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = $1`,
		userID)
	if err != nil {
		return fmt.Errorf("failed to remove avatar: %w", err)
	}

	// Delete file from storage
	if avatarURL.Valid && avatarURL.String != "" {
		go func() {
			if err := s.storageService.DeleteFile(context.Background(), avatarURL.String); err != nil {
				s.logger.Error("Failed to delete avatar file", zap.Error(err))
			}
		}()
	}

	return nil
}

// SearchProfiles searches for public profiles
func (s *EnhancedProfileService) SearchProfiles(ctx context.Context, query string, limit int) ([]EnhancedProfile, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var profiles []EnhancedProfile
	err := s.db.SelectContext(ctx, &profiles,
		`SELECT p.*, u.email, u.display_name
		 FROM user_profiles p
		 JOIN users u ON p.user_id = u.id
		 WHERE p.searchable = true
		   AND p.privacy_level = 'public'
		   AND u.deleted_at IS NULL
		   AND (
		     LOWER(u.display_name) LIKE LOWER($1) OR
		     LOWER(p.bio) LIKE LOWER($1)
		   )
		 LIMIT $2`,
		"%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search profiles: %w", err)
	}

	return profiles, nil
}

// canViewProfile checks if a viewer can see a profile based on privacy settings
func (s *EnhancedProfileService) canViewProfile(ctx context.Context, privacy PrivacyLevel, profileUserID, viewerUserID uuid.UUID) bool {
	// Owner can always view their own profile
	if profileUserID == viewerUserID {
		return true
	}

	switch privacy {
	case PrivacyPublic:
		return true
	case PrivacyFriends:
		// TODO: Check if users are connected/friends
		// For now, return false
		return false
	case PrivacyPrivate:
		return false
	default:
		return false
	}
}

// trackProfileView tracks when a profile is viewed
func (s *EnhancedProfileService) trackProfileView(ctx context.Context, profileUserID, viewerUserID uuid.UUID) {
	// Increment view count
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_profiles
		 SET view_count = COALESCE(view_count, 0) + 1,
		     last_viewed_at = CURRENT_TIMESTAMP
		 WHERE user_id = $1`,
		profileUserID)
	if err != nil {
		s.logger.Warn("Failed to track profile view", zap.Error(err))
	}

	// TODO: Store detailed view analytics if needed
}

// logSettingChange logs changes to settings in audit log
func (s *EnhancedProfileService) logSettingChange(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID, settingType, settingName string, oldValue interface{}) {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO settings_audit_log (user_id, setting_type, setting_name, old_value, new_value)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, settingType, settingName, oldValue, nil)
	if err != nil {
		s.logger.Warn("Failed to log setting change",
			zap.Error(err),
			zap.String("setting_type", settingType))
	}
}

// GetProfileStats returns profile statistics
func (s *EnhancedProfileService) GetProfileStats(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get basic profile stats
	err := s.db.GetContext(ctx, &stats,
		`SELECT
		   COALESCE(view_count, 0) as profile_views,
		   COALESCE(searchable, true) as is_searchable,
		   privacy_level
		 FROM user_profiles
		 WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile stats: %w", err)
	}

	// Get checkin count
	var checkinCount int
	err = s.db.GetContext(ctx, &checkinCount,
		`SELECT COUNT(*) FROM checkins WHERE user_id = $1`,
		userID)
	if err == nil {
		stats["total_checkins"] = checkinCount
	}

	// Get account age
	var createdAt time.Time
	err = s.db.GetContext(ctx, &createdAt,
		`SELECT created_at FROM users WHERE id = $1`,
		userID)
	if err == nil {
		stats["account_age_days"] = int(time.Since(createdAt).Hours() / 24)
	}

	return stats, nil
}
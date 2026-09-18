package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// DataExportService handles user data export operations
type DataExportService struct {
	db         *sqlx.DB
	csvExport  *CSVExporter
	jsonExport *JSONExporter
	pdfExport  *PDFExporter
	logger     *zap.Logger
}

// NewDataExportService creates a new data export service
func NewDataExportService(db *sqlx.DB, logger *zap.Logger) *DataExportService {
	return &DataExportService{
		db:         db,
		csvExport:  NewCSVExporter(logger),
		jsonExport: NewJSONExporter(logger),
		pdfExport:  NewPDFExporter(logger),
		logger:     logger,
	}
}

// DataExportRequest represents a data export request
type DataExportRequest struct {
	ID          uuid.UUID    `db:"id" json:"id"`
	UserID      uuid.UUID    `db:"user_id" json:"user_id"`
	Status      string       `db:"status" json:"status"`
	Format      ExportFormat `db:"format" json:"format"`
	FileURL     *string      `db:"file_url" json:"file_url,omitempty"`
	ExpiresAt   *time.Time   `db:"expires_at" json:"expires_at,omitempty"`
	RequestedAt time.Time    `db:"requested_at" json:"requested_at"`
	CompletedAt *time.Time   `db:"completed_at" json:"completed_at,omitempty"`
	Error       *string      `db:"error_message" json:"error,omitempty"`
}

// UserDataExport represents all exportable user data
type UserDataExport struct {
	ExportedAt time.Time              `json:"exported_at"`
	User       UserData               `json:"user"`
	Profile    ProfileData            `json:"profile"`
	Settings   SettingsData           `json:"settings"`
	Checkins   []CheckinData          `json:"checkins"`
	Tasks      []TaskData             `json:"tasks"`
	Projects   []ProjectData          `json:"projects"`
	Tags       []TagData              `json:"tags"`
	Security   SecurityData           `json:"security"`
	Activity   []ActivityData         `json:"activity"`
}

// UserData represents basic user information
type UserData struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	DisplayName *string    `json:"display_name,omitempty"`
	Role        string     `json:"role"`
	Tier        string     `json:"tier"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ProfileData represents user profile data
type ProfileData struct {
	Bio       *string `json:"bio,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	Timezone  string  `json:"timezone"`
	Locale    string  `json:"locale"`
	Privacy   string  `json:"privacy_level"`
}

// SettingsData represents user settings
type SettingsData struct {
	Theme                   string                 `json:"theme"`
	Language                string                 `json:"language"`
	DateFormat              string                 `json:"date_format"`
	TimeFormat              string                 `json:"time_format"`
	NotificationPreferences NotificationPreferences `json:"notifications"`
	DataRetention           DataRetentionSettings  `json:"data_retention"`
}

// NotificationPreferences represents notification settings
type NotificationPreferences struct {
	EmailEnabled     bool   `json:"email_enabled"`
	PushEnabled      bool   `json:"push_enabled"`
	ReminderInterval int    `json:"reminder_interval_minutes"`
	DNDEnabled       bool   `json:"dnd_enabled"`
	DNDSchedule      string `json:"dnd_schedule,omitempty"`
}

// DataRetentionSettings represents data retention preferences
type DataRetentionSettings struct {
	AutoDeleteEnabled bool `json:"auto_delete_enabled"`
	RetentionDays     *int `json:"retention_days,omitempty"`
	InactivityDays    *int `json:"inactivity_days,omitempty"`
}

// CheckinData represents a checkin record
type CheckinData struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	Tags        []string   `json:"tags"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TaskData represents a task record
type TaskData struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	ProjectID   *uuid.UUID `json:"project_id,omitempty"`
	Tags        []string   `json:"tags"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ProjectData represents a project record
type ProjectData struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TagData represents a tag record
type TagData struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color,omitempty"`
	UsageCount int      `json:"usage_count"`
	CreatedAt time.Time `json:"created_at"`
}

// SecurityData represents security-related data
type SecurityData struct {
	TwoFactorEnabled bool       `json:"two_factor_enabled"`
	RecoveryEmail    *string    `json:"recovery_email,omitempty"`
	LastPasswordChange *time.Time `json:"last_password_change,omitempty"`
	LoginHistory     []LoginRecord `json:"recent_logins"`
}

// LoginRecord represents a login history record
type LoginRecord struct {
	Timestamp time.Time `json:"timestamp"`
	IPAddress *string   `json:"ip_address,omitempty"`
	UserAgent *string   `json:"user_agent,omitempty"`
	Success   bool      `json:"success"`
}

// ActivityData represents user activity
type ActivityData struct {
	Date         string `json:"date"`
	CheckinCount int    `json:"checkins"`
	TaskCount    int    `json:"tasks_completed"`
	ActiveTime   int    `json:"active_minutes"`
}

// RequestDataExport initiates a data export request
func (s *DataExportService) RequestDataExport(ctx context.Context, userID uuid.UUID, format ExportFormat) (*DataExportRequest, error) {
	// Check for existing pending request
	var existingID uuid.UUID
	err := s.db.GetContext(ctx, &existingID,
		`SELECT id FROM data_export_requests
		 WHERE user_id = $1 AND status IN ('pending', 'processing')`,
		userID)
	if err == nil {
		return nil, fmt.Errorf("export already in progress")
	}

	// Create new export request
	request := &DataExportRequest{
		ID:          uuid.New(),
		UserID:      userID,
		Status:      "pending",
		Format:      format,
		RequestedAt: time.Now(),
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO data_export_requests (id, user_id, status, format, requested_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		request.ID, request.UserID, request.Status, request.Format, request.RequestedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create export request: %w", err)
	}

	// Process export asynchronously
	go s.processExport(context.Background(), request.ID)

	return request, nil
}

// processExport processes a data export request
func (s *DataExportService) processExport(ctx context.Context, requestID uuid.UUID) {
	// Update status to processing
	_, err := s.db.ExecContext(ctx,
		`UPDATE data_export_requests
		 SET status = 'processing'
		 WHERE id = $1`,
		requestID)
	if err != nil {
		s.logger.Error("Failed to update export status", zap.Error(err))
		return
	}

	// Get request details
	var request DataExportRequest
	err = s.db.GetContext(ctx, &request,
		`SELECT * FROM data_export_requests WHERE id = $1`,
		requestID)
	if err != nil {
		s.logger.Error("Failed to get export request", zap.Error(err))
		s.markExportFailed(ctx, requestID, "Failed to retrieve request")
		return
	}

	// Gather all user data
	exportData, err := s.gatherUserData(ctx, request.UserID)
	if err != nil {
		s.logger.Error("Failed to gather user data", zap.Error(err))
		s.markExportFailed(ctx, requestID, "Failed to gather data")
		return
	}

	// Export based on format - convert UserDataExport to ExportData format
	convertedData := &ExportData{
		Checkins: make([]CheckinExportData, 0, len(exportData.Checkins)),
		Metadata: ExportMetadata{
			ExportedAt:   exportData.ExportedAt,
			ExportFormat: request.Format,
			TotalRecords: len(exportData.Checkins),
		},
	}
	for _, c := range exportData.Checkins {
		convertedData.Checkins = append(convertedData.Checkins, CheckinExportData{
			ID:        c.ID,
			Title:     c.Title,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		})
	}

	var exportBytes []byte
	switch request.Format {
	case FormatJSON:
		exportBytes, err = s.jsonExport.Export(convertedData)
	case FormatCSV:
		exportBytes, err = s.csvExport.Export(convertedData)
	case FormatPDF:
		exportBytes, err = s.pdfExport.Export(convertedData)
	default:
		err = fmt.Errorf("unsupported format: %s", request.Format)
	}

	if err != nil {
		s.logger.Error("Failed to export data", zap.Error(err))
		s.markExportFailed(ctx, requestID, "Failed to generate export file")
		return
	}

	// Save export bytes to storage and get file URL
	fileURL, err := s.saveExportFile(ctx, requestID, request.Format, exportBytes)

	if err != nil {
		s.logger.Error("Failed to export data", zap.Error(err))
		s.markExportFailed(ctx, requestID, "Failed to generate export file")
		return
	}

	// Update request with file URL
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days
	completedAt := time.Now()
	_, err = s.db.ExecContext(ctx,
		`UPDATE data_export_requests
		 SET status = 'completed',
		     file_url = $2,
		     expires_at = $3,
		     completed_at = $4
		 WHERE id = $1`,
		requestID, fileURL, expiresAt, completedAt)
	if err != nil {
		s.logger.Error("Failed to update export request", zap.Error(err))
		return
	}

	// TODO: Send notification to user that export is ready
}

// saveExportFile saves the export bytes to storage and returns a URL
func (s *DataExportService) saveExportFile(_ context.Context, requestID uuid.UUID, format ExportFormat, data []byte) (string, error) {
	// Get file extension
	extension := string(format)

	// For now, save to local filesystem and return a path-based URL
	// In production, this would upload to S3, GCS, or similar
	exportDir := "/tmp/exports"
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	filePath := filepath.Join(exportDir, fmt.Sprintf("%s.%s", requestID.String(), extension))
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write export file: %w", err)
	}

	// Return a URL that can be used to download the file
	// In production, this would be a signed URL from cloud storage
	return fmt.Sprintf("/api/v1/exports/download/%s", requestID.String()), nil
}

// gatherUserData collects all user data for export
func (s *DataExportService) gatherUserData(ctx context.Context, userID uuid.UUID) (*UserDataExport, error) {
	export := &UserDataExport{
		ExportedAt: time.Now(),
	}

	// Get user data
	err := s.db.GetContext(ctx, &export.User,
		`SELECT id, email, display_name, role, tier, created_at, updated_at
		 FROM users WHERE id = $1`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user data: %w", err)
	}

	// Get profile data
	err = s.db.GetContext(ctx, &export.Profile,
		`SELECT bio, avatar_url, timezone, locale,
		        COALESCE(privacy_level, 'private') as privacy_level
		 FROM user_profiles WHERE user_id = $1`,
		userID)
	if err != nil {
		s.logger.Warn("Failed to get profile data", zap.Error(err))
	}

	// Get settings
	var settings struct {
		Theme        string `db:"theme"`
		Language     string `db:"language"`
		DateFormat   string `db:"date_format"`
		TimeFormat   string `db:"time_format"`
	}
	err = s.db.GetContext(ctx, &settings,
		`SELECT theme, language, date_format, time_format
		 FROM user_settings WHERE user_id = $1`,
		userID)
	if err == nil {
		export.Settings.Theme = settings.Theme
		export.Settings.Language = settings.Language
		export.Settings.DateFormat = settings.DateFormat
		export.Settings.TimeFormat = settings.TimeFormat
	}

	// Get notification settings
	var notif struct {
		EmailEnabled     bool `db:"email_notifications"`
		PushEnabled      bool `db:"push_notifications"`
		ReminderInterval int  `db:"reminder_interval_minutes"`
		DNDEnabled       bool `db:"dnd_enabled"`
	}
	err = s.db.GetContext(ctx, &notif,
		`SELECT email_notifications, push_notifications,
		        reminder_interval_minutes, dnd_enabled
		 FROM notification_settings WHERE user_id = $1`,
		userID)
	if err == nil {
		export.Settings.NotificationPreferences.EmailEnabled = notif.EmailEnabled
		export.Settings.NotificationPreferences.PushEnabled = notif.PushEnabled
		export.Settings.NotificationPreferences.ReminderInterval = notif.ReminderInterval
		export.Settings.NotificationPreferences.DNDEnabled = notif.DNDEnabled
	}

	// Get checkins
	err = s.db.SelectContext(ctx, &export.Checkins,
		`SELECT id, title, description, status, priority,
		        due_date, completed_at, created_at, updated_at
		 FROM checkins WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID)
	if err != nil {
		s.logger.Warn("Failed to get checkins", zap.Error(err))
	}

	// Get security data
	var secData struct {
		TwoFactorEnabled bool      `db:"two_factor_enabled"`
		RecoveryEmail    *string   `db:"recovery_email"`
	}
	err = s.db.GetContext(ctx, &secData,
		`SELECT two_factor_enabled, recovery_email
		 FROM users WHERE id = $1`,
		userID)
	if err == nil {
		export.Security.TwoFactorEnabled = secData.TwoFactorEnabled
		export.Security.RecoveryEmail = secData.RecoveryEmail
	}

	return export, nil
}

// markExportFailed marks an export request as failed
func (s *DataExportService) markExportFailed(ctx context.Context, requestID uuid.UUID, errorMsg string) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE data_export_requests
		 SET status = 'failed',
		     error_message = $2
		 WHERE id = $1`,
		requestID, errorMsg)
	if err != nil {
		s.logger.Error("Failed to mark export as failed", zap.Error(err))
	}
}

// GetExportRequest retrieves an export request
func (s *DataExportService) GetExportRequest(ctx context.Context, requestID uuid.UUID, userID uuid.UUID) (*DataExportRequest, error) {
	var request DataExportRequest
	err := s.db.GetContext(ctx, &request,
		`SELECT * FROM data_export_requests
		 WHERE id = $1 AND user_id = $2`,
		requestID, userID)
	if err != nil {
		return nil, fmt.Errorf("export request not found: %w", err)
	}
	return &request, nil
}

// GetExportHistory retrieves export history for a user
func (s *DataExportService) GetExportHistory(ctx context.Context, userID uuid.UUID) ([]DataExportRequest, error) {
	var requests []DataExportRequest
	err := s.db.SelectContext(ctx, &requests,
		`SELECT * FROM data_export_requests
		 WHERE user_id = $1
		 ORDER BY requested_at DESC
		 LIMIT 10`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get export history: %w", err)
	}
	return requests, nil
}
package checkin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// OfflineQueueConfig contains configuration for offline storage queue
type OfflineQueueConfig struct {
	DBPath        string        // Path to SQLite database file
	EncryptionKey []byte        // 32-byte encryption key for AES-256
	MaxItems      int           // Maximum items in queue (default: 1000)
	MaxAge        time.Duration // Maximum age of items (default: 30 days)
	Logger        *zap.Logger
}

// OfflineQueueItem represents a check-in queued for sync
type OfflineQueueItem struct {
	ID           uuid.UUID              `json:"id"`
	UserID       uuid.UUID              `json:"user_id"`
	CheckinData  map[string]interface{} `json:"checkin_data"`
	CreatedAt    time.Time              `json:"created_at"`
	SyncStatus   string                 `json:"sync_status"` // pending, syncing, failed, synced
	RetryCount   int                    `json:"retry_count"`
	LastAttempt  *time.Time             `json:"last_attempt,omitempty"`
	ErrorMessage *string                `json:"error_message,omitempty"`
}

// OfflineQueue manages check-in data when network is unavailable
type OfflineQueue struct {
	db            *sql.DB
	encryptionKey []byte
	maxItems      int
	maxAge        time.Duration
	logger        *zap.Logger
	gcm           cipher.AEAD
}

// NewOfflineQueue creates a new offline queue with SQLite storage
func NewOfflineQueue(cfg OfflineQueueConfig) (*OfflineQueue, error) {
	// Set defaults
	if cfg.MaxItems == 0 {
		cfg.MaxItems = 1000
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 30 * 24 * time.Hour // 30 days
	}
	if len(cfg.EncryptionKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes for AES-256")
	}

	// Open SQLite database
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for SQLite
	db.SetMaxOpenConns(1) // SQLite works best with single connection
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Initialize encryption
	block, err := aes.NewCipher(cfg.EncryptionKey)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	q := &OfflineQueue{
		db:            db,
		encryptionKey: cfg.EncryptionKey,
		maxItems:      cfg.MaxItems,
		maxAge:        cfg.MaxAge,
		logger:        cfg.Logger,
		gcm:           gcm,
	}

	// Initialize schema
	if err := q.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return q, nil
}

// initSchema creates the necessary tables if they don't exist
func (q *OfflineQueue) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS offline_queue (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		checkin_data_encrypted TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		sync_status TEXT NOT NULL DEFAULT 'pending',
		retry_count INTEGER NOT NULL DEFAULT 0,
		last_attempt INTEGER,
		error_message TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_user_id ON offline_queue(user_id);
	CREATE INDEX IF NOT EXISTS idx_sync_status ON offline_queue(sync_status);
	CREATE INDEX IF NOT EXISTS idx_created_at ON offline_queue(created_at);
	CREATE INDEX IF NOT EXISTS idx_user_status ON offline_queue(user_id, sync_status);
	`

	_, err := q.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	q.logger.Info("Offline queue schema initialized")
	return nil
}

// encrypt encrypts data using AES-256-GCM
func (q *OfflineQueue) encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, q.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := q.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts data using AES-256-GCM
func (q *OfflineQueue) decrypt(ciphertext string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	nonceSize := q.gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := q.gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// Enqueue adds a check-in to the offline queue
func (q *OfflineQueue) Enqueue(ctx context.Context, userID uuid.UUID, checkinData map[string]interface{}) (*OfflineQueueItem, error) {
	// Check if we need to prune old items first
	if err := q.pruneIfNeeded(ctx); err != nil {
		q.logger.Warn("Failed to prune queue", zap.Error(err))
	}

	// Marshal check-in data
	dataJSON, err := json.Marshal(checkinData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	// Encrypt data
	encryptedData, err := q.encrypt(dataJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt data: %w", err)
	}

	item := &OfflineQueueItem{
		ID:          uuid.New(),
		UserID:      userID,
		CheckinData: checkinData,
		CreatedAt:   time.Now(),
		SyncStatus:  "pending",
		RetryCount:  0,
	}

	// Insert into database
	query := `
		INSERT INTO offline_queue (id, user_id, checkin_data_encrypted, created_at, sync_status, retry_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = q.db.ExecContext(ctx, query,
		item.ID.String(),
		item.UserID.String(),
		encryptedData,
		item.CreatedAt.Unix(),
		item.SyncStatus,
		item.RetryCount,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert item: %w", err)
	}

	q.logger.Info("Check-in enqueued for offline sync",
		zap.String("item_id", item.ID.String()),
		zap.String("user_id", userID.String()),
	)

	return item, nil
}

// Dequeue retrieves and removes the next pending item for a user
func (q *OfflineQueue) Dequeue(ctx context.Context, userID uuid.UUID) (*OfflineQueueItem, error) {
	// Start transaction
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get next pending item
	query := `
		SELECT id, user_id, checkin_data_encrypted, created_at, sync_status, retry_count, last_attempt, error_message
		FROM offline_queue
		WHERE user_id = ? AND sync_status = 'pending'
		ORDER BY created_at ASC
		LIMIT 1
	`

	var (
		id                     string
		userIDStr              string
		encryptedData          string
		createdAtUnix          int64
		syncStatus             string
		retryCount             int
		lastAttemptUnix        sql.NullInt64
		errorMessage           sql.NullString
	)

	err = tx.QueryRowContext(ctx, query, userID.String()).Scan(
		&id, &userIDStr, &encryptedData, &createdAtUnix, &syncStatus, &retryCount, &lastAttemptUnix, &errorMessage,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No pending items
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query item: %w", err)
	}

	// Decrypt data
	plaintext, err := q.decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	var checkinData map[string]interface{}
	if err := json.Unmarshal(plaintext, &checkinData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	item := &OfflineQueueItem{
		ID:          uuid.MustParse(id),
		UserID:      uuid.MustParse(userIDStr),
		CheckinData: checkinData,
		CreatedAt:   time.Unix(createdAtUnix, 0),
		SyncStatus:  syncStatus,
		RetryCount:  retryCount,
	}

	if lastAttemptUnix.Valid {
		lastAttempt := time.Unix(lastAttemptUnix.Int64, 0)
		item.LastAttempt = &lastAttempt
	}

	if errorMessage.Valid {
		item.ErrorMessage = &errorMessage.String
	}

	// Delete the item
	deleteQuery := `DELETE FROM offline_queue WHERE id = ?`
	_, err = tx.ExecContext(ctx, deleteQuery, id)
	if err != nil {
		return nil, fmt.Errorf("failed to delete item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	q.logger.Debug("Item dequeued",
		zap.String("item_id", item.ID.String()),
		zap.String("user_id", userID.String()),
	)

	return item, nil
}

// List retrieves all items for a user without removing them
func (q *OfflineQueue) List(ctx context.Context, userID uuid.UUID, status string) ([]*OfflineQueueItem, error) {
	query := `
		SELECT id, user_id, checkin_data_encrypted, created_at, sync_status, retry_count, last_attempt, error_message
		FROM offline_queue
		WHERE user_id = ?
	`
	args := []interface{}{userID.String()}

	if status != "" {
		query += " AND sync_status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at ASC"

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}
	defer rows.Close()

	items := make([]*OfflineQueueItem, 0)
	for rows.Next() {
		var (
			id                     string
			userIDStr              string
			encryptedData          string
			createdAtUnix          int64
			syncStatus             string
			retryCount             int
			lastAttemptUnix        sql.NullInt64
			errorMessage           sql.NullString
		)

		if err := rows.Scan(&id, &userIDStr, &encryptedData, &createdAtUnix, &syncStatus, &retryCount, &lastAttemptUnix, &errorMessage); err != nil {
			q.logger.Warn("Failed to scan row", zap.Error(err))
			continue
		}

		// Decrypt data
		plaintext, err := q.decrypt(encryptedData)
		if err != nil {
			q.logger.Warn("Failed to decrypt item", zap.String("id", id), zap.Error(err))
			continue
		}

		var checkinData map[string]interface{}
		if err := json.Unmarshal(plaintext, &checkinData); err != nil {
			q.logger.Warn("Failed to unmarshal item", zap.String("id", id), zap.Error(err))
			continue
		}

		item := &OfflineQueueItem{
			ID:          uuid.MustParse(id),
			UserID:      uuid.MustParse(userIDStr),
			CheckinData: checkinData,
			CreatedAt:   time.Unix(createdAtUnix, 0),
			SyncStatus:  syncStatus,
			RetryCount:  retryCount,
		}

		if lastAttemptUnix.Valid {
			lastAttempt := time.Unix(lastAttemptUnix.Int64, 0)
			item.LastAttempt = &lastAttempt
		}

		if errorMessage.Valid {
			item.ErrorMessage = &errorMessage.String
		}

		items = append(items, item)
	}

	return items, nil
}

// UpdateStatus updates the sync status of an item
func (q *OfflineQueue) UpdateStatus(ctx context.Context, itemID uuid.UUID, status string, errorMsg *string) error {
	query := `
		UPDATE offline_queue
		SET sync_status = ?,
		    retry_count = retry_count + 1,
		    last_attempt = ?,
		    error_message = ?
		WHERE id = ?
	`

	var errMsgVal sql.NullString
	if errorMsg != nil {
		errMsgVal = sql.NullString{String: *errorMsg, Valid: true}
	}

	_, err := q.db.ExecContext(ctx, query, status, time.Now().Unix(), errMsgVal, itemID.String())
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	q.logger.Debug("Item status updated",
		zap.String("item_id", itemID.String()),
		zap.String("status", status),
	)

	return nil
}

// Count returns the number of items in the queue for a user
func (q *OfflineQueue) Count(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM offline_queue WHERE user_id = ?`

	var count int
	err := q.db.QueryRowContext(ctx, query, userID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count items: %w", err)
	}

	return count, nil
}

// pruneIfNeeded removes old items or excess items to maintain limits
func (q *OfflineQueue) pruneIfNeeded(ctx context.Context) error {
	// Prune items older than maxAge
	cutoffTime := time.Now().Add(-q.maxAge)
	deleteOldQuery := `DELETE FROM offline_queue WHERE created_at < ?`

	result, err := q.db.ExecContext(ctx, deleteOldQuery, cutoffTime.Unix())
	if err != nil {
		return fmt.Errorf("failed to prune old items: %w", err)
	}

	if deleted, _ := result.RowsAffected(); deleted > 0 {
		q.logger.Info("Pruned old items from queue", zap.Int64("count", deleted))
	}

	// Prune excess items per user
	// First, get total count
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM offline_queue`
	if err := q.db.QueryRowContext(ctx, countQuery).Scan(&totalCount); err != nil {
		return fmt.Errorf("failed to count total items: %w", err)
	}

	if totalCount > q.maxItems {
		// Delete oldest items to get back to maxItems
		excessCount := totalCount - q.maxItems
		deleteExcessQuery := `
			DELETE FROM offline_queue
			WHERE id IN (
				SELECT id FROM offline_queue
				ORDER BY created_at ASC
				LIMIT ?
			)
		`

		result, err := q.db.ExecContext(ctx, deleteExcessQuery, excessCount)
		if err != nil {
			return fmt.Errorf("failed to prune excess items: %w", err)
		}

		if deleted, _ := result.RowsAffected(); deleted > 0 {
			q.logger.Warn("Pruned excess items from queue",
				zap.Int64("count", deleted),
				zap.Int("max_items", q.maxItems),
			)
		}
	}

	return nil
}

// Prune manually triggers pruning of old or excess items
func (q *OfflineQueue) Prune(ctx context.Context) error {
	return q.pruneIfNeeded(ctx)
}

// Clear removes all items for a user
func (q *OfflineQueue) Clear(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM offline_queue WHERE user_id = ?`

	result, err := q.db.ExecContext(ctx, query, userID.String())
	if err != nil {
		return fmt.Errorf("failed to clear queue: %w", err)
	}

	deleted, _ := result.RowsAffected()
	q.logger.Info("Queue cleared for user",
		zap.String("user_id", userID.String()),
		zap.Int64("items_deleted", deleted),
	)

	return nil
}

// Close closes the database connection
func (q *OfflineQueue) Close() error {
	if q.db != nil {
		return q.db.Close()
	}
	return nil
}

// GetStats returns queue statistics
func (q *OfflineQueue) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total items
	var total int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM offline_queue`).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}
	stats["total_items"] = total

	// Items by status
	statusQuery := `SELECT sync_status, COUNT(*) FROM offline_queue GROUP BY sync_status`
	rows, err := q.db.QueryContext(ctx, statusQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query status counts: %w", err)
	}
	defer rows.Close()

	statusCounts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		statusCounts[status] = count
	}
	stats["by_status"] = statusCounts

	// Oldest item
	var oldestUnix sql.NullInt64
	if err := q.db.QueryRowContext(ctx, `SELECT MIN(created_at) FROM offline_queue`).Scan(&oldestUnix); err == nil && oldestUnix.Valid {
		stats["oldest_item"] = time.Unix(oldestUnix.Int64, 0)
	}

	return stats, nil
}

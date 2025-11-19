package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// Fixtures provides test data creation helpers without importing domain models
type Fixtures struct {
	DB *TestDB
}

// NewFixtures creates a new fixtures instance
func NewFixtures(db *TestDB) *Fixtures {
	return &Fixtures{DB: db}
}

// CreateTestUser creates a test user and returns its ID
func (f *Fixtures) CreateTestUser(t *testing.T, email, password, username string) uuid.UUID {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	userID := uuid.New()
	query := `
		INSERT INTO users (id, email, username, password_hash, tier, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err = f.DB.DB.Exec(query, userID, email, username, string(hashedPassword), "free", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return userID
}

// CreatePremiumUser creates a premium tier test user
func (f *Fixtures) CreatePremiumUser(t *testing.T, email, password, username string) uuid.UUID {
	userID := f.CreateTestUser(t, email, password, username)

	// Upgrade to premium
	_, err := f.DB.DB.Exec("UPDATE users SET tier = 'premium' WHERE id = $1", userID)
	if err != nil {
		t.Fatalf("Failed to upgrade user to premium: %v", err)
	}

	return userID
}

// CreateTestCategory creates a test category and returns its ID
func (f *Fixtures) CreateTestCategory(t *testing.T, userID uuid.UUID, name string, color, icon *string) uuid.UUID {
	categoryID := uuid.New()
	query := `
		INSERT INTO categories (id, user_id, name, color, icon, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := f.DB.DB.Exec(query, categoryID, userID, name, color, icon, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test category: %v", err)
	}

	return categoryID
}

// CreateTestTag creates a test tag and returns its ID
func (f *Fixtures) CreateTestTag(t *testing.T, userID uuid.UUID, name string) uuid.UUID {
	tagID := uuid.New()
	query := `
		INSERT INTO tags (id, user_id, name, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := f.DB.DB.Exec(query, tagID, userID, name, time.Now())
	if err != nil {
		t.Fatalf("Failed to create test tag: %v", err)
	}

	return tagID
}

// CreateTestCheckin creates a test check-in and returns its ID
func (f *Fixtures) CreateTestCheckin(t *testing.T, userID uuid.UUID, categoryID *uuid.UUID, content string, duration int) uuid.UUID {
	checkinID := uuid.New()
	query := `
		INSERT INTO checkins (id, user_id, category_id, content, checkin_time, duration_minutes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := f.DB.DB.Exec(query, checkinID, userID, categoryID, content, time.Now(), duration, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test checkin: %v", err)
	}

	return checkinID
}

// CreateCheckinWithTags creates a check-in with associated tags and returns the checkin ID
func (f *Fixtures) CreateCheckinWithTags(t *testing.T, userID uuid.UUID, content string, tagNames []string) uuid.UUID {
	ctx := context.Background()

	// Create check-in
	checkinID := f.CreateTestCheckin(t, userID, nil, content, 30)

	// Create and associate tags
	for _, name := range tagNames {
		tagID := f.CreateTestTag(t, userID, name)

		query := `INSERT INTO checkin_tags (checkin_id, tag_id) VALUES ($1, $2)`
		_, err := f.DB.DB.ExecContext(ctx, query, checkinID, tagID)
		if err != nil {
			t.Fatalf("Failed to associate tag with checkin: %v", err)
		}
	}

	return checkinID
}

// CreateOldCheckin creates a check-in that's older than 2 hours and returns its ID
func (f *Fixtures) CreateOldCheckin(t *testing.T, userID uuid.UUID, content string) uuid.UUID {
	checkinID := uuid.New()
	oldTime := time.Now().Add(-3 * time.Hour)

	query := `
		INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := f.DB.DB.Exec(query, checkinID, userID, content, oldTime, 30, oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to create old checkin: %v", err)
	}

	return checkinID
}

// Helper function to get a string pointer
func StringPtr(s string) *string {
	return &s
}

// CreateTestUser creates a test user without needing a Fixtures instance
func CreateTestUser(t *testing.T, db *sqlx.DB, email, username string) uuid.UUID {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("TestPassword123!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	userID := uuid.New()
	query := `
		INSERT INTO users (id, email, username, password_hash, tier, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err = db.Exec(query, userID, email, username, string(hashedPassword), "free", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return userID
}
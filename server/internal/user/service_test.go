package user

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockRepository implements user Repository for testing
type mockRepository struct {
	getByIDFunc func(ctx context.Context, id uuid.UUID) (*User, error)
	updateFunc  func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error)
	deleteFunc  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, sql.ErrNoRows
}

func (m *mockRepository) Update(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, input)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return errors.New("not implemented")
}

// Other repository methods (not used by Service, so stub implementations)
func (m *mockRepository) Create(ctx context.Context, input CreateUserInput) (*User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	return nil, sql.ErrNoRows
}

func (m *mockRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	return nil, sql.ErrNoRows
}

func (m *mockRepository) List(ctx context.Context, limit, offset int) ([]*User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockRepository) Count(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}

func TestNewService(t *testing.T) {
	repo := &mockRepository{}
	logger := zap.NewNop()

	service := NewService(repo, logger)

	assert.NotNil(t, service)
	assert.Equal(t, repo, service.repo)
	assert.Equal(t, logger, service.logger)
}

func TestService_GetByID_Success(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	expectedUser := &User{
		ID:       userID,
		Email:    "test@example.com",
		Username: "testuser",
		Tier:     "free",
	}

	repo := &mockRepository{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*User, error) {
			assert.Equal(t, userID, id)
			return expectedUser, nil
		},
	}

	service := NewService(repo, zap.NewNop())
	user, err := service.GetByID(ctx, userID)

	require.NoError(t, err)
	assert.Equal(t, expectedUser, user)
}

func TestService_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	repo := &mockRepository{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*User, error) {
			return nil, sql.ErrNoRows
		},
	}

	service := NewService(repo, zap.NewNop())
	user, err := service.GetByID(ctx, userID)

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "user not found")
}

func TestService_Update_Success(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	displayName := "Updated Name"
	input := UpdateUserInput{
		DisplayName: &displayName,
	}

	expectedUser := &User{
		ID:          userID,
		Email:       "test@example.com",
		Username:    "testuser",
		DisplayName: &displayName,
	}

	repo := &mockRepository{
		updateFunc: func(ctx context.Context, id uuid.UUID, inp UpdateUserInput) (*User, error) {
			assert.Equal(t, userID, id)
			assert.Equal(t, input.DisplayName, inp.DisplayName)
			return expectedUser, nil
		},
	}

	service := NewService(repo, zap.NewNop())
	user, err := service.Update(ctx, userID, input)

	require.NoError(t, err)
	assert.Equal(t, expectedUser, user)
}

func TestService_Update_DisplayNameTooLong(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	longName := string(make([]byte, 101)) // 101 characters
	input := UpdateUserInput{
		DisplayName: &longName,
	}

	repo := &mockRepository{}
	service := NewService(repo, zap.NewNop())
	user, err := service.Update(ctx, userID, input)

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "display name must be less than 100 characters")
}

func TestService_Update_RepositoryError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	displayName := "Valid Name"
	input := UpdateUserInput{
		DisplayName: &displayName,
	}

	repo := &mockRepository{
		updateFunc: func(ctx context.Context, id uuid.UUID, inp UpdateUserInput) (*User, error) {
			return nil, errors.New("database error")
		},
	}

	service := NewService(repo, zap.NewNop())
	user, err := service.Update(ctx, userID, input)

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "failed to update user")
}

func TestService_Delete_Success(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	repo := &mockRepository{
		deleteFunc: func(ctx context.Context, id uuid.UUID) error {
			assert.Equal(t, userID, id)
			return nil
		},
	}

	service := NewService(repo, zap.NewNop())
	err := service.Delete(ctx, userID)

	assert.NoError(t, err)
}

func TestService_Delete_RepositoryError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	repo := &mockRepository{
		deleteFunc: func(ctx context.Context, id uuid.UUID) error {
			return errors.New("database error")
		},
	}

	service := NewService(repo, zap.NewNop())
	err := service.Delete(ctx, userID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete user")
}

// Table-driven tests
func TestService_Update_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		userID      uuid.UUID
		input       UpdateUserInput
		repoFunc    func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error)
		wantErr     bool
		errContains string
	}{
		{
			name:   "valid display name",
			userID: uuid.New(),
			input: UpdateUserInput{
				DisplayName: stringPtr("John Doe"),
			},
			repoFunc: func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
				return &User{ID: id, DisplayName: input.DisplayName}, nil
			},
			wantErr: false,
		},
		{
			name:   "display name too long",
			userID: uuid.New(),
			input: UpdateUserInput{
				DisplayName: stringPtr(string(make([]byte, 101))),
			},
			wantErr:     true,
			errContains: "display name must be less than 100 characters",
		},
		{
			name:   "repository error",
			userID: uuid.New(),
			input: UpdateUserInput{
				DisplayName: stringPtr("Valid Name"),
			},
			repoFunc: func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
				return nil, errors.New("db error")
			},
			wantErr:     true,
			errContains: "failed to update user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				updateFunc: tt.repoFunc,
			}

			service := NewService(repo, zap.NewNop())
			_, err := service.Update(context.Background(), tt.userID, tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

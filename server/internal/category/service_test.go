package category

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockRepository is a mock implementation of the Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, input CreateCategoryInput) (*Category, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Category), args.Error(1)
}

func (m *MockRepository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	args := m.Called(ctx, id, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Category), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, userID uuid.UUID) ([]*Category, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Category), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, id, userID uuid.UUID, input UpdateCategoryInput) (*Category, error) {
	args := m.Called(ctx, id, userID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Category), args.Error(1)
}

func (m *MockRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	args := m.Called(ctx, id, userID)
	return args.Error(0)
}

func (m *MockRepository) NameExists(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	args := m.Called(ctx, userID, name)
	return args.Bool(0), args.Error(1)
}

func TestService_Create(t *testing.T) {
	logger := zap.NewNop()
	userID := uuid.New()

	t.Run("successful creation", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		color := "#FF5733"
		icon := "work"
		input := CreateInput{
			UserID: userID,
			Name:   "Work",
			Color:  &color,
			Icon:   &icon,
		}

		mockRepo.On("NameExists", mock.Anything, userID, "Work").Return(false, nil)
		// Mock List for color similarity check
		mockRepo.On("List", mock.Anything, userID).Return([]*Category{}, nil)
		mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(i CreateCategoryInput) bool {
			return i.UserID == userID && i.Name == "Work"
		})).Return(&Category{
			ID:     uuid.New(),
			UserID: userID,
			Name:   "Work",
			Color:  &color,
			Icon:   &icon,
		}, nil)

		result, err := service.Create(context.Background(), input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Work", result.Name)
		assert.NotNil(t, result.Color)
		assert.Equal(t, &icon, result.Icon)
		mockRepo.AssertExpectations(t)
	})

	t.Run("empty name fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		input := CreateInput{
			UserID: userID,
			Name:   "",
		}

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("name too long fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		input := CreateInput{
			UserID: userID,
			Name:   "This is a very long category name that exceeds fifty characters",
		}

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "less than 50 characters")
	})

	t.Run("duplicate name fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		input := CreateInput{
			UserID: userID,
			Name:   "Work",
		}

		mockRepo.On("NameExists", mock.Anything, userID, "Work").Return(true, nil)

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "already exists")
		mockRepo.AssertExpectations(t)
	})

	t.Run("invalid color format fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		color := "FF5733" // Missing #
		input := CreateInput{
			UserID: userID,
			Name:   "Work",
			Color:  &color,
		}

		mockRepo.On("NameExists", mock.Anything, userID, "Work").Return(false, nil)

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "hex format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("short color format fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		color := "#FF57" // Too short
		input := CreateInput{
			UserID: userID,
			Name:   "Work",
			Color:  &color,
		}

		mockRepo.On("NameExists", mock.Anything, userID, "Work").Return(false, nil)

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "hex format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error on name check", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		input := CreateInput{
			UserID: userID,
			Name:   "Work",
		}

		mockRepo.On("NameExists", mock.Anything, userID, "Work").Return(false, errors.New("db error"))

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error on create", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		input := CreateInput{
			UserID: userID,
			Name:   "Work",
		}

		mockRepo.On("NameExists", mock.Anything, userID, "Work").Return(false, nil)
		mockRepo.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		result, err := service.Create(context.Background(), input)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_GetByID(t *testing.T) {
	logger := zap.NewNop()
	categoryID := uuid.New()
	userID := uuid.New()

	t.Run("successful retrieval", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		expectedCategory := &Category{
			ID:     categoryID,
			UserID: userID,
			Name:   "Work",
		}

		mockRepo.On("GetByID", mock.Anything, categoryID, userID).Return(expectedCategory, nil)

		result, err := service.GetByID(context.Background(), categoryID, userID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, expectedCategory.ID, result.ID)
		assert.Equal(t, expectedCategory.Name, result.Name)
		mockRepo.AssertExpectations(t)
	})

	t.Run("category not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		mockRepo.On("GetByID", mock.Anything, categoryID, userID).Return(nil, sql.ErrNoRows)

		result, err := service.GetByID(context.Background(), categoryID, userID)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "not found")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_List(t *testing.T) {
	logger := zap.NewNop()
	userID := uuid.New()

	t.Run("successful list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		expectedCategories := []*Category{
			{ID: uuid.New(), UserID: userID, Name: "Work"},
			{ID: uuid.New(), UserID: userID, Name: "Personal"},
		}

		mockRepo.On("List", mock.Anything, userID).Return(expectedCategories, nil)

		result, err := service.List(context.Background(), userID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 2)
		mockRepo.AssertExpectations(t)
	})

	t.Run("empty list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		mockRepo.On("List", mock.Anything, userID).Return([]*Category{}, nil)

		result, err := service.List(context.Background(), userID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		mockRepo.On("List", mock.Anything, userID).Return(nil, errors.New("db error"))

		result, err := service.List(context.Background(), userID)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_Update(t *testing.T) {
	logger := zap.NewNop()
	categoryID := uuid.New()
	userID := uuid.New()

	t.Run("successful update", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		newName := "Updated Work"
		newColor := "#00FF00"
		input := UpdateInput{
			Name:  &newName,
			Color: &newColor,
		}

		expectedCategory := &Category{
			ID:     categoryID,
			UserID: userID,
			Name:   newName,
			Color:  &newColor,
		}

		mockRepo.On("Update", mock.Anything, categoryID, userID, mock.MatchedBy(func(i UpdateCategoryInput) bool {
			return i.Name != nil && *i.Name == newName && i.Color != nil && *i.Color == newColor
		})).Return(expectedCategory, nil)

		result, err := service.Update(context.Background(), categoryID, userID, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, newName, result.Name)
		assert.Equal(t, &newColor, result.Color)
		mockRepo.AssertExpectations(t)
	})

	t.Run("empty name fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		emptyName := ""
		input := UpdateInput{
			Name: &emptyName,
		}

		result, err := service.Update(context.Background(), categoryID, userID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("name too long fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		longName := "This is a very long category name that exceeds fifty characters"
		input := UpdateInput{
			Name: &longName,
		}

		result, err := service.Update(context.Background(), categoryID, userID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "less than 50 characters")
	})

	t.Run("invalid color format fails", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		invalidColor := "00FF00" // Missing #
		input := UpdateInput{
			Color: &invalidColor,
		}

		result, err := service.Update(context.Background(), categoryID, userID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "hex format")
	})

	t.Run("category not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		newName := "Updated"
		input := UpdateInput{
			Name: &newName,
		}

		mockRepo.On("Update", mock.Anything, categoryID, userID, mock.Anything).Return(nil, sql.ErrNoRows)

		result, err := service.Update(context.Background(), categoryID, userID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		newName := "Updated"
		input := UpdateInput{
			Name: &newName,
		}

		mockRepo.On("Update", mock.Anything, categoryID, userID, mock.Anything).Return(nil, errors.New("db error"))

		result, err := service.Update(context.Background(), categoryID, userID, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_Delete(t *testing.T) {
	logger := zap.NewNop()
	categoryID := uuid.New()
	userID := uuid.New()

	t.Run("successful deletion", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		mockRepo.On("Delete", mock.Anything, categoryID, userID).Return(nil)

		err := service.Delete(context.Background(), categoryID, userID)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("category not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		mockRepo.On("Delete", mock.Anything, categoryID, userID).Return(errors.New("category not found"))

		err := service.Delete(context.Background(), categoryID, userID)

		assert.Error(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, logger)

		mockRepo.On("Delete", mock.Anything, categoryID, userID).Return(errors.New("db error"))

		err := service.Delete(context.Background(), categoryID, userID)

		assert.Error(t, err)
		mockRepo.AssertExpectations(t)
	})
}

package category

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// MockRepositoryInterface is a mock implementation of RepositoryInterface for testing
type MockRepositoryInterface struct {
	mock.Mock
}

// Create mocks the Create method
func (m *MockRepositoryInterface) Create(ctx context.Context, input CreateCategoryInput) (*Category, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Category), args.Error(1)
}

// GetByID mocks the GetByID method
func (m *MockRepositoryInterface) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	args := m.Called(ctx, id, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Category), args.Error(1)
}

// List mocks the List method
func (m *MockRepositoryInterface) List(ctx context.Context, userID uuid.UUID) ([]*Category, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Category), args.Error(1)
}

// Update mocks the Update method
func (m *MockRepositoryInterface) Update(ctx context.Context, id, userID uuid.UUID, input UpdateCategoryInput) (*Category, error) {
	args := m.Called(ctx, id, userID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Category), args.Error(1)
}

// Delete mocks the Delete method
func (m *MockRepositoryInterface) Delete(ctx context.Context, id, userID uuid.UUID) error {
	args := m.Called(ctx, id, userID)
	return args.Error(0)
}

// NameExists mocks the NameExists method
func (m *MockRepositoryInterface) NameExists(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	args := m.Called(ctx, userID, name)
	return args.Bool(0), args.Error(1)
}
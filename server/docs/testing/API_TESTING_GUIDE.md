# API Testing Guide

## Overview

This guide provides comprehensive instructions for testing the DoneList API across multiple levels: unit tests, integration tests, E2E tests, load tests, and contract tests.

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Environment Setup](#test-environment-setup)
3. [Unit Testing](#unit-testing)
4. [Integration Testing](#integration-testing)
5. [E2E Testing](#e2e-testing)
6. [Load Testing](#load-testing)
7. [Contract Testing](#contract-testing)
8. [CI/CD Integration](#cicd-integration)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

## Testing Philosophy

### Test Pyramid

```
         /\
        /  \    E2E Tests (Few)
       /____\
      /      \   Integration Tests (Some)
     /________\
    /          \  Unit Tests (Many)
   /____________\
```

**Distribution:**
- **Unit Tests**: 70% - Fast, isolated, test business logic
- **Integration Tests**: 20% - Test component interactions, database operations
- **E2E Tests**: 10% - Test complete user workflows

### Testing Principles

1. **Fast Feedback**: Tests should run quickly in CI/CD
2. **Isolation**: Each test should be independent
3. **Repeatability**: Tests produce same results every time
4. **Clear Failures**: Test failures should clearly indicate the problem
5. **Maintainability**: Tests should be easy to update as code evolves

## Test Environment Setup

### Prerequisites

```bash
# Install Go 1.24+
go version

# Install Docker for testcontainers
docker --version

# Install test dependencies
cd server
go mod download
```

### Environment Variables

Create `.env.test` for test configuration:

```bash
# Database
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_DB=donelist_test

# Redis
REDIS_URL=redis://localhost:6379/1

# JWT
JWT_SECRET=test-secret-key-at-least-32-characters-long
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=7d

# API
API_PORT=8080
API_ENV=test
```

### Test Database Setup

```bash
# Using Docker
docker run -d \
  --name postgres-test \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=donelist_test \
  -p 5432:5432 \
  postgres:15-alpine

# Run migrations
make migrate-test
```

## Unit Testing

### Running Unit Tests

```bash
# Run all unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Run specific package
go test -v ./internal/auth

# Run specific test
go test -v -run TestHashPassword ./internal/auth
```

### Writing Unit Tests

**Example: Testing Service Layer**

```go
package user_test

import (
	"context"
	"testing"

	"github.com/dev-jelly/donelist/internal/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserService_CreateUser(t *testing.T) {
	// Arrange
	mockRepo := &mockUserRepository{}
	service := user.NewService(mockRepo, logger)

	ctx := context.Background()
	req := &user.CreateUserRequest{
		Email:       "test@example.com",
		Password:    "SecurePass123!",
		DisplayName: "Test User",
	}

	// Act
	result, err := service.CreateUser(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, req.Email, result.Email)
	assert.Equal(t, req.DisplayName, result.DisplayName)
}
```

### Unit Test Structure

```
internal/
├── auth/
│   ├── password.go
│   ├── password_test.go        # Unit tests for password functions
│   ├── service.go
│   └── service_test.go         # Unit tests for auth service
├── user/
│   ├── repository.go
│   ├── repository_test.go      # Unit tests with mock DB
│   ├── service.go
│   └── service_test.go         # Unit tests for user service
```

### Mocking Best Practices

```go
// Define interface for dependency
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
}

// Create mock implementation
type mockUserRepository struct {
	createFunc  func(ctx context.Context, user *User) error
	getByIDFunc func(ctx context.Context, id uuid.UUID) (*User, error)
}

func (m *mockUserRepository) Create(ctx context.Context, user *User) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, user)
	}
	return nil
}
```

## Integration Testing

### Running Integration Tests

```bash
# Run all integration tests
go test -v ./tests/integration

# Run with Docker testcontainers
go test -v ./tests/integration -timeout 10m

# Run specific integration test
go test -v -run TestAuthIntegration ./tests/integration

# Skip integration tests (for fast CI)
go test -short ./...
```

### Integration Test Setup

Integration tests use Docker testcontainers to spin up real PostgreSQL and Redis instances:

```go
func setupTestDB(t *testing.T) *testutil.TestDB {
	testDB, err := testutil.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() {
		testDB.Teardown()
	})

	return testDB
}
```

### Integration Test Example

```go
func TestAuthIntegration_RegisterAndLogin(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	testDB := setupTestDB(t)
	testDB.CleanTables(t)

	service := auth.NewService(
		auth.NewRepository(testDB.DB),
		user.NewRepository(testDB.DB),
		logger,
	)

	ctx := context.Background()

	// Register user
	registerReq := &auth.RegisterRequest{
		Email:       "test@example.com",
		Password:    "SecurePass123!",
		DisplayName: "Test User",
	}

	registerResp, err := service.Register(ctx, registerReq)
	require.NoError(t, err)
	require.NotEmpty(t, registerResp.AccessToken)

	// Login with credentials
	loginReq := &auth.LoginRequest{
		Email:    "test@example.com",
		Password: "SecurePass123!",
	}

	loginResp, err := service.Login(ctx, loginReq)
	require.NoError(t, err)
	assert.NotEmpty(t, loginResp.AccessToken)
	assert.NotEmpty(t, loginResp.RefreshToken)
}
```

### What to Test in Integration Tests

✅ **DO Test:**
- Database operations (CRUD)
- Transaction handling
- Foreign key constraints
- Index usage
- Query performance
- Repository-service interactions
- Error handling with real DB errors

❌ **DON'T Test:**
- Business logic (use unit tests)
- External API calls (use mocks)
- Complete user workflows (use E2E tests)

## E2E Testing

### Running E2E Tests

```bash
# Start API server in test mode
make run-test

# In another terminal, run E2E tests
go test -v ./tests/e2e

# Run with API URL override
API_URL=http://localhost:8080 go test -v ./tests/e2e
```

### E2E Test Structure

```go
func TestE2E_UserJourney_CreateCheckinWithCategory(t *testing.T) {
	// Setup
	apiURL := getAPIURL()
	client := newTestClient()

	// Step 1: Register and login
	authResp := registerAndLogin(t, client, apiURL)
	token := authResp.AccessToken

	// Step 2: Create category
	categoryResp := createCategory(t, client, apiURL, token, &CategoryRequest{
		Name:  "Work",
		Color: "#FF5733",
		Icon:  "💼",
	})

	// Step 3: Create checkin with category
	checkinResp := createCheckin(t, client, apiURL, token, &CheckinRequest{
		Title:       "Completed project documentation",
		Description: ptr("Updated API docs and testing guides"),
		CategoryID:  categoryResp.ID,
		Tags:        []string{"documentation", "testing"},
	})

	// Step 4: Verify checkin appears in timeline
	timelineResp := getTimeline(t, client, apiURL, token, time.Now())
	assert.Contains(t, timelineResp.Checkins, checkinResp.ID)

	// Step 5: Update checkin
	updateCheckin(t, client, apiURL, token, checkinResp.ID, &UpdateCheckinRequest{
		Title: "Completed comprehensive project documentation",
	})

	// Step 6: Delete checkin
	deleteCheckin(t, client, apiURL, token, checkinResp.ID)

	// Step 7: Verify deletion
	timelineResp = getTimeline(t, client, apiURL, token, time.Now())
	assert.NotContains(t, timelineResp.Checkins, checkinResp.ID)
}
```

### E2E Test Scenarios

1. **Authentication Flow**
   - Register → Login → Refresh Token → Logout

2. **Checkin Lifecycle**
   - Create → Update → Delete → Verify

3. **Category Management**
   - Create → List → Update → Merge → Delete

4. **Timeline Features**
   - Daily timeline → Weekly view → Monthly view
   - Filter by category → Filter by tags → Date ranges

5. **Premium Features**
   - Edit history → Version restore → Analytics

## Load Testing

### Using k6 for Load Testing

See [tests/load/README.md](../../tests/load/README.md) for detailed instructions.

```bash
# Auth load test
k6 run tests/load/auth_load_test.js

# Checkin operations load test
k6 run tests/load/checkin_load_test.js

# Spike test (sudden traffic increase)
k6 run tests/load/spike_test.js

# Stress test (find breaking point)
k6 run tests/load/stress_test.js

# Soak test (long-running stability)
k6 run tests/load/soak_test.js
```

### Performance Targets

| Metric | Target | Acceptable | Critical |
|--------|--------|------------|----------|
| P95 Response Time (Read) | < 200ms | < 500ms | < 1000ms |
| P95 Response Time (Write) | < 300ms | < 800ms | < 1500ms |
| Error Rate | < 0.1% | < 1% | < 5% |
| Concurrent Users | 100+ | 50+ | 20+ |
| Throughput | 500 req/s | 200 req/s | 50 req/s |

## Contract Testing

### API Contract Tests

Verify API responses match OpenAPI specification:

```go
func TestContract_AuthRegister(t *testing.T) {
	// Load OpenAPI spec
	spec, err := loadOpenAPISpec("../../docs/api/openapi.yaml")
	require.NoError(t, err)

	// Make request
	resp := makeAuthRegisterRequest(t, validRegisterRequest())

	// Validate against schema
	err = validateResponse(spec, "/auth/register", "POST", 201, resp)
	assert.NoError(t, err, "Response should match OpenAPI schema")
}
```

### Schema Validation

```bash
# Install validator
npm install -g @stoplight/spectral-cli

# Validate OpenAPI spec
spectral lint docs/api/openapi.yaml

# Validate example responses
spectral lint --ruleset docs/api/.spectral.yaml docs/api/openapi.yaml
```

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Test Suite

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Run Unit Tests
        run: |
          cd server
          go test -v -race -coverprofile=coverage.txt ./...

      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./coverage.txt

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: donelist_test
        ports:
          - 5432:5432
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Run Migrations
        run: |
          cd server
          make migrate-test

      - name: Run Integration Tests
        run: |
          cd server
          go test -v ./tests/integration -timeout 10m

  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Start API
        run: |
          docker-compose -f docker-compose.test.yml up -d
          sleep 10

      - name: Run E2E Tests
        run: |
          cd server
          go test -v ./tests/e2e

      - name: Cleanup
        if: always()
        run: docker-compose -f docker-compose.test.yml down
```

## Best Practices

### Test Naming Conventions

```go
// Pattern: Test<Function>_<Scenario>_<ExpectedBehavior>

func TestHashPassword_ValidPassword_ReturnsHash(t *testing.T) {}
func TestHashPassword_EmptyPassword_ReturnsError(t *testing.T) {}
func TestHashPassword_LongPassword_ReturnsError(t *testing.T) {}
```

### Table-Driven Tests

```go
func TestIsValidPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid password with all requirements",
			password: "SecurePass123!",
			wantErr:  false,
		},
		{
			name:     "too short",
			password: "Short1!",
			wantErr:  true,
			errMsg:   "at least 12 characters",
		},
		{
			name:     "no special characters",
			password: "SecurePass123",
			wantErr:  true,
			errMsg:   "at least 3 of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidPassword(tt.password)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

### Test Cleanup

```go
func TestWithCleanup(t *testing.T) {
	// Setup
	db := setupTestDB(t)

	// Register cleanup (runs even if test fails)
	t.Cleanup(func() {
		db.Close()
	})

	// Test code...
}
```

### Test Fixtures

```go
// testutil/fixtures.go
package testutil

func CreateTestUser(t *testing.T, db *TestDB, opts ...UserOption) *user.User {
	u := &user.User{
		ID:          uuid.New(),
		Email:       fmt.Sprintf("test-%s@example.com", uuid.New().String()[:8]),
		DisplayName: "Test User",
		CreatedAt:   time.Now(),
	}

	for _, opt := range opts {
		opt(u)
	}

	err := db.Insert(u)
	require.NoError(t, err)

	return u
}

// Usage:
user := CreateTestUser(t, db, WithEmail("specific@example.com"))
```

### Parallel Tests

```go
func TestParallel(t *testing.T) {
	t.Parallel() // Run in parallel with other parallel tests

	// Test code...
}
```

## Troubleshooting

### Common Issues

#### 1. Port Already in Use

```bash
# Find process using port
lsof -i :8080

# Kill process
kill -9 <PID>
```

#### 2. Test Database Connection Failed

```bash
# Check Docker container
docker ps

# Check logs
docker logs postgres-test

# Restart container
docker restart postgres-test
```

#### 3. Tests Flaking

- Add explicit timeouts
- Ensure proper cleanup
- Check for race conditions (`go test -race`)
- Verify test isolation

#### 4. Slow Tests

```bash
# Profile tests
go test -cpuprofile cpu.prof ./...
go tool pprof cpu.prof

# Identify slow tests
go test -v ./... 2>&1 | grep -E "PASS|FAIL" | sort -k3 -rn
```

### Debug Mode

```go
func TestWithDebug(t *testing.T) {
	// Enable verbose logging
	if testing.Verbose() {
		logger := zap.NewDevelopment()
		defer logger.Sync()

		// Use logger in test
	}
}

// Run with verbose flag
// go test -v ./...
```

## Test Coverage Goals

### Current Coverage

Run to generate current coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

### Coverage Targets

- **Critical Paths**: 90%+ coverage
  - Authentication
  - Payment processing
  - Data integrity operations

- **Business Logic**: 80%+ coverage
  - User management
  - Checkin operations
  - Category management

- **Infrastructure**: 60%+ coverage
  - Middleware
  - Utilities
  - Configuration

### Excluding from Coverage

```go
// coverage:ignore
func debugHelper() {
	// Debug code not tested in production
}
```

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Testcontainers Go](https://golang.testcontainers.org/)
- [k6 Load Testing](https://k6.io/docs/)
- [OpenAPI Validation](https://swagger.io/docs/specification/about/)

## Contributing

When adding new features:

1. Write unit tests first (TDD)
2. Add integration tests for database interactions
3. Add E2E test for user-facing workflows
4. Update this guide with new patterns
5. Ensure CI passes before merging

---

**Last Updated**: 2025-11-24
**Version**: 1.0.0
**Maintained By**: DoneList Development Team

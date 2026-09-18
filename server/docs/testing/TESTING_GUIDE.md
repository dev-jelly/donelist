# Donelist API Testing Guide

## Overview

This guide covers all aspects of testing the Donelist API, including unit tests, integration tests, E2E tests, performance tests, and security testing.

## Table of Contents

1. [Test Strategy](#test-strategy)
2. [Test Infrastructure](#test-infrastructure)
3. [Running Tests](#running-tests)
4. [Writing Tests](#writing-tests)
5. [Coverage Goals](#coverage-goals)
6. [CI/CD Integration](#cicd-integration)
7. [Performance Testing](#performance-testing)
8. [Security Testing](#security-testing)

## Test Strategy

### Test Pyramid

```
                 /\
                /  \
               / E2E \
              /________\
             /          \
            / Integration \
           /______________\
          /                \
         /   Unit Tests     \
        /____________________\
```

- **Unit Tests (70%)**: Fast, isolated tests for individual functions
- **Integration Tests (20%)**: Test interactions between components
- **E2E Tests (10%)**: Full user workflow tests

### Coverage Goals

- **Overall**: 80% minimum, 90% target
- **Critical Paths**: 95% minimum
- **Business Logic**: 90% minimum
- **Handlers**: 85% minimum
- **Utilities**: 95% minimum

## Test Infrastructure

### Required Services

Tests require PostgreSQL and Redis:

```bash
# Start test services
docker-compose -f docker-compose.test.yml up -d

# Stop test services
docker-compose -f docker-compose.test.yml down
```

### Test Database

```yaml
# docker-compose.test.yml
version: '3.8'
services:
  postgres-test:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: test
      POSTGRES_PASSWORD: test
      POSTGRES_DB: donelist_test
    ports:
      - "5433:5432"

  redis-test:
    image: redis:7-alpine
    ports:
      - "6380:6379"
```

### Environment Variables

```bash
# .env.test
DATABASE_URL=postgresql://test:test@localhost:5433/donelist_test?sslmode=disable
REDIS_URL=redis://localhost:6380
JWT_SECRET=test-secret-key-minimum-32-chars-long
GO_ENV=test
```

## Running Tests

### All Tests

```bash
# Run all tests with coverage
make test

# Or use the test runner script
./scripts/test-runner.sh
```

### Unit Tests Only

```bash
go test -v -race -cover ./...
```

### Integration Tests Only

```bash
go test -v -tags=integration ./tests/integration/...
```

### E2E Tests Only

```bash
go test -v -tags=e2e ./tests/e2e/...
```

### Specific Package

```bash
go test -v -cover ./internal/auth/...
```

### With Coverage Report

```bash
# Generate coverage
go test -coverprofile=coverage.out ./...

# View in browser
go tool cover -html=coverage.out

# View in terminal
go tool cover -func=coverage.out
```

### Watch Mode (for development)

```bash
# Install gotestsum
go install gotest.tools/gotestsum@latest

# Run in watch mode
gotestsum --watch
```

## Writing Tests

### Unit Test Example

```go
package auth_test

import (
    "context"
    "testing"

    "github.com/dev-jelly/donelist/internal/auth"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestService_Register(t *testing.T) {
    // Arrange
    service := setupTestService(t)
    input := auth.RegisterInput{
        Email:    "test@example.com",
        Password: "SecurePass123!",
    }

    // Act
    user, tokens, err := service.Register(context.Background(), input)

    // Assert
    require.NoError(t, err)
    assert.NotEmpty(t, user.ID)
    assert.Equal(t, input.Email, user.Email)
    assert.NotEmpty(t, tokens.AccessToken)
    assert.NotEmpty(t, tokens.RefreshToken)
}

func TestService_Register_DuplicateEmail(t *testing.T) {
    // Arrange
    service := setupTestService(t)
    input := auth.RegisterInput{
        Email:    "test@example.com",
        Password: "SecurePass123!",
    }

    // Create first user
    _, _, err := service.Register(context.Background(), input)
    require.NoError(t, err)

    // Act - Try to create duplicate
    _, _, err = service.Register(context.Background(), input)

    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "already registered")
}
```

### Integration Test Example

```go
//go:build integration
// +build integration

package integration_test

import (
    "context"
    "testing"

    "github.com/dev-jelly/donelist/internal/testutil"
    "github.com/stretchr/testify/suite"
)

type AuthIntegrationSuite struct {
    suite.Suite
    db     *sql.DB
    redis  *redis.Client
    server *httptest.Server
}

func (s *AuthIntegrationSuite) SetupSuite() {
    // Setup test database and services
    s.db = testutil.SetupTestDB(s.T())
    s.redis = testutil.SetupTestRedis(s.T())
    s.server = setupTestServer(s.db, s.redis)
}

func (s *AuthIntegrationSuite) TearDownSuite() {
    s.server.Close()
    s.db.Close()
    s.redis.Close()
}

func (s *AuthIntegrationSuite) SetupTest() {
    // Clean database before each test
    testutil.CleanDB(s.T(), s.db)
}

func (s *AuthIntegrationSuite) TestFullAuthFlow() {
    // Test complete authentication flow
    // 1. Register
    // 2. Login
    // 3. Access protected endpoint
    // 4. Refresh token
    // 5. Logout
}

func TestAuthIntegration(t *testing.T) {
    suite.Run(t, new(AuthIntegrationSuite))
}
```

### E2E Test Example

```go
//go:build e2e
// +build e2e

package e2e_test

import (
    "testing"

    "github.com/dev-jelly/donelist/internal/testutil"
)

func TestUserJourney_CreateAndManageCheckins(t *testing.T) {
    client := testutil.NewE2EClient(t)

    // Register new user
    user := client.Register("test@example.com", "SecurePass123!")

    // Create category
    category := client.CreateCategory("Work")

    // Create check-in
    checkin := client.CreateCheckin("Completed task", category.ID)

    // Update check-in
    client.UpdateCheckin(checkin.ID, "Completed important task")

    // List check-ins
    checkins := client.ListCheckins()
    require.Len(t, checkins, 1)

    // Delete check-in
    client.DeleteCheckin(checkin.ID)

    // Verify deletion
    checkins = client.ListCheckins()
    require.Len(t, checkins, 0)
}
```

### Table-Driven Tests

```go
func TestPasswordValidation(t *testing.T) {
    tests := []struct {
        name     string
        password string
        wantErr  bool
        errMsg   string
    }{
        {
            name:     "valid password",
            password: "SecurePass123!",
            wantErr:  false,
        },
        {
            name:     "too short",
            password: "Short1!",
            wantErr:  true,
            errMsg:   "at least 8 characters",
        },
        {
            name:     "no uppercase",
            password: "lowercase123!",
            wantErr:  true,
            errMsg:   "uppercase letter",
        },
        {
            name:     "no number",
            password: "NoNumbers!",
            wantErr:  true,
            errMsg:   "number",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidatePassword(tt.password)

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

## Test Utilities

### Test Fixtures

```go
// internal/testutil/fixtures.go

func CreateTestUser(t *testing.T, db *sql.DB) *User {
    user := &User{
        ID:    uuid.New().String(),
        Email: fmt.Sprintf("test-%s@example.com", uuid.New().String()[:8]),
    }

    _, err := db.Exec(`
        INSERT INTO users (id, email, password_hash)
        VALUES ($1, $2, $3)
    `, user.ID, user.Email, "hashed_password")

    require.NoError(t, err)
    return user
}

func CreateTestCheckin(t *testing.T, db *sql.DB, userID string) *Checkin {
    checkin := &Checkin{
        ID:      uuid.New().String(),
        UserID:  userID,
        Message: "Test check-in",
    }

    _, err := db.Exec(`
        INSERT INTO checkins (id, user_id, message)
        VALUES ($1, $2, $3)
    `, checkin.ID, checkin.UserID, checkin.Message)

    require.NoError(t, err)
    return checkin
}
```

### Mock Services

```go
// internal/testutil/mocks.go

type MockEmailService struct {
    mock.Mock
}

func (m *MockEmailService) SendEmail(to, subject, body string) error {
    args := m.Called(to, subject, body)
    return args.Error(0)
}

// Usage in tests
func TestSendWelcomeEmail(t *testing.T) {
    mockEmail := new(MockEmailService)
    mockEmail.On("SendEmail", "user@example.com", "Welcome", mock.Anything).Return(nil)

    service := NewUserService(mockEmail)
    err := service.SendWelcomeEmail("user@example.com")

    assert.NoError(t, err)
    mockEmail.AssertExpectations(t)
}
```

## Coverage Goals

### Current Coverage Status

```
Package                              Coverage
----------------------------------------
internal/auth                         85.3%
internal/checkin                      78.2%
internal/category                     82.1%
internal/tag                          76.5%
internal/user                         88.9%
internal/websocket                    45.2%  ⚠️
internal/health                       80.2%
internal/signing                      79.3%
----------------------------------------
Overall                               75.4%
```

### Priority Areas for Improvement

1. **WebSocket Package** (45.2% → 80% target)
   - Add unit tests for handlers
   - Integration tests for message flow
   - Connection lifecycle tests

2. **Statistics Package** (19.0% → 80% target)
   - Test calculation logic
   - Test aggregation functions
   - Test date range handling

3. **Calendar Package** (8.7% → 80% target)
   - Test calendar generation
   - Test date calculations
   - Test event mapping

## CI/CD Integration

### GitHub Actions Workflow

Tests run automatically on:
- Every push to main/develop
- Every pull request
- Nightly at 2 AM UTC

```yaml
# .github/workflows/test.yml
name: Tests

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]
  schedule:
    - cron: '0 2 * * *'  # Nightly at 2 AM UTC

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:15-alpine
        env:
          POSTGRES_PASSWORD: test
          POSTGRES_DB: donelist_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

      redis:
        image: redis:7-alpine
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 6379:6379

    steps:
    - uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.24'

    - name: Run tests
      run: |
        go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
      env:
        DATABASE_URL: postgresql://postgres:test@localhost:5432/donelist_test?sslmode=disable
        REDIS_URL: redis://localhost:6379

    - name: Upload coverage
      uses: codecov/codecov-action@v4
      with:
        files: ./coverage.out
```

## Performance Testing

### Load Testing with k6

```bash
# Install k6
brew install k6

# Run load test
k6 run tests/load/checkin_load_test.js
```

Example k6 test:

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
    stages: [
        { duration: '2m', target: 100 },  // Ramp up
        { duration: '5m', target: 100 },  // Stay at 100 users
        { duration: '2m', target: 0 },    // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'],  // 95% of requests < 500ms
        http_req_failed: ['rate<0.01'],    // < 1% failures
    },
};

export default function() {
    let response = http.get('http://localhost:8080/api/v1/health');

    check(response, {
        'status is 200': (r) => r.status === 200,
        'response time < 500ms': (r) => r.timings.duration < 500,
    });

    sleep(1);
}
```

### Benchmark Tests

```go
func BenchmarkPasswordHash(b *testing.B) {
    password := "SecurePass123!"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = HashPassword(password)
    }
}

func BenchmarkJWTGeneration(b *testing.B) {
    manager, _ := NewJWTManager(config)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = manager.GenerateAccessToken("user-id")
    }
}
```

Run benchmarks:

```bash
go test -bench=. -benchmem ./...
```

## Security Testing

### Static Analysis

```bash
# Install gosec
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Run security scan
gosec ./...
```

### Dependency Scanning

```bash
# Check for known vulnerabilities
go list -json -m all | nancy sleuth
```

### SQL Injection Testing

```go
func TestSQLInjectionProtection(t *testing.T) {
    tests := []string{
        "'; DROP TABLE users; --",
        "1' OR '1'='1",
        "admin' --",
    }

    for _, malicious := range tests {
        t.Run(malicious, func(t *testing.T) {
            _, err := service.GetUser(malicious)
            // Should fail safely, not cause SQL injection
            assert.Error(t, err)
        })
    }
}
```

## Best Practices

### DO

- ✅ Write tests before or alongside code (TDD)
- ✅ Use table-driven tests for multiple scenarios
- ✅ Mock external dependencies
- ✅ Test error cases and edge cases
- ✅ Use descriptive test names
- ✅ Keep tests fast and independent
- ✅ Use test fixtures for common setup
- ✅ Run tests before committing

### DON'T

- ❌ Test implementation details
- ❌ Share state between tests
- ❌ Use production database for tests
- ❌ Ignore failing tests
- ❌ Write flaky tests
- ❌ Skip error case testing
- ❌ Commit without running tests

## Troubleshooting

### Tests Fail Locally

```bash
# Clean test database
make clean-test-db

# Restart test services
docker-compose -f docker-compose.test.yml restart

# Clear test cache
go clean -testcache
```

### Race Condition Detected

```bash
# Run with race detector
go test -race ./...

# Fix data races using proper synchronization
```

### Coverage Not Updating

```bash
# Remove old coverage data
rm -rf coverage/
go clean -cache
go clean -testcache

# Regenerate coverage
go test -coverprofile=coverage.out ./...
```

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Testify Documentation](https://github.com/stretchr/testify)
- [k6 Documentation](https://k6.io/docs/)
- [GoSec Documentation](https://github.com/securego/gosec)

## Contributing

When adding new features:

1. Write tests first (TDD)
2. Ensure coverage meets thresholds
3. Run full test suite locally
4. Verify CI pipeline passes
5. Update test documentation

## Support

For testing questions:
- GitHub Issues: https://github.com/dev-jelly/donelist/issues
- Documentation: https://docs.donelist.io/testing
- Email: dev@donelist.io

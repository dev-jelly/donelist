# Test Infrastructure Documentation

## Overview
This document outlines the comprehensive test suite infrastructure for the donelist server project.

## Test Strategy

### Test Pyramid
- **Unit Tests (70%)**: Fast, isolated tests for business logic
- **Integration Tests (20%)**: Database and external service integration
- **End-to-End Tests (10%)**: Full API flow testing

### Coverage Goals
- Overall coverage: 80%+
- Critical paths: 95%+
- New code: 80%+ before merge

## Test Infrastructure Components

### 1. Test Database (testutil package)
Location: `internal/testutil/`

#### TestDB
Provides PostgreSQL testcontainer setup for integration tests.

```go
// Example usage
testDB := testutil.SetupTestDB(t)
defer testDB.TearDown(t)

// Clean tables between tests
testDB.CleanTables(t)
```

#### Fixtures
Helper functions to create test data without import cycles:

```go
fixtures := testutil.NewFixtures(testDB)

// Create test user
userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")

// Create test category
categoryID := fixtures.CreateTestCategory(t, userID, "Work",
    testutil.StringPtr("#FF5733"), testutil.StringPtr("📁"))

// Create test checkin
checkinID := fixtures.CreateTestCheckin(t, userID, &categoryID, "Test content", 30)
```

**Important**: Fixtures return UUIDs to avoid circular dependencies. Fetch full objects using repositories when needed.

### 2. Unit Tests

#### Naming Convention
- File: `*_test.go`
- Function: `Test<Package><Function>_<Scenario>`
- Example: `TestUserRepository_Create_Success`

#### Table-Driven Tests
Use for testing multiple scenarios:

```go
func TestCalculateDiscount(t *testing.T) {
    tests := []struct {
        name     string
        input    float64
        tier     string
        expected float64
        wantErr  bool
    }{
        {
            name:     "Premium tier with $100",
            input:    100.0,
            tier:     "premium",
            expected: 80.0,
            wantErr:  false,
        },
        {
            name:     "Free tier with $100",
            input:    100.0,
            tier:     "free",
            expected: 100.0,
            wantErr:  false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := CalculateDiscount(tt.input, tt.tier)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### 3. Integration Tests

Location: `tests/integration/`

#### Database Integration Tests
Use testcontainers for real database testing:

```go
func TestAuthenticationFlow(t *testing.T) {
    // Setup
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    userRepo := user.NewRepository(testDB.DB)
    authService := auth.NewService(userRepo, ...)

    // Test registration
    user, tokens, err := authService.Register(ctx, auth.RegisterInput{
        Email:    "test@example.com",
        Password: "SecurePass123!",
    })
    require.NoError(t, err)
    assert.NotNil(t, user)
    assert.NotEmpty(t, tokens.AccessToken)
}
```

#### External Service Integration
Mock external services for integration tests:

```go
// Use gomock for mocking
mockCtrl := gomock.NewController(t)
defer mockCtrl.Finish()

mockEmailService := mocks.NewMockEmailService(mockCtrl)
mockEmailService.EXPECT().
    SendEmail(gomock.Any(), gomock.Any()).
    Return(nil).
    Times(1)
```

### 4. End-to-End API Tests

Location: `tests/e2e/`

Test full request/response cycles:

```go
func TestCheckinCRUD(t *testing.T) {
    // Setup test server
    router := setupTestRouter(t)
    server := httptest.NewServer(router)
    defer server.Close()

    // Register and login
    accessToken := registerAndLogin(t, server.URL)

    // Create checkin
    checkin := createCheckin(t, server.URL, accessToken, CheckinInput{
        Content:         "Test checkin",
        DurationMinutes: 30,
    })

    // Verify checkin was created
    assert.NotEmpty(t, checkin.ID)

    // Test update, get, delete...
}
```

### 5. Load Testing

Location: `tests/load/`

Use k6 for load testing:

```javascript
// tests/load/checkin_load_test.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 20 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
};

export default function () {
  const url = 'http://localhost:8080/api/v1/checkins';
  const payload = JSON.stringify({
    content: 'Load test checkin',
    duration_minutes: 30,
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${__ENV.ACCESS_TOKEN}`,
    },
  };

  const res = http.post(url, payload, params);
  check(res, {
    'status is 201': (r) => r.status === 201,
    'response time < 200ms': (r) => r.timings.duration < 200,
  });

  sleep(1);
}
```

Run with: `k6 run tests/load/checkin_load_test.js`

### 6. Mutation Testing

Use `go-mutesting` for mutation testing critical paths:

```bash
go-mutesting ./internal/auth/...
```

### 7. Test Coverage

#### Generate Coverage Report
```bash
# Run tests with coverage
go test ./... -coverprofile=coverage.out

# View coverage in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html

# Check coverage threshold
go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//' | \
    awk '{if ($1 < 80) exit 1}'
```

#### Coverage by Package
```bash
# Get coverage by package
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E '^github.com'
```

### 8. Mocking Strategy

#### Use gomock for interfaces
```bash
# Generate mocks
go install github.com/golang/mock/mockgen@latest

# Generate mock for interface
mockgen -source=internal/email/service.go -destination=internal/email/mock_service.go -package=email
```

#### Mock Usage
```go
mockCtrl := gomock.NewController(t)
defer mockCtrl.Finish()

mockEmailService := NewMockEmailService(mockCtrl)
mockEmailService.EXPECT().
    SendWelcomeEmail(gomock.Any(), gomock.Eq("test@example.com")).
    Return(nil)
```

## Running Tests

### Run All Tests
```bash
make test
# or
go test ./...
```

### Run Specific Package
```bash
go test ./internal/user/...
```

### Run with Race Detector
```bash
go test -race ./...
```

### Run Integration Tests Only
```bash
go test -tags=integration ./tests/integration/...
```

### Run with Verbose Output
```bash
go test -v ./...
```

### Run Specific Test
```bash
go test -run TestUserRepository_Create ./internal/user/...
```

## CI/CD Integration

### GitHub Actions Workflow
`.github/workflows/test.yml`:

```yaml
name: Tests

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]

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
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.24'

    - name: Cache Go modules
      uses: actions/cache@v3
      with:
        path: ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
        restore-keys: |
          ${{ runner.os }}-go-

    - name: Download dependencies
      run: go mod download

    - name: Run unit tests
      run: go test -v -race -coverprofile=coverage.out ./...

    - name: Check coverage threshold
      run: |
        COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        echo "Coverage: $COVERAGE%"
        if (( $(echo "$COVERAGE < 80" | bc -l) )); then
          echo "Coverage is below 80%"
          exit 1
        fi

    - name: Upload coverage to Codecov
      uses: codecov/codecov-action@v3
      with:
        files: ./coverage.out

    - name: Run integration tests
      run: go test -v -tags=integration ./tests/integration/...
      env:
        DATABASE_URL: postgresql://postgres:test@localhost:5432/donelist_test?sslmode=disable
        REDIS_URL: redis://localhost:6379

    - name: Run load tests
      if: github.event_name == 'push' && github.ref == 'refs/heads/main'
      run: |
        docker run --rm -i grafana/k6 run - < tests/load/checkin_load_test.js
```

## Best Practices

### 1. Test Isolation
- Each test should be independent
- Use `testDB.CleanTables(t)` between tests
- Don't rely on test execution order

### 2. Test Data
- Use fixtures for common test data
- Keep test data minimal and relevant
- Clean up test data after tests

### 3. Assertions
- Use `require` for critical assertions (stops test on failure)
- Use `assert` for non-critical assertions (continues test)
- Provide descriptive error messages

### 4. Test Organization
- Group related tests with `t.Run()`
- Use descriptive test names
- Keep tests focused and small

### 5. Performance
- Mock slow external services
- Use parallel tests when possible: `t.Parallel()`
- Keep integration tests separate from unit tests

## Common Patterns

### Testing Repository Methods
```go
func TestUserRepository_Create(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    repo := user.NewRepository(testDB.DB)
    ctx := context.Background()

    t.Run("Success", func(t *testing.T) {
        // Test implementation
    })

    t.Run("Duplicate email", func(t *testing.T) {
        // Test implementation
    })
}
```

### Testing Service Methods
```go
func TestAuthService_Login(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    userRepo := user.NewRepository(testDB.DB)
    authService := auth.NewService(userRepo, ...)
    ctx := context.Background()

    // Setup test user
    fixtures := testutil.NewFixtures(testDB)
    userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")

    t.Run("Valid credentials", func(t *testing.T) {
        user, tokens, err := authService.Login(ctx, auth.LoginInput{
            Email:    "test@example.com",
            Password: "password",
        })
        require.NoError(t, err)
        assert.NotNil(t, user)
        assert.NotEmpty(t, tokens.AccessToken)
    })
}
```

## Troubleshooting

### Tests Failing in CI but Passing Locally
- Check environment variables
- Verify service dependencies (postgres, redis)
- Check for race conditions: run with `-race` flag

### Import Cycle Errors
- Use fixtures that return UUIDs instead of objects
- Avoid importing domain packages in testutil
- Consider creating package-specific test helpers

### Slow Tests
- Profile tests: `go test -cpuprofile=cpu.prof`
- Use mocks for external services
- Run heavy tests separately with build tags

## Future Enhancements

1. **Mutation Testing**: Implement automated mutation testing for critical paths
2. **Chaos Engineering**: Add chaos testing for resilience
3. **Performance Benchmarks**: Add benchmark tests for critical operations
4. **Contract Testing**: Implement contract tests for external APIs
5. **Visual Regression Testing**: Add visual testing for frontend components
6. **A/B Testing Framework**: Implement A/B testing capabilities

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Testcontainers Go](https://golang.testcontainers.org/)
- [k6 Documentation](https://k6.io/docs/)
- [gomock Documentation](https://github.com/golang/mock)

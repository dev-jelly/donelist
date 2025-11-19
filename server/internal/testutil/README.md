# Test Utilities Package

## Overview
This package provides shared testing utilities and fixtures to help write consistent, maintainable tests across the project.

## Components

### TestDB
Manages PostgreSQL testcontainer lifecycle for integration tests.

```go
testDB := testutil.SetupTestDB(t)
defer testDB.TearDown(t)
```

**Features:**
- Automatic PostgreSQL container creation
- Migration execution on startup
- Connection pooling configuration
- Cleanup on test completion

**Methods:**
- `SetupTestDB(t *testing.T) *TestDB`: Creates and initializes test database
- `TearDown(t *testing.T)`: Cleans up container and connections
- `CleanTables(t *testing.T)`: Truncates all tables for test isolation
- `ExecSQL(t *testing.T, query string, args ...interface{})`: Executes SQL statement
- `MustExec(query string, args ...interface{})`: Executes SQL, panics on error

### Fixtures
Helper functions to create test data without circular dependencies.

**Design Philosophy:**
- Returns UUIDs instead of domain objects to avoid import cycles
- Minimal parameters with sensible defaults
- Explicit where it matters (email, userID relationships)

## Usage Examples

### Basic User Test
```go
func TestUserRepository(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    repo := user.NewRepository(testDB.DB)
    fixtures := testutil.NewFixtures(testDB)

    // Create test user
    userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")

    // Fetch full user object
    user, err := repo.GetByID(context.Background(), userID)
    require.NoError(t, err)
    assert.Equal(t, "test@example.com", user.Email)
}
```

### Checkin Test with Category
```go
func TestCheckinWithCategory(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    fixtures := testutil.NewFixtures(testDB)

    // Create user and category
    userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")
    categoryID := fixtures.CreateTestCategory(t, userID, "Work",
        testutil.StringPtr("#FF5733"),
        testutil.StringPtr("📁"))

    // Create checkin
    checkinID := fixtures.CreateTestCheckin(t, userID, &categoryID, "Completed task", 30)

    // Verify checkin
    repo := checkin.NewRepository(testDB.DB)
    c, err := repo.GetByID(context.Background(), checkinID, userID)
    require.NoError(t, err)
    assert.Equal(t, "Completed task", c.Content)
}
```

### Test with Tags
```go
func TestCheckinWithTags(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    fixtures := testutil.NewFixtures(testDB)
    userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")

    // Create checkin with tags
    checkinID := fixtures.CreateCheckinWithTags(t, userID, "Tagged work",
        []string{"urgent", "backend"})

    // Verify tags are associated
    repo := checkin.NewRepository(testDB.DB)
    c, err := repo.GetByID(context.Background(), checkinID, userID)
    require.NoError(t, err)
    assert.Len(t, c.Tags, 2)
}
```

### Premium User Test
```go
func TestPremiumFeature(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    fixtures := testutil.NewFixtures(testDB)

    // Create premium user
    premiumUserID := fixtures.CreatePremiumUser(t, "premium@example.com",
        "password", "premiumuser")

    // Test premium-only feature
    service := NewPremiumService(...)
    result, err := service.UsePremiumFeature(context.Background(), premiumUserID)
    require.NoError(t, err)
    assert.NotNil(t, result)
}
```

### Test Isolation Between Tests
```go
func TestMultipleScenarios(t *testing.T) {
    testDB := testutil.SetupTestDB(t)
    defer testDB.TearDown(t)

    repo := user.NewRepository(testDB.DB)
    fixtures := testutil.NewFixtures(testDB)

    t.Run("Scenario 1", func(t *testing.T) {
        userID := fixtures.CreateTestUser(t, "test1@example.com", "password", "user1")
        // Test implementation

        // Clean for next test
        testDB.CleanTables(t)
    })

    t.Run("Scenario 2", func(t *testing.T) {
        userID := fixtures.CreateTestUser(t, "test2@example.com", "password", "user2")
        // Test implementation
    })
}
```

## Available Fixtures

### CreateTestUser
Creates a free-tier user.

```go
userID := fixtures.CreateTestUser(t, email, password, username string) uuid.UUID
```

**Parameters:**
- `email`: User's email (must be unique)
- `password`: Plain text password (will be hashed)
- `username`: Display username

**Returns:** User UUID

### CreatePremiumUser
Creates a premium-tier user.

```go
userID := fixtures.CreatePremiumUser(t, email, password, username string) uuid.UUID
```

**Parameters:** Same as CreateTestUser

**Returns:** User UUID

### CreateTestCategory
Creates a category for a user.

```go
categoryID := fixtures.CreateTestCategory(t, userID uuid.UUID, name string,
    color *string, icon *string) uuid.UUID
```

**Parameters:**
- `userID`: Owner's UUID
- `name`: Category name
- `color`: Optional color (e.g., "#FF5733")
- `icon`: Optional icon emoji

**Returns:** Category UUID

### CreateTestTag
Creates a tag for a user.

```go
tagID := fixtures.CreateTestTag(t, userID uuid.UUID, name string) uuid.UUID
```

**Parameters:**
- `userID`: Owner's UUID
- `name`: Tag name

**Returns:** Tag UUID

### CreateTestCheckin
Creates a basic checkin.

```go
checkinID := fixtures.CreateTestCheckin(t, userID uuid.UUID, categoryID *uuid.UUID,
    content string, duration int) uuid.UUID
```

**Parameters:**
- `userID`: Owner's UUID
- `categoryID`: Optional category UUID
- `content`: Checkin content/description
- `duration`: Duration in minutes

**Returns:** Checkin UUID

### CreateCheckinWithTags
Creates a checkin with associated tags.

```go
checkinID := fixtures.CreateCheckinWithTags(t, userID uuid.UUID,
    content string, tagNames []string) uuid.UUID
```

**Parameters:**
- `userID`: Owner's UUID
- `content`: Checkin content
- `tagNames`: Slice of tag names to create and associate

**Returns:** Checkin UUID

### CreateOldCheckin
Creates a checkin that's 3 hours old (useful for testing time-based features).

```go
checkinID := fixtures.CreateOldCheckin(t, userID uuid.UUID, content string) uuid.UUID
```

**Parameters:**
- `userID`: Owner's UUID
- `content`: Checkin content

**Returns:** Checkin UUID

## Helper Functions

### StringPtr
Converts string to pointer (useful for optional fields).

```go
color := testutil.StringPtr("#FF5733")
icon := testutil.StringPtr("📁")
categoryID := fixtures.CreateTestCategory(t, userID, "Work", color, icon)
```

## Best Practices

### 1. Always Clean Up
```go
testDB := testutil.SetupTestDB(t)
defer testDB.TearDown(t)  // Always defer cleanup
```

### 2. Use CleanTables Between Sub-Tests
```go
t.Run("Test 1", func(t *testing.T) {
    // Test implementation
    testDB.CleanTables(t)
})

t.Run("Test 2", func(t *testing.T) {
    // Test implementation (clean slate)
})
```

### 3. Fetch Full Objects When Needed
Fixtures return IDs to avoid import cycles. Fetch full objects using repositories:

```go
userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")

// Fetch full user object
repo := user.NewRepository(testDB.DB)
user, err := repo.GetByID(context.Background(), userID)
```

### 4. Use Descriptive Test Data
```go
// Good
userID := fixtures.CreateTestUser(t, "john.doe@example.com", "SecurePass123!", "johndoe")

// Less clear
userID := fixtures.CreateTestUser(t, "a@b.com", "pass", "user")
```

### 5. Don't Share State Between Tests
Each test should create its own data or use `CleanTables()`.

## Migration Path for Existing Tests

If you have tests using the old fixture methods that returned full objects:

**Old:**
```go
testUser := fixtures.CreateTestUser(t, "test@example.com", "password")
assert.Equal(t, "test@example.com", testUser.Email)
```

**New:**
```go
userID := fixtures.CreateTestUser(t, "test@example.com", "password", "testuser")
user, err := repo.GetByID(ctx, userID)
require.NoError(t, err)
assert.Equal(t, "test@example.com", user.Email)
```

## Troubleshooting

### Import Cycle Errors
If you see import cycle errors:
- ✅ DO: Use fixtures that return UUIDs
- ❌ DON'T: Import domain packages in testutil
- ✅ DO: Fetch full objects using repositories

### Container Startup Issues
If PostgreSQL container fails to start:
- Check Docker is running
- Ensure ports 5432 isn't already in use
- Check disk space
- Increase timeout in `SetupTestDB`

### Slow Tests
If tests are running slow:
- Use `testDB.CleanTables(t)` instead of recreating containers
- Run tests in parallel: `t.Parallel()` (when tests are independent)
- Consider mocking database for pure unit tests

## Contributing

When adding new fixtures:
1. Return UUIDs, not domain objects
2. Use minimal parameters with sensible defaults
3. Document in this README
4. Add example usage
5. Ensure no import cycles

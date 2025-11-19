# Premium Historical Edit - Test Plan

## Overview
This document outlines the testing strategy for the premium historical edit feature integration.

## Test Levels

### 1. Unit Tests
Test individual components in isolation.

#### Subscription Middleware Tests
**File**: `internal/middleware/subscription_test.go`

- [ ] Test tier enrichment for valid users
- [ ] Test expired tier downgrade to free
- [ ] Test invalid tier defaults to free
- [ ] Test missing user_id handling

#### Time Boundary Validation Tests
**File**: `internal/checkin/time_utils_test.go` (already exists)

- [x] Test CanEditCheckin for free users within 2-hour window
- [x] Test CanEditCheckin for free users beyond 2-hour window
- [x] Test CanEditCheckin for premium users at any time
- [x] Test timezone safety (UTC calculations)
- [x] Test boundary conditions (exactly 2 hours)

#### Repository Tests
**File**: `internal/checkin/repository_test.go` (already exists)

- [x] Test optimistic locking (version mismatch)
- [x] Test edit history preservation
- [x] Test concurrent update detection
- [x] Test version increment on successful update

### 2. Integration Tests
Test component interactions.

#### Service Layer Integration
**File**: `internal/checkin/service_integration_test.go`

```go
// Test: Premium user can edit historical checkin
func TestService_Update_PremiumHistoricalEdit(t *testing.T) {
    // Setup: Create checkin 5 hours old
    // Setup: User with premium tier
    // Execute: Update checkin
    // Assert: Update succeeds
    // Assert: Version incremented
    // Assert: Edit history created
}

// Test: Free user denied for historical edit
func TestService_Update_FreeUserDeniedHistorical(t *testing.T) {
    // Setup: Create checkin 3 hours old
    // Setup: User with free tier
    // Execute: Update checkin
    // Assert: EditPermissionError returned
    // Assert: No changes in database
}

// Test: Concurrent edit detection
func TestService_Update_ConcurrentEditDetected(t *testing.T) {
    // Setup: Create checkin
    // Setup: Get initial version
    // Execute: Update with stale version
    // Assert: Concurrent edit error
}
```

### 3. API Integration Tests
Test full HTTP request/response cycle.

#### Handler Integration Tests
**File**: `tests/integration/checkin_api_integration_test.go`

```bash
# Run with test database
go test -v ./tests/integration -tags=integration

# Test scenarios:
1. Success: Premium historical edit (200 OK)
2. Denied: Free user historical edit (403 Forbidden)
3. Conflict: Concurrent edit (409 Conflict)
4. Not Found: Non-existent checkin (404)
5. Validation: Invalid content (422)
6. Unauthorized: Missing token (401)
```

### 4. End-to-End Tests
Test complete user workflows.

#### E2E Test Scenarios
**Tool**: Playwright or similar

1. **Premium User Historical Edit Workflow**
   ```
   - Login as premium user
   - Create checkin
   - Wait 3 hours (or manipulate time)
   - Edit checkin successfully
   - Verify edit appears in history
   - Verify audit log entry
   ```

2. **Free User Upgrade Prompt Workflow**
   ```
   - Login as free user
   - Create checkin
   - Wait 3 hours
   - Attempt edit
   - See "Premium Required" modal
   - Click upgrade button
   - Redirect to subscription page
   ```

3. **Concurrent Edit Conflict Workflow**
   ```
   - Open checkin in two browser tabs
   - Edit in tab 1, save
   - Edit in tab 2, attempt save
   - See conflict message
   - Refresh to see latest version
   ```

## Manual Test Cases

### Test Case 1: Premium Historical Edit Success
**Preconditions**:
- User has premium subscription
- Checkin exists, created 5 hours ago

**Steps**:
1. GET `/api/v1/checkins/{id}` to get current version
2. PATCH `/api/v1/checkins/{id}` with:
   ```json
   {
     "content": "Updated content",
     "edit_reason": "Fixed typo",
     "version": 1
   }
   ```
3. Verify response: 200 OK with version = 2
4. GET `/api/v1/checkins/{id}/history` to verify edit history

**Expected Results**:
- HTTP 200 OK
- Checkin updated with new content
- `is_edited` = true
- `edit_count` = 1
- `version` incremented to 2
- Edit history entry created
- Audit log entry created

---

### Test Case 2: Free User Edit Denial
**Preconditions**:
- User has free subscription
- Checkin exists, created 3 hours ago

**Steps**:
1. GET `/api/v1/checkins/{id}` to get current version
2. PATCH `/api/v1/checkins/{id}` with update data

**Expected Results**:
- HTTP 403 Forbidden
- Error response:
  ```json
  {
    "error": "edit_permission_denied",
    "message": "premium subscription required to edit check-ins older than 2 hours",
    "required_tier": "premium",
    "current_tier": "free",
    "upgrade_url": "/api/v1/subscription/upgrade",
    "edit_window_expired": true,
    "edit_window_hours": 2
  }
  ```
- No database changes
- Audit log entry created (denied attempt)

---

### Test Case 3: Concurrent Edit Conflict
**Preconditions**:
- Checkin exists with version = 1

**Steps**:
1. User A gets checkin (version = 1)
2. User B gets checkin (version = 1)
3. User A updates checkin successfully (version becomes 2)
4. User B attempts to update with version = 1

**Expected Results** (for User B):
- HTTP 409 Conflict
- Error response:
  ```json
  {
    "error": "concurrent_edit_detected",
    "message": "This check-in was modified by another request. Please refresh and try again.",
    "checkin_id": "uuid"
  }
  ```
- User B's update not applied
- User A's update preserved

---

### Test Case 4: Free User Within Edit Window
**Preconditions**:
- User has free subscription
- Checkin created 30 minutes ago (within 2-hour window)

**Steps**:
1. PATCH `/api/v1/checkins/{id}` with update data

**Expected Results**:
- HTTP 200 OK
- Update succeeds
- Normal edit flow

---

### Test Case 5: Version Mismatch Handling
**Preconditions**:
- Checkin exists with version = 5

**Steps**:
1. Attempt update with version = 3 (stale version)

**Expected Results**:
- HTTP 409 Conflict
- Concurrent edit error
- Client should refresh and retry

---

## Performance Tests

### Load Testing Scenarios

#### Scenario 1: Concurrent Updates to Same Checkin
```bash
# Use Apache Bench or similar
ab -n 100 -c 10 -T application/json -p update_payload.json \
   https://api.example.com/api/v1/checkins/{id}
```

**Expected**:
- Only 1 update succeeds (first to commit)
- Other 99 requests return 409 Conflict
- No data corruption
- Database remains consistent

#### Scenario 2: High Volume Edits
```bash
# Test 1000 edits to different checkins
for i in {1..1000}; do
  curl -X PATCH /api/v1/checkins/${CHECKIN_IDS[$i]} \
       -H "Authorization: Bearer $TOKEN" \
       -d '{"content":"Update '$i'","version":1}'
done
```

**Expected**:
- All valid requests succeed
- Audit logs created for all
- Edit history preserved for all
- Response time < 500ms (p95)

---

## Security Tests

### Test Case S1: Authorization Bypass Attempt
**Steps**:
1. User A creates checkin
2. User B attempts to edit User A's checkin

**Expected**:
- HTTP 404 Not Found (checkin not found for User B)
- OR HTTP 403 Forbidden
- No data modification

### Test Case S2: Tier Manipulation Attempt
**Steps**:
1. Free user attempts to send `user_tier: "premium"` in request body

**Expected**:
- Tier from database used (not from request)
- Free user still denied for historical edit
- Server-side validation prevents bypass

### Test Case S3: Version Manipulation Attempt
**Steps**:
1. Attempt to send very high version number (version: 999)

**Expected**:
- Version mismatch detected
- Update fails
- Proper error returned

---

## Audit & Monitoring Tests

### Test Case A1: Audit Log Creation
**Steps**:
1. Perform successful edit
2. Query audit logs

**Expected**:
```sql
SELECT * FROM audit_logs
WHERE event_type = 'checkin.update'
  AND resource_id = '{checkin_id}'
  AND success = true;
```

**Verify**:
- Log entry exists
- Contains user_id
- Contains IP address
- Contains timestamp
- Contains request details

### Test Case A2: Failed Edit Audit
**Steps**:
1. Attempt denied edit (free user, old checkin)
2. Query audit logs

**Expected**:
```sql
SELECT * FROM audit_logs
WHERE event_type = 'checkin.update'
  AND success = false
  AND details->>'error' = 'edit_permission_denied';
```

**Verify**:
- Failed attempt logged
- Contains denial reason
- Contains user tier info

---

## Database State Tests

### Test Case D1: Edit History Preservation
**Steps**:
1. Create checkin with content "Original"
2. Update to "Version 2"
3. Update to "Version 3"

**Verify**:
```sql
-- Check current checkin
SELECT content, version FROM checkins WHERE id = '{checkin_id}';
-- Should show: "Version 3", version = 2

-- Check edit history
SELECT previous_content, edited_at
FROM edit_history
WHERE checkin_id = '{checkin_id}'
ORDER BY edited_at DESC;
-- Should show 2 entries: "Version 2", "Original"
```

### Test Case D2: Transaction Integrity
**Steps**:
1. Simulate database error during edit history insert
2. Verify checkin update also rolled back

**Expected**:
- Both operations succeed or both fail
- No partial updates
- Database remains consistent

---

## Regression Tests

After any changes to the premium edit feature, verify:

1. **Time Boundary Tests** still pass
   ```bash
   go test -v ./internal/checkin -run TestTimeUtils
   ```

2. **Repository Tests** still pass
   ```bash
   go test -v ./internal/checkin -run TestRepository
   ```

3. **Service Tests** still pass
   ```bash
   go test -v ./internal/checkin -run TestService
   ```

4. **API Tests** still pass
   ```bash
   go test -v ./tests/integration -tags=integration
   ```

---

## Test Data Setup

### SQL Scripts for Test Data

```sql
-- Create test users
INSERT INTO users (id, email, password_hash, tier, tier_expires_at) VALUES
  ('11111111-1111-1111-1111-111111111111', 'free@test.com', '$2a$10$...', 'free', NULL),
  ('22222222-2222-2222-2222-222222222222', 'premium@test.com', '$2a$10$...', 'premium', '2026-12-31'),
  ('33333333-3333-3333-3333-333333333333', 'expired@test.com', '$2a$10$...', 'premium', '2020-01-01');

-- Create test checkins
INSERT INTO checkins (id, user_id, content, checkin_time, duration_minutes, version) VALUES
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', '11111111-1111-1111-1111-111111111111',
   'Old checkin', NOW() - INTERVAL '5 hours', 30, 1),
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '22222222-2222-2222-2222-222222222222',
   'Recent checkin', NOW() - INTERVAL '30 minutes', 30, 1);
```

---

## Test Environment Setup

### Local Development Testing
```bash
# 1. Start test database
docker-compose -f docker-compose.test.yml up -d

# 2. Run migrations
make migrate-test

# 3. Seed test data
psql donelist_test < tests/testdata/seed.sql

# 4. Run tests
go test -v ./... -tags=integration

# 5. Cleanup
docker-compose -f docker-compose.test.yml down -v
```

### CI/CD Testing
```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
          POSTGRES_DB: donelist_test
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: make test-integration
```

---

## Test Coverage Goals

- **Unit Tests**: > 80% code coverage
- **Integration Tests**: All critical paths
- **API Tests**: All endpoints
- **E2E Tests**: Primary user workflows

---

## Test Automation

### Continuous Testing
```bash
# Watch mode for development
go test -v ./... -watch

# Coverage report
go test -v ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Nightly Full Test Suite
```bash
# Run all tests including slow integration tests
go test -v ./... -tags=integration -timeout=30m
```

---

## Known Issues & Edge Cases

1. **Clock Skew**: Tests may fail if system clock is significantly off
   - **Solution**: Use time.Now() mocking in tests

2. **Database Connection Pooling**: Concurrent tests may exhaust connections
   - **Solution**: Use t.Parallel() carefully, set max connections

3. **Timezone Issues**: Tests may behave differently in different timezones
   - **Solution**: All tests use UTC explicitly

---

## Test Maintenance

### When to Update Tests

1. **API Changes**: Update request/response schemas
2. **Business Logic Changes**: Update expected behavior
3. **New Features**: Add new test cases
4. **Bug Fixes**: Add regression tests

### Test Review Checklist

- [ ] All tests have clear names describing what they test
- [ ] Tests are independent (no shared state)
- [ ] Tests clean up after themselves
- [ ] Tests use meaningful assertions
- [ ] Tests document expected behavior
- [ ] Tests are fast (< 1s for unit tests)
- [ ] Tests are reliable (no flaky tests)

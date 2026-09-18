# Payment System Testing Guide

Complete guide for testing the payment and subscription system including integration tests, E2E tests, and manual testing procedures.

## Overview

This guide covers testing for:
- Stripe payment integration
- In-App Purchase (IAP) validation (iOS/Android)
- Subscription lifecycle management
- Webhook event processing
- Proration and refund logic
- Admin operations dashboard

## Quick Start

### Run All Payment Tests
```bash
cd server

# Run integration tests
go test ./tests/integration -v -run Payment

# Run with coverage
go test ./tests/integration -v -run Payment -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run specific test
go test ./tests/integration -v -run TestPaymentFlowE2E
```

## Test Categories

### 1. Integration Tests

**Location**: `/tests/integration/payment_integration_test.go`

**Coverage**:
- Complete payment flow (checkout → subscription)
- Stripe webhook scenarios
- IAP validation (iOS/Android)
- Subscription lifecycle
- Proration and refunds

**Run**:
```bash
go test ./tests/integration -v
```

**Key Tests**:
- `TestPaymentFlowE2E`: Full checkout flow
- `TestStripeWebhookScenarios`: All webhook events
- `TestIAPValidation`: Mobile receipt validation
- `TestSubscriptionLifecycle`: Create/renew/cancel/expire
- `TestProrationAndRefund`: Plan changes and refunds

### 2. Unit Tests

**Location**: Various `*_test.go` files

**Coverage**:
- Individual service methods
- Repository operations
- Helper functions
- Business logic validation

**Run**:
```bash
go test ./internal/payment -v
go test ./internal/subscription -v
go test ./internal/admin -v
```

### 3. E2E Tests

**Location**: `/tests/e2e/`

**Coverage**:
- Complete API flows
- Multi-step workflows
- User journeys

**Run**:
```bash
go test ./tests/e2e -v
```

## Test Scenarios

### Stripe Payment Flow

#### Scenario 1: Successful Subscription Creation
```
1. User creates checkout session
2. User completes Stripe checkout
3. Stripe sends checkout.session.completed webhook
4. Stripe sends customer.subscription.created webhook
5. System creates subscription record
6. User can access premium features
```

**Test**: `TestPaymentFlowE2E`

#### Scenario 2: Payment Failure
```
1. Subscription active
2. Renewal payment fails
3. Stripe sends invoice.payment_failed webhook
4. System marks subscription as past_due
5. Failed payment recorded
6. Alert triggered for admin
```

**Test**: `TestStripeWebhookScenarios/Invoice_Payment_Failed`

#### Scenario 3: Subscription Cancellation
```
1. User cancels subscription
2. System calls Stripe API to cancel
3. Stripe sends customer.subscription.updated webhook
4. cancel_at_period_end flag set
5. Subscription remains active until period end
6. At period end, subscription expires
```

**Test**: `TestSubscriptionLifecycle/Cancel_Subscription_at_Period_End`

### IAP Flow (iOS)

#### Scenario 1: New Purchase
```
1. User purchases in app
2. App sends receipt to server
3. Server validates with Apple
4. Server creates subscription
5. User gets premium access
```

**Test**: `TestIAPValidation/iOS_Receipt_Validation`

#### Scenario 2: Receipt Renewal
```
1. Subscription renews on App Store
2. App receives notification
3. App sends updated receipt
4. Server validates and updates subscription
5. Subscription period extended
```

**Test**: `TestIAPValidation/iOS_Receipt_Validation_Update`

### Plan Changes

#### Scenario 1: Upgrade
```
1. User upgrades from Premium to Enterprise
2. System calculates prorated charge
3. Stripe creates invoice for difference
4. invoice.payment_succeeded webhook received
5. Subscription plan updated
6. Prorated payment recorded
```

**Test**: `TestProrationAndRefund/Plan_Upgrade_with_Proration`

#### Scenario 2: Downgrade
```
1. User downgrades from Enterprise to Premium
2. System schedules downgrade at period end
3. At period end, plan changes
4. No prorated credit (policy decision)
5. Next billing cycle at lower price
```

## Mock Data

### Test Users
```go
testUser := &user.User{
    ID:          uuid.New(),
    Email:       "test@example.com",
    DisplayName: ptrString("Test User"),
}
```

### Test Subscriptions
```go
testSub := &subscription.Subscription{
    ID:                   uuid.New(),
    UserID:               testUser.ID,
    PlanID:               string(subscription.PlanPremium),
    Status:               subscription.StatusActive,
    CurrentPeriodStart:   time.Now(),
    CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
    StripeCustomerID:     ptrString("cus_test_123"),
    StripeSubscriptionID: ptrString("sub_test_123"),
}
```

### Mock Stripe Events
```go
func createMockStripeSubscription(customerID string, userID uuid.UUID) *stripe.Subscription {
    now := time.Now()
    return &stripe.Subscription{
        ID:                 "sub_test_" + uuid.New().String(),
        Status:             stripe.SubscriptionStatusActive,
        Customer:           &stripe.Customer{ID: customerID},
        CurrentPeriodStart: now.Unix(),
        CurrentPeriodEnd:   now.AddDate(0, 1, 0).Unix(),
        Metadata: map[string]string{
            "user_id": userID.String(),
            "plan_id": string(subscription.PlanPremium),
        },
    }
}
```

## Manual Testing

### 1. Stripe Test Mode

Use Stripe test mode for manual testing:

**Test Cards**:
- Success: `4242 4242 4242 4242`
- Decline: `4000 0000 0000 0002`
- Insufficient funds: `4000 0000 0000 9995`
- Expired card: `4000 0000 0000 0069`

**Test Webhooks**:
```bash
stripe listen --forward-to localhost:8080/api/v1/stripe/webhook
stripe trigger customer.subscription.created
stripe trigger invoice.payment_failed
```

### 2. IAP Testing

#### iOS Sandbox Testing
1. Create sandbox test user in App Store Connect
2. Sign out of production Apple ID on device
3. Make test purchase
4. Capture receipt data
5. Send to test server for validation

#### Android Testing
1. Add test account in Google Play Console
2. Create test license testers
3. Make test purchase
4. Capture purchase token
5. Validate with server

### 3. Admin Dashboard Testing

Test all admin operations manually:

```bash
# Get metrics
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/admin/subscriptions/metrics

# List failed payments
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/admin/payments/failed

# Manual cancellation
curl -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action":"cancel","reason":"Test cancellation"}' \
  http://localhost:8080/admin/subscriptions/{id}/manual-update
```

## Test Data Cleanup

### After Each Test
```go
defer testEnv.Cleanup()  // Automatic cleanup
```

### Manual Cleanup
```sql
-- Clean test subscriptions
DELETE FROM subscriptions WHERE stripe_customer_id LIKE 'cus_test_%';

-- Clean test payments
DELETE FROM payments WHERE stripe_payment_intent_id LIKE 'pi_test_%';

-- Clean test events
DELETE FROM subscription_events WHERE subscription_id IN (
    SELECT id FROM subscriptions WHERE stripe_customer_id LIKE 'cus_test_%'
);
```

## Coverage Goals

### Target Coverage by Component

| Component | Target | Current |
|-----------|--------|---------|
| Payment Service | 90% | - |
| Webhook Handler | 95% | - |
| IAP Service | 85% | - |
| Subscription Repository | 90% | - |
| Admin Operations | 80% | - |
| Payment Monitoring | 75% | - |

### Generate Coverage Report
```bash
# Run tests with coverage
go test ./internal/payment ./internal/subscription ./internal/admin \
  -coverprofile=coverage.out

# View HTML report
go tool cover -html=coverage.out

# View summary
go tool cover -func=coverage.out
```

## Debugging Tests

### Enable Verbose Logging
```go
logger, _ := zap.NewDevelopment()
```

### Print Test Data
```go
t.Logf("Subscription: %+v", sub)
t.Logf("Payment: %+v", payment)
```

### Debug Specific Test
```bash
go test ./tests/integration -v -run TestPaymentFlowE2E -count=1
```

### Debug with Delve
```bash
dlv test ./tests/integration -- -test.run TestPaymentFlowE2E
```

## Common Issues

### Issue: Webhook Signature Validation Fails
**Solution**: Use mock webhook secret in tests
```go
stripeClient := payment.NewStripeClient(
    "sk_test_mock_key",
    "whsec_test_mock_secret",
)
```

### Issue: Race Conditions
**Solution**: Add small delays for async operations
```go
time.Sleep(100 * time.Millisecond)
```

### Issue: Database Connection Pool Exhausted
**Solution**: Ensure proper cleanup
```go
defer db.Close()
```

### Issue: Tests Fail in CI but Pass Locally
**Solution**: Check for timing dependencies, use eventually assertions
```go
require.Eventually(t, func() bool {
    sub, _ := repo.GetSubscription(ctx, id)
    return sub.Status == subscription.StatusActive
}, 5*time.Second, 100*time.Millisecond)
```

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Payment Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:14
        env:
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run migrations
        run: make db-migrate-test

      - name: Run integration tests
        run: go test ./tests/integration -v -coverprofile=coverage.out

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
```

## Performance Testing

### Load Test Webhook Processing
```bash
# Using k6
k6 run scripts/load-test-webhooks.js

# Using Apache Bench
ab -n 1000 -c 10 -p webhook-payload.json \
  http://localhost:8080/api/v1/stripe/webhook
```

### Measure Webhook Latency
```go
func BenchmarkWebhookProcessing(b *testing.B) {
    testEnv := setupTestEnvironment(b)
    defer testEnv.Cleanup()

    event := createMockWebhookEvent()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        testEnv.WebhookHandler.HandleEvent(context.Background(), event)
    }
}
```

## Best Practices

### 1. Test Isolation
- Each test should be independent
- Use unique UUIDs for test data
- Clean up after each test

### 2. Mock External Services
- Don't make real Stripe API calls in tests
- Use mock Apple/Google receipt validation
- Mock time-dependent operations when appropriate

### 3. Test Real Behavior
- Use real database operations
- Test actual business logic
- Validate complete workflows

### 4. Assertions
```go
// Good: Specific assertions
assert.Equal(t, subscription.StatusActive, sub.Status)
assert.NotNil(t, sub.StripeCustomerID)

// Bad: Generic assertions
assert.NotNil(t, sub)
```

### 5. Error Cases
Test both happy paths and error scenarios:
- Payment failures
- Network errors
- Invalid input
- Race conditions
- Edge cases

## Documentation

- [Payment Integration Tests](./PAYMENT_INTEGRATION_TESTS.md)
- [Operations Dashboard](../admin/OPERATIONS_DASHBOARD.md)
- [Stripe Integration](../STRIPE_INTEGRATION.md)
- [IAP Validation](../IAP_VALIDATION.md)

## Support

For questions or issues with payment testing:
1. Check this guide
2. Review test code examples
3. Check Stripe documentation
4. Review webhook event types
5. Contact the payments team

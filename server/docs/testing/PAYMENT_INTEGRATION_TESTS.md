# Payment Integration Tests

Comprehensive integration tests for the payment and subscription system covering Stripe webhooks, IAP validation, subscription lifecycle, and proration logic.

## Overview

The payment integration tests validate the complete payment flow from checkout to subscription management, including:

- **E2E Payment Flow**: Complete checkout session to subscription creation
- **Stripe Webhooks**: All webhook event scenarios
- **IAP Validation**: iOS and Android receipt validation
- **Subscription Lifecycle**: Create, renew, cancel, expire flows
- **Proration & Refunds**: Plan changes and refund processing

## Test Structure

### Location
- Integration tests: `/server/tests/integration/payment_integration_test.go`
- Test setup: `/server/tests/integration/payment_test_setup.go`

### Test Environment Setup

The test environment includes:
- Test database with migrations
- Mock Stripe client
- Subscription repository
- Payment service
- Webhook handler
- Logger

```go
testEnv := setupTestEnvironment(t)
defer testEnv.Cleanup()
```

## Test Suites

### 1. Complete Payment Flow (TestPaymentFlowE2E)

Tests the entire payment workflow from customer creation to subscription activation.

**Workflow:**
1. Create Stripe customer
2. Create checkout session
3. Simulate successful checkout webhook
4. Verify subscription creation in database
5. Verify subscription event logging

**Assertions:**
- Customer created with correct metadata
- Checkout session has valid URL
- Subscription created with correct status
- Stripe IDs properly stored
- Event logged with correct type

### 2. Stripe Webhook Scenarios (TestStripeWebhookScenarios)

Tests all critical Stripe webhook events.

#### Subscription Updated - Plan Change
- **Event**: `customer.subscription.updated`
- **Scenario**: User changes subscription plan
- **Validation**: Plan ID updated, status remains active

#### Subscription Updated - Cancel at Period End
- **Event**: `customer.subscription.updated`
- **Scenario**: User schedules cancellation
- **Validation**: `cancel_at_period_end` flag set, canceled_at timestamp recorded

#### Subscription Deleted
- **Event**: `customer.subscription.deleted`
- **Scenario**: Subscription expires or is immediately canceled
- **Validation**: Status changed to canceled, canceled_at set

#### Invoice Payment Succeeded
- **Event**: `invoice.payment_succeeded`
- **Scenario**: Successful subscription renewal payment
- **Validation**: Payment record created with succeeded status, paid_at timestamp set

#### Invoice Payment Failed
- **Event**: `invoice.payment_failed`
- **Scenario**: Payment declined or failed
- **Validation**:
  - Subscription status changed to past_due
  - Failed payment record created
  - Failure reason recorded

### 3. IAP Validation (TestIAPValidation)

Tests In-App Purchase validation for mobile platforms.

#### iOS Receipt Validation - Active
- **Platform**: Apple App Store
- **Scenario**: Valid, active subscription receipt
- **Validation**:
  - Subscription created with correct platform metadata
  - Status set to active
  - Transaction IDs stored
  - Expiry date properly parsed

#### iOS Receipt Validation - Expired
- **Scenario**: Expired subscription receipt
- **Validation**:
  - Status set to expired
  - Auto-renewal disabled

#### iOS Receipt Validation - Canceled
- **Scenario**: Subscription with cancellation date
- **Validation**:
  - Status set to canceled
  - Cancellation date recorded

**Note**: Google Play validation requires additional implementation using Google Play Developer API.

### 4. Subscription Lifecycle (TestSubscriptionLifecycle)

Tests the complete subscription lifecycle from creation to expiration.

#### Create Active Subscription
- Creates new active subscription
- Validates all fields stored correctly
- Tests `IsActive()` helper method

#### Renew Subscription
- Simulates renewal payment succeeded
- Updates subscription period dates
- Validates continuous active status

#### Cancel Subscription at Period End
- Sets `cancel_at_period_end` flag
- Records cancellation timestamp
- Validates subscription remains active until period end

#### Expire Subscription
- Processes subscription deletion event
- Updates status to canceled
- Validates `IsActive()` returns false

### 5. Proration and Refunds (TestProrationAndRefund)

Tests plan changes with proration and refund processing.

#### Plan Upgrade with Proration
- **Scenario**: User upgrades mid-billing cycle
- **Workflow**:
  1. Create active subscription on lower plan
  2. Process prorated payment invoice
  3. Update subscription to higher plan
- **Validation**:
  - Proration payment recorded
  - Plan updated correctly
  - Payment amount reflects prorated charge

#### Refund Processing
- **Scenario**: Process full or partial refund
- **Workflow**:
  1. Create succeeded payment
  2. Process refund
  3. Update payment status
- **Validation**:
  - Payment status changed to refunded
  - Refund amount recorded

## Running the Tests

### Run all integration tests
```bash
cd server
go test ./tests/integration -v
```

### Run specific test
```bash
go test ./tests/integration -v -run TestPaymentFlowE2E
```

### Run with race detection
```bash
go test ./tests/integration -v -race
```

### Skip integration tests (short mode)
```bash
go test ./tests/integration -v -short
```

## Test Data

### Mock Stripe Objects

The tests use helper functions to create mock Stripe objects:

```go
func createMockStripeSubscription(customerID string, userID uuid.UUID) *stripe.Subscription
func marshalToJSON(t *testing.T, v interface{}) json.RawMessage
```

### Test User IDs
- Generated using `uuid.New()` for each test
- Ensures test isolation

### Test Timestamps
- Use `time.Now()` for current operations
- Use relative dates (e.g., `time.Now().AddDate(0, 1, 0)`) for future/past dates

## Database Setup

Tests use a clean database for each run:
- Migrations applied automatically
- Transactions rolled back after each test (if using transaction-based tests)
- Full cleanup on test completion

## Mocking Strategy

### What's Mocked
- Stripe API calls (using test Stripe client)
- External HTTP requests
- Time-dependent operations (where appropriate)

### What's Real
- Database operations
- Repository layer
- Service layer business logic
- Webhook processing logic

## Coverage Goals

Target coverage for payment/subscription code:
- **Payment Service**: >90%
- **Webhook Handler**: >95%
- **IAP Service**: >85%
- **Subscription Repository**: >90%

## Common Test Patterns

### Creating Test Subscriptions
```go
sub := &subscription.Subscription{
    ID:                   uuid.New(),
    UserID:               userID,
    PlanID:               string(subscription.PlanPremium),
    Status:               subscription.StatusActive,
    CurrentPeriodStart:   time.Now(),
    CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
    CancelAtPeriodEnd:    false,
    StripeCustomerID:     &customerID,
    StripeSubscriptionID: &subscriptionID,
    CreatedAt:            time.Now(),
    UpdatedAt:            time.Now(),
}
err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
```

### Simulating Webhooks
```go
err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
    ID:   "evt_test_" + uuid.New().String(),
    Type: "customer.subscription.created",
    Data: &stripe.EventData{
        Raw: marshalToJSON(t, mockSubscription),
    },
})
```

### Verifying Database State
```go
sub, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
require.NoError(t, err)
assert.Equal(t, subscription.StatusActive, sub.Status)
```

## Troubleshooting

### Tests Failing Due to Timing
- Add small delays for async operations: `time.Sleep(100 * time.Millisecond)`
- Use eventually assertions for async behavior

### Database Connection Issues
- Ensure test database is accessible
- Check database migrations are up to date
- Verify test database credentials

### Webhook Signature Validation
- Tests use mock webhook secrets
- Real Stripe signature validation is bypassed in tests

## Future Enhancements

1. **Load Testing**: Add concurrent webhook processing tests
2. **Chaos Testing**: Simulate network failures, database errors
3. **Performance Benchmarks**: Track webhook processing times
4. **Additional Scenarios**:
   - Trial period expiration
   - Payment method updates
   - Multiple subscriptions per user
   - Subscription pausing/resuming

## Related Documentation

- [Payment System Architecture](../PAYMENT_SYSTEM.md)
- [Stripe Integration Guide](../STRIPE_INTEGRATION.md)
- [IAP Validation Guide](../IAP_VALIDATION.md)
- [Admin Operations Dashboard](../admin/OPERATIONS_DASHBOARD.md)

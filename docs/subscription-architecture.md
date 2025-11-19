# Subscription and Payment System Architecture

## Overview
This document outlines the architecture for the Donelist subscription and payment system, supporting Stripe web payments and mobile In-App Purchases (IAP) for iOS and Android.

## System Components

### 1. Data Models

#### Subscription Tiers
- **Free**: Basic features, 2-hour edit window
- **Premium**: Extended features, unlimited edit history
- **Enterprise**: All features, team collaboration (future)

#### Subscription Status State Machine
```
pending -> active -> past_due -> canceled
         -> active -> trial -> active
         -> active -> incomplete -> active/canceled
```

States:
- `pending`: Subscription created but not yet activated
- `active`: Active subscription with valid payment
- `trial`: In trial period
- `past_due`: Payment failed, grace period active
- `incomplete`: Initial payment incomplete
- `canceled`: User canceled, ends at period end
- `expired`: Subscription period ended

### 2. Database Schema

Already implemented in migrations:
- `subscriptions`: Core subscription data with Stripe IDs
- `payments`: Payment transaction history
- `subscription_events`: Audit trail for all subscription changes
- `checkin_history`: Premium feature tracking

### 3. Payment Provider Abstraction

```go
type PaymentProvider interface {
    // Customer Management
    CreateCustomer(ctx context.Context, user *User) (*Customer, error)
    GetCustomer(ctx context.Context, customerID string) (*Customer, error)
    UpdateCustomer(ctx context.Context, customerID string, params *CustomerParams) error

    // Subscription Management
    CreateSubscription(ctx context.Context, params *SubscriptionParams) (*Subscription, error)
    UpdateSubscription(ctx context.Context, subID string, params *SubscriptionParams) error
    CancelSubscription(ctx context.Context, subID string, cancelAtPeriodEnd bool) error

    // Checkout & Billing
    CreateCheckoutSession(ctx context.Context, params *CheckoutParams) (*CheckoutSession, error)
    CreateBillingPortalSession(ctx context.Context, customerID string, returnURL string) (*BillingPortalSession, error)

    // Payment Intents
    GetPaymentIntent(ctx context.Context, paymentIntentID string) (*PaymentIntent, error)

    // Webhooks
    VerifyWebhookSignature(payload []byte, signature string, secret string) error
    ParseWebhookEvent(payload []byte) (*Event, error)
}
```

### 4. Stripe Integration

#### Products and Prices
- Stripe Dashboard: Create products and prices
- Environment variables:
  - `STRIPE_SECRET_KEY`: API secret key
  - `STRIPE_WEBHOOK_SECRET`: Webhook signing secret
  - `STRIPE_PRICE_ID_PREMIUM_MONTHLY`: Premium monthly price ID
  - `STRIPE_PRICE_ID_PREMIUM_YEARLY`: Premium yearly price ID

#### Key Stripe Webhooks to Handle
1. `customer.subscription.created`
2. `customer.subscription.updated`
3. `customer.subscription.deleted`
4. `customer.subscription.trial_will_end`
5. `invoice.payment_succeeded`
6. `invoice.payment_failed`
7. `invoice.finalized`
8. `payment_intent.succeeded`
9. `payment_intent.payment_failed`
10. `charge.refunded`

### 5. Subscription Service

Core service handling all subscription business logic:

```go
type SubscriptionService interface {
    // Subscription Lifecycle
    CreateSubscription(ctx context.Context, userID uuid.UUID, planID string, params *CreateSubscriptionParams) (*Subscription, error)
    GetSubscription(ctx context.Context, subscriptionID uuid.UUID) (*Subscription, error)
    GetUserSubscription(ctx context.Context, userID uuid.UUID) (*Subscription, error)

    // Plan Changes
    UpgradeSubscription(ctx context.Context, userID uuid.UUID, newPlanID string) (*Subscription, error)
    DowngradeSubscription(ctx context.Context, userID uuid.UUID, newPlanID string) (*Subscription, error)
    CancelSubscription(ctx context.Context, userID uuid.UUID, cancelAtPeriodEnd bool) (*Subscription, error)
    ReactivateSubscription(ctx context.Context, userID uuid.UUID) (*Subscription, error)

    // State Management
    UpdateSubscriptionStatus(ctx context.Context, subscriptionID uuid.UUID, status string, reason string) error
    HandleExpiredSubscriptions(ctx context.Context) error
    HandlePastDueSubscriptions(ctx context.Context) error

    // Webhooks
    HandleStripeWebhook(ctx context.Context, event *stripe.Event) error

    // IAP Validation
    ValidateAppleReceipt(ctx context.Context, userID uuid.UUID, receiptData string) (*Subscription, error)
    ValidateGooglePurchase(ctx context.Context, userID uuid.UUID, purchaseToken string, productID string) (*Subscription, error)
}
```

### 6. Webhook Handler

Secure webhook endpoint at `/api/v1/webhooks/stripe`:
- Verify Stripe signature using `STRIPE_WEBHOOK_SECRET`
- Parse event type
- Delegate to appropriate handler
- Idempotent processing (check event ID)
- Error handling with retries (Stripe retries automatically)

### 7. API Endpoints

#### Public Endpoints
- `POST /api/v1/subscription/checkout` - Create Stripe checkout session
- `POST /api/v1/webhooks/stripe` - Stripe webhook receiver (public, verified)

#### Protected Endpoints (require auth)
- `GET /api/v1/subscription` - Get current user subscription
- `POST /api/v1/subscription/upgrade` - Upgrade to premium
- `POST /api/v1/subscription/downgrade` - Downgrade plan
- `POST /api/v1/subscription/cancel` - Cancel subscription
- `POST /api/v1/subscription/reactivate` - Reactivate canceled subscription
- `GET /api/v1/subscription/billing-portal` - Get Stripe billing portal URL
- `GET /api/v1/subscription/invoices` - List user invoices
- `POST /api/v1/subscription/iap/apple` - Validate Apple IAP receipt
- `POST /api/v1/subscription/iap/google` - Validate Google Play purchase

#### Admin Endpoints (require admin role - future)
- `GET /api/v1/admin/subscriptions` - List all subscriptions
- `GET /api/v1/admin/subscriptions/:id` - Get subscription details
- `POST /api/v1/admin/subscriptions/:id/refund` - Issue refund

### 8. Mobile IAP Integration

#### iOS (Apple StoreKit)
- Validate receipts with Apple's servers
- Handle subscription status updates
- Server-to-server notifications for renewals/cancellations

#### Android (Google Play Billing)
- Validate purchase tokens with Google Play Developer API
- Handle Real-time Developer Notifications (RTDN)
- Check subscription status

### 9. Security Considerations

1. **Webhook Security**
   - Always verify Stripe webhook signatures
   - Use HTTPS only
   - Store webhook secrets securely
   - Implement replay attack prevention

2. **Payment Data**
   - Never store credit card details
   - Use Stripe's PCI-compliant infrastructure
   - Only store Stripe customer/subscription IDs

3. **IAP Validation**
   - Always validate receipts server-side
   - Never trust client-side validation
   - Implement receipt replay prevention

4. **Access Control**
   - Check subscription status on every premium feature access
   - Implement grace periods for payment failures
   - Cache subscription status in Redis with TTL

### 10. Subscription State Machine

```go
type SubscriptionStatus string

const (
    StatusPending    SubscriptionStatus = "pending"
    StatusActive     SubscriptionStatus = "active"
    StatusTrial      SubscriptionStatus = "trial"
    StatusPastDue    SubscriptionStatus = "past_due"
    StatusIncomplete SubscriptionStatus = "incomplete"
    StatusCanceled   SubscriptionStatus = "canceled"
    StatusExpired    SubscriptionStatus = "expired"
)

// Valid state transitions
var validTransitions = map[SubscriptionStatus][]SubscriptionStatus{
    StatusPending:    {StatusActive, StatusTrial, StatusIncomplete, StatusCanceled},
    StatusActive:     {StatusPastDue, StatusCanceled, StatusExpired},
    StatusTrial:      {StatusActive, StatusCanceled, StatusExpired},
    StatusPastDue:    {StatusActive, StatusCanceled, StatusExpired},
    StatusIncomplete: {StatusActive, StatusCanceled},
    StatusCanceled:   {StatusExpired, StatusActive}, // Can reactivate
    StatusExpired:    {StatusActive}, // Can subscribe again
}
```

### 11. Proration Logic

When upgrading/downgrading:
- Stripe handles proration automatically
- Credit unused time from current plan
- Charge difference for new plan
- Update `current_period_end` appropriately

### 12. Failed Payment Handling

1. **First Failure**: Send email, mark as `past_due`, grace period starts
2. **Grace Period**: 7 days to update payment method
3. **After Grace**: Downgrade to free tier, mark as `expired`
4. **Retry Logic**: Stripe automatically retries failed payments

### 13. Scheduled Jobs

Cron jobs to run periodically:
1. **Expire Subscriptions**: Check `current_period_end` and expire ended subscriptions
2. **Grace Period Checks**: Notify users in grace period
3. **Trial End Notifications**: Remind users trial ending soon
4. **Sync Stripe Status**: Periodically sync with Stripe for consistency

### 14. Monitoring and Logging

Track key metrics:
- New subscriptions
- Cancellations and reasons
- Failed payments
- Churn rate
- Monthly Recurring Revenue (MRR)
- Customer Lifetime Value (LTV)

### 15. Testing Strategy

1. **Unit Tests**: Test state machine, business logic
2. **Integration Tests**: Test Stripe API integration (use test mode)
3. **Webhook Tests**: Mock Stripe webhook events
4. **IAP Tests**: Mock Apple/Google validation
5. **E2E Tests**: Full subscription flow

## Implementation Phases

### Phase 1: Core Infrastructure (Subtasks 1-2)
- Data models and migrations
- Stripe service integration
- Basic subscription CRUD

### Phase 2: Stripe Integration (Subtasks 3-4)
- Checkout flow
- Webhook handlers
- Payment processing

### Phase 3: Subscription Logic (Subtasks 5-6)
- State machine
- Lifecycle management
- Proration and cancellation

### Phase 4: Mobile IAP (Subtask 7)
- Apple receipt validation
- Google Play validation
- Unified subscription interface

### Phase 5: Premium Features (Subtask 8)
- Feature flags
- Access control middleware
- Premium feature APIs

### Phase 6: Operations (Subtasks 9-10)
- Notification system
- Audit logging
- Scheduled jobs

### Phase 7: Testing & Admin (Subtasks 11-12)
- Comprehensive test suite
- Admin dashboard
- Monitoring tools

## File Structure

```
server/
├── internal/
│   ├── subscription/
│   │   ├── models.go              # Subscription, Payment, Plan models
│   │   ├── repository.go          # Database operations
│   │   ├── service.go             # Business logic
│   │   ├── state_machine.go       # State transitions
│   │   ├── stripe_provider.go     # Stripe implementation
│   │   ├── iap_apple.go          # Apple IAP validation
│   │   ├── iap_google.go         # Google Play validation
│   │   └── scheduler.go           # Cron jobs
│   ├── api/
│   │   └── handlers/
│   │       ├── subscription_handler.go  # API handlers
│   │       └── webhook_handler.go       # Stripe webhook handler
│   └── middleware/
│       └── subscription.go        # Already exists, may need updates
└── migrations/
    └── 000005_payments_subscriptions.up.sql  # Already exists
```

## Environment Variables

Add to `.env`:
```
# Stripe Configuration
STRIPE_SECRET_KEY=sk_test_...
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
STRIPE_PRICE_ID_PREMIUM_MONTHLY=price_...
STRIPE_PRICE_ID_PREMIUM_YEARLY=price_...

# Apple IAP
APPLE_SHARED_SECRET=...
APPLE_SANDBOX_MODE=true

# Google Play
GOOGLE_SERVICE_ACCOUNT_JSON=...

# Subscription Settings
SUBSCRIPTION_GRACE_PERIOD_DAYS=7
SUBSCRIPTION_TRIAL_DAYS=14
```

## Next Steps

1. Start with Subtask 10.1: Review and consolidate database migrations
2. Implement Subtask 10.2: Stripe service integration
3. Build Subtask 10.3: Subscription service with state machine
4. Add Subtask 10.4: Webhook handlers
5. Create Subtask 10.5: API endpoints
6. Implement Subtask 10.7: IAP validation
7. Complete testing and monitoring

---

Last Updated: 2025-01-13

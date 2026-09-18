# Subscription and Payment System Implementation

## Overview

The subscription and payment system is built on Stripe integration with full webhook support, subscription lifecycle management, and billing portal functionality.

## Architecture

### Core Components

```
├── internal/subscription/       # Subscription domain models
│   ├── models.go               # Subscription, Payment, Plan models
│   ├── repository.go           # Database operations
│   └── state_machine.go        # Subscription state transitions
├── internal/payment/            # Stripe integration
│   ├── init.go                 # Payment manager initialization
│   ├── service.go              # Stripe API operations
│   ├── stripe_client.go        # Stripe SDK wrapper
│   ├── webhook_handler.go      # Webhook event processing
│   └── product_manager.go      # Product sync with Stripe
├── internal/premium/            # Premium feature gating
│   ├── service.go              # Tier management and feature checks
│   ├── tiers.go                # Tier definitions
│   ├── features.go             # Feature flags per tier
│   └── middleware.go           # Request-level feature gating
└── internal/api/                # HTTP handlers
    └── payment_handler.go      # Payment API endpoints
```

## Subscription Tiers

### Free Tier
- **Plan ID**: `free`
- **Price**: $0
- **Features**:
  - Unlimited check-ins
  - 2-hour edit window
  - Basic categories and tags
  - 7-day history

### Premium Tier
- **Plan ID**: `premium`
- **Monthly Price**: $9.99
- **Yearly Price**: $99.90 (2 months free)
- **Trial**: 14 days
- **Features**:
  - All Free features
  - Unlimited edit history
  - Edit old check-ins anytime
  - Advanced analytics
  - Priority support
  - Export data

### Enterprise Tier (Coming Soon)
- **Plan ID**: `enterprise`
- **Monthly Price**: $29.99
- **Yearly Price**: $299.90
- **Trial**: 30 days
- **Features**:
  - All Premium features
  - Team collaboration
  - Custom integrations
  - SSO authentication
  - Dedicated support

## API Endpoints

### Checkout and Billing

#### POST /api/v1/checkout/session
Create a Stripe Checkout session for subscription.

**Request:**
```json
{
  "plan_id": "premium",
  "billing_interval": "month",
  "success_url": "https://app.example.com/success",
  "cancel_url": "https://app.example.com/cancel"
}
```

**Response:**
```json
{
  "session_id": "cs_test_...",
  "url": "https://checkout.stripe.com/..."
}
```

#### POST /api/v1/billing-portal
Create a billing portal session for subscription management.

**Request:**
```json
{
  "return_url": "https://app.example.com/settings"
}
```

**Response:**
```json
{
  "url": "https://billing.stripe.com/..."
}
```

### Subscription Management

#### GET /api/v1/subscription/status
Get current subscription status.

**Response:**
```json
{
  "has_subscription": true,
  "status": "active",
  "plan_id": "premium",
  "current_period_start": "2025-01-01T00:00:00Z",
  "current_period_end": "2025-02-01T00:00:00Z",
  "cancel_at_period_end": false,
  "is_active": true,
  "is_trial": false,
  "is_past_due": false,
  "days_until_expiry": 30,
  "trial_start": null,
  "trial_end": null,
  "recent_payments": [...]
}
```

#### POST /api/v1/subscription/cancel
Cancel subscription (can be immediate or at period end).

**Request:**
```json
{
  "immediately": false
}
```

**Response:**
```json
{
  "status": "active",
  "cancel_at_period_end": true,
  "current_period_end": "2025-02-01T00:00:00Z"
}
```

#### POST /api/v1/subscription/reactivate
Reactivate a subscription scheduled for cancellation.

**Response:**
```json
{
  "status": "active",
  "cancel_at_period_end": false,
  "current_period_end": "2025-02-01T00:00:00Z"
}
```

### Payment Methods

#### GET /api/v1/payment-methods
List customer's payment methods.

**Response:**
```json
{
  "payment_methods": [
    {
      "id": "pm_...",
      "type": "card",
      "card": {
        "last4": "4242",
        "brand": "visa",
        "exp_month": 12,
        "exp_year": 2025
      }
    }
  ]
}
```

#### POST /api/v1/payment-method/attach
Attach a payment method to customer.

**Request:**
```json
{
  "payment_method_id": "pm_...",
  "set_as_default": true
}
```

**Response:**
```json
{
  "payment_method_id": "pm_...",
  "type": "card",
  "card": {
    "last4": "4242",
    "brand": "visa",
    "exp_month": 12,
    "exp_year": 2025
  }
}
```

#### DELETE /api/v1/payment-method/:id
Detach a payment method.

**Response:**
```json
{
  "message": "Payment method detached successfully"
}
```

### Webhook Endpoint

#### POST /api/v1/stripe/webhook
Receive Stripe webhook events (signature verified).

**Supported Events:**
- `customer.subscription.created`
- `customer.subscription.updated`
- `customer.subscription.deleted`
- `customer.subscription.trial_will_end`
- `invoice.payment_succeeded`
- `invoice.payment_failed`
- `payment_intent.succeeded`
- `payment_intent.payment_failed`
- `payment_method.attached`
- `checkout.session.completed`

## Subscription Lifecycle

### State Machine

```
pending -> active (payment succeeds)
pending -> incomplete (payment fails)
active -> trial (trial period active)
trial -> active (trial ends, payment succeeds)
trial -> past_due (trial ends, payment fails)
active -> past_due (renewal payment fails)
past_due -> active (retry payment succeeds)
past_due -> canceled (grace period expires)
active -> canceled (user cancels, period ends)
canceled -> expired (cleanup)
```

### Grace Period

When a subscription enters `past_due` status (payment failed), users have a **7-day grace period** where premium features remain accessible. After the grace period:
- Subscription moves to `canceled` status
- Premium features are restricted
- User can reactivate by updating payment method

## Feature Gating

### Premium Service

The `premium.Service` provides feature checking and usage limit enforcement:

```go
// Check if user has access to a feature
err := premiumService.CheckFeatureAccess(ctx, userID, premium.FeatureAdvancedAnalytics)

// Check if user meets minimum tier
err := premiumService.CheckTierAccess(ctx, userID, premium.TierPremium)

// Check usage limits
usageInfo, err := premiumService.CheckUsageLimit(ctx, userID, "exports_per_month")
```

### Feature Flags per Tier

```go
// Free tier
- FeatureBasicCheckins
- FeatureBasicCategories
- Feature2HourEditWindow
- Feature7DayHistory

// Premium tier (includes all Free features)
- FeatureUnlimitedEditHistory
- FeatureAdvancedAnalytics
- FeaturePrioritySupport
- FeatureDataExport
- FeatureCustomWebhooks
- FeatureAPIAccess

// Enterprise tier (includes all Premium features)
- FeatureTeamCollaboration
- FeatureCustomIntegrations
- FeatureSSO
- FeatureDedicatedSupport
- FeatureCustomBranding
```

### Usage Limits

| Limit Type | Free | Premium | Enterprise |
|------------|------|---------|------------|
| `checkins_per_day` | Unlimited | Unlimited | Unlimited |
| `checkins_per_month` | Unlimited | Unlimited | Unlimited |
| `categories_count` | 10 | 50 | Unlimited |
| `webhooks_count` | 0 | 5 | Unlimited |
| `api_tokens_count` | 0 | 3 | Unlimited |
| `exports_per_month` | 1 | 50 | Unlimited |
| `api_calls_per_hour` | 60 | 600 | Unlimited |

## Database Schema

### subscriptions table
```sql
CREATE TABLE subscriptions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id),
  plan_id VARCHAR(50) NOT NULL,
  status VARCHAR(20) NOT NULL,
  current_period_start TIMESTAMP NOT NULL,
  current_period_end TIMESTAMP NOT NULL,
  cancel_at_period_end BOOLEAN DEFAULT FALSE,
  canceled_at TIMESTAMP,
  trial_start TIMESTAMP,
  trial_end TIMESTAMP,
  stripe_customer_id VARCHAR(255),
  stripe_subscription_id VARCHAR(255),
  metadata JSONB,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);
```

### payments table
```sql
CREATE TABLE payments (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id),
  subscription_id UUID REFERENCES subscriptions(id),
  amount INTEGER NOT NULL,
  currency VARCHAR(3) NOT NULL,
  status VARCHAR(20) NOT NULL,
  payment_method VARCHAR(50),
  stripe_payment_intent_id VARCHAR(255),
  stripe_charge_id VARCHAR(255),
  description TEXT,
  metadata JSONB,
  paid_at TIMESTAMP,
  failed_at TIMESTAMP,
  failure_reason TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);
```

### subscription_events table
```sql
CREATE TABLE subscription_events (
  id UUID PRIMARY KEY,
  subscription_id UUID NOT NULL REFERENCES subscriptions(id),
  event_type VARCHAR(50) NOT NULL,
  previous_value JSONB,
  new_value JSONB,
  reason TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);
```

## Configuration

### Environment Variables

```bash
# Stripe Configuration
STRIPE_SECRET_KEY=sk_test_...           # Stripe secret key
STRIPE_WEBHOOK_SECRET=whsec_...         # Webhook signing secret
STRIPE_SYNC_PRODUCTS=true                # Sync products on startup

# Database
DATABASE_URL=postgresql://...            # PostgreSQL connection string

# Application
APP_ENV=development                      # development, staging, production
```

### Stripe Setup

1. **Create Products in Stripe Dashboard** (or use product sync)
   - Premium (monthly & yearly prices)
   - Enterprise (monthly & yearly prices)

2. **Configure Webhook Endpoint**
   - URL: `https://your-domain.com/api/v1/stripe/webhook`
   - Events: Select all subscription and payment events
   - Copy webhook signing secret to `STRIPE_WEBHOOK_SECRET`

3. **Enable Customer Portal**
   - Configure in Stripe Dashboard > Settings > Customer Portal
   - Allow customers to update payment methods
   - Allow customers to cancel subscriptions

## Testing

### Test Cards

Use Stripe test cards for development:
- Success: `4242 4242 4242 4242`
- Decline: `4000 0000 0000 0002`
- Insufficient funds: `4000 0000 0000 9995`
- Requires authentication: `4000 0025 0000 3155`

### Webhook Testing

Use Stripe CLI for local webhook testing:
```bash
stripe listen --forward-to localhost:8080/api/v1/stripe/webhook
stripe trigger customer.subscription.created
stripe trigger invoice.payment_failed
```

### Manual Testing Checklist

- [ ] Create checkout session
- [ ] Complete payment flow
- [ ] Verify subscription created in database
- [ ] Test billing portal access
- [ ] Update payment method
- [ ] Cancel subscription (at period end)
- [ ] Reactivate subscription
- [ ] Test immediate cancellation
- [ ] Trigger payment failure
- [ ] Verify grace period behavior
- [ ] Test trial period flow
- [ ] Verify feature gating
- [ ] Test usage limits

## Monitoring and Observability

### Metrics to Track

- Subscription churn rate
- Trial conversion rate
- Payment success/failure rate
- Average revenue per user (ARPU)
- Customer lifetime value (LTV)
- Grace period conversions

### Logging

All subscription and payment events are logged with structured logging:
```go
logger.Info("Created checkout session",
  zap.String("session_id", session.ID),
  zap.String("user_id", userID.String()),
  zap.String("plan_id", planID),
)
```

### Audit Trail

All subscription changes are logged to `subscription_events` table for audit purposes.

## Security Considerations

1. **Webhook Signature Verification**: All webhook payloads are verified using Stripe's signature
2. **Authentication**: All API endpoints require valid JWT authentication
3. **Authorization**: Users can only access their own subscription data
4. **PCI Compliance**: No credit card data stored; handled entirely by Stripe
5. **HTTPS Only**: All Stripe communication uses HTTPS
6. **Environment Separation**: Test and live modes strictly separated

## Error Handling

### Common Errors

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `payment_method_required` | No payment method attached | Prompt user to add payment method |
| `card_declined` | Card declined by issuer | Try different card |
| `insufficient_funds` | Insufficient funds | Try different card or wait |
| `subscription_not_found` | No active subscription | Create new subscription |
| `already_subscribed` | User already has active subscription | Manage existing subscription |
| `past_due` | Payment failed, in grace period | Update payment method |

### Retry Logic

- Failed payments are automatically retried by Stripe
- Webhooks are retried with exponential backoff
- Application ensures idempotency for all webhook handlers

## Future Enhancements

### Phase 2 (Subtasks 10.3-10.9)
- [ ] Prorat ion, refunds, and cancellation logic (10.4)
- [ ] iOS/Android In-App Purchase validation (10.5)
- [ ] Enhanced feature flag middleware (10.6)
- [ ] Subscription expiry notifications (10.7)
- [ ] End-to-end integration tests (10.8)
- [ ] Operations dashboard and monitoring (10.9)

### Phase 3 (Future)
- [ ] Multi-currency support
- [ ] Family/team subscriptions
- [ ] Promotional codes and discounts
- [ ] Referral program
- [ ] Usage-based billing
- [ ] Subscription pause/resume
- [ ] Dunning management

## Support

For implementation questions or issues:
- Review Stripe documentation: https://stripe.com/docs
- Check webhook logs in Stripe Dashboard
- Review subscription_events table for audit trail
- Contact Stripe support for payment-specific issues

---

**Last Updated**: November 24, 2025
**Status**: Subtask 10.2 Complete - Checkout Session and Billing Portal API Implemented

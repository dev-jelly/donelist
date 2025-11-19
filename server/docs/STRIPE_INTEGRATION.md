# Stripe Integration Documentation

## Overview

This document describes the Stripe payment integration for the Donelist subscription system. The integration supports subscription management, payment processing, and webhook handling.

## Setup

### 1. Obtain Stripe API Keys

1. Create a Stripe account at [stripe.com](https://stripe.com)
2. Navigate to the [API Keys page](https://dashboard.stripe.com/apikeys)
3. Copy your test keys for development:
   - Secret Key (starts with `sk_test_`)
   - Publishable Key (starts with `pk_test_`)

### 2. Configure Environment Variables

Update your `.env` file with your Stripe keys:

```bash
# Stripe Configuration
STRIPE_SECRET_KEY=sk_test_your_actual_test_key_here
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret_here
STRIPE_SYNC_PRODUCTS_ON_STARTUP=true  # For development
```

### 3. Set Up Webhook Endpoint

1. Go to [Stripe Webhooks](https://dashboard.stripe.com/webhooks)
2. Click "Add endpoint"
3. Enter your webhook URL: `https://your-domain.com/api/v1/payments/stripe/webhook`
4. Select the following events to listen for:
   - `customer.subscription.created`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `customer.subscription.trial_will_end`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
   - `payment_intent.succeeded`
   - `payment_intent.payment_failed`
   - `checkout.session.completed`
5. Copy the webhook signing secret and add it to your `.env` file

## Architecture

### Package Structure

```
internal/payment/
├── stripe_client.go      # Stripe client initialization and configuration
├── service.go            # Core payment service with Stripe operations
├── webhook_handler.go    # Webhook event processing
├── product_manager.go    # Product and price management
├── init.go              # Package initialization and manager
└── stripe_client_test.go # Unit tests
```

### Key Components

#### 1. StripeClient
- Initializes the Stripe SDK with proper configuration
- Manages API keys and webhook secrets
- Provides logging integration
- Detects test vs. live mode automatically

#### 2. PaymentService
- Creates and manages customers
- Handles checkout sessions for subscriptions
- Manages subscription lifecycle (create, update, cancel)
- Processes payment intents for one-time payments
- Manages payment methods

#### 3. WebhookHandler
- Verifies webhook signatures for security
- Processes various Stripe events
- Updates local subscription records
- Triggers notifications and side effects

#### 4. ProductManager
- Syncs local plan definitions with Stripe products
- Creates and updates Stripe prices
- Manages product catalog

## Integration with Existing Models

The Stripe integration works with the existing subscription models in `internal/subscription/models.go`:

- `Subscription` model has `StripeCustomerID` and `StripeSubscriptionID` fields
- `Payment` model has `StripePaymentIntentID` and `StripeChargeID` fields
- Status mappings between Stripe and internal statuses are handled automatically

## API Endpoints

### Public Endpoints

#### POST `/api/v1/payments/stripe/webhook`
Receives and processes Stripe webhook events. No authentication required but signature verification is mandatory.

### Protected Endpoints (Require Authentication)

#### POST `/api/v1/payments/checkout/session`
Creates a Stripe Checkout session for subscription signup.

**Request Body:**
```json
{
  "plan_id": "premium",
  "billing_interval": "month",
  "success_url": "https://app.donelist.com/subscription/success",
  "cancel_url": "https://app.donelist.com/subscription/cancel"
}
```

**Response:**
```json
{
  "session_id": "cs_test_...",
  "url": "https://checkout.stripe.com/c/pay/cs_test_..."
}
```

#### POST `/api/v1/payments/subscription/cancel`
Cancels the user's subscription.

**Request Body:**
```json
{
  "immediately": false  // If true, cancels immediately. If false, cancels at period end
}
```

#### GET `/api/v1/payments/subscription/status`
Returns the current subscription status for the authenticated user.

**Response:**
```json
{
  "has_subscription": true,
  "status": "active",
  "plan_id": "premium",
  "current_period_end": "2024-12-01T00:00:00Z",
  "cancel_at_period_end": false,
  "is_active": true,
  "is_trial": false,
  "days_until_expiry": 15
}
```

## Testing

### Unit Tests

Run the payment package tests:

```bash
go test ./internal/payment/... -v
```

### Integration Testing with Stripe Test Mode

1. Set up test API keys in your `.env` file
2. Use Stripe's test card numbers:
   - Success: `4242 4242 4242 4242`
   - Decline: `4000 0000 0000 0002`
   - Requires authentication: `4000 0025 0000 3155`

### Testing Webhooks Locally

Use Stripe CLI to forward webhooks to your local server:

1. Install Stripe CLI:
   ```bash
   brew install stripe/stripe-cli/stripe
   ```

2. Login to your Stripe account:
   ```bash
   stripe login
   ```

3. Forward webhooks to your local server:
   ```bash
   stripe listen --forward-to localhost:8080/api/v1/payments/stripe/webhook
   ```

4. The CLI will display your webhook signing secret. Update your `.env` file with this secret.

5. Trigger test events:
   ```bash
   stripe trigger customer.subscription.created
   ```

## Subscription Plans

The application defines three subscription plans in `internal/subscription/models.go`:

1. **Free Plan**
   - Price: $0
   - Basic features
   - No trial period

2. **Premium Plan**
   - Monthly: $9.99
   - Yearly: $99.90 (2 months free)
   - 14-day trial
   - Advanced features

3. **Enterprise Plan** (Currently inactive)
   - Monthly: $29.99
   - Yearly: $299.90
   - 30-day trial
   - Full features for teams

## Product Synchronization

To sync your local plan definitions with Stripe:

1. Set `STRIPE_SYNC_PRODUCTS_ON_STARTUP=true` in your `.env` file
2. Restart the application

Or manually sync via code:
```go
paymentManager.SyncProducts(ctx)
```

## Security Considerations

1. **Webhook Signature Verification**: All webhooks are verified using the signing secret
2. **API Key Management**: Never commit API keys to version control
3. **Test vs. Live Mode**: The system automatically detects test mode based on key prefix
4. **Error Handling**: Sensitive errors are logged but not exposed to clients
5. **Idempotency**: Webhook handlers are designed to be idempotent

## Monitoring and Logging

The integration includes comprehensive logging:

- All Stripe API calls are logged with request/response details
- Webhook events are logged with event type and ID
- Errors include context for debugging
- Test mode operations are clearly marked in logs

## Future Enhancements

- [ ] Support for usage-based billing
- [ ] Multiple payment methods per customer
- [ ] Invoice customization
- [ ] Subscription upgrades/downgrades with proration
- [ ] Coupon and discount support
- [ ] Revenue reporting and analytics
- [ ] Support for multiple currencies
- [ ] Automated dunning for failed payments

## Troubleshooting

### Common Issues

1. **Webhook signature verification fails**
   - Ensure the webhook secret in `.env` matches Stripe Dashboard
   - Check that the raw request body is used for verification

2. **Products not appearing in Stripe**
   - Enable `STRIPE_SYNC_PRODUCTS_ON_STARTUP`
   - Check logs for sync errors
   - Verify API key has necessary permissions

3. **Subscriptions not updating locally**
   - Ensure webhook endpoint is configured correctly
   - Check webhook event logs in Stripe Dashboard
   - Verify database connection and migrations

### Debug Mode

Enable debug logging for Stripe operations:

```go
// In your logger configuration
logger := zap.NewDevelopment()
```

## Support

For issues or questions:
1. Check the Stripe Dashboard for API logs
2. Review application logs for errors
3. Test with Stripe CLI for webhook debugging
4. Refer to [Stripe Documentation](https://stripe.com/docs)
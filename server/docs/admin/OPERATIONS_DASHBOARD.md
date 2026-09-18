# Operations Dashboard - Admin API

Comprehensive admin API for monitoring and managing subscriptions, payments, and payment failures.

## Overview

The Operations Dashboard provides administrators with tools to:
- Monitor subscription health and metrics
- View and manage all subscriptions
- Track payment history and failures
- Perform manual subscription operations
- Receive alerts for payment issues
- Generate reports on revenue and churn

## Architecture

### Components

```
┌─────────────────────────────────────────┐
│         Admin API Endpoints             │
├─────────────────────────────────────────┤
│  SubscriptionHandlers                   │
│  MonitoringHandlers                     │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│       Business Logic Layer              │
├─────────────────────────────────────────┤
│  SubscriptionOperations                 │
│  PaymentMonitor                         │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│         Data Layer                      │
├─────────────────────────────────────────┤
│  SubscriptionRepository                 │
│  PaymentService (Stripe)                │
└─────────────────────────────────────────┘
```

### Files
- `/internal/admin/subscription_operations.go` - Subscription management operations
- `/internal/admin/subscription_handlers.go` - HTTP handlers for subscriptions
- `/internal/admin/payment_monitoring.go` - Payment health monitoring and alerts
- `/internal/admin/monitoring_handlers.go` - HTTP handlers for monitoring

## API Endpoints

### Subscription Management

#### List All Subscriptions
```http
GET /admin/subscriptions?status=active&plan_id=premium&page=0&page_size=50&sort_by=created_at&sort_order=desc
```

**Query Parameters:**
- `status` (optional): Filter by subscription status (active, trial, canceled, etc.)
- `plan_id` (optional): Filter by plan ID
- `user_id` (optional): Filter by user ID
- `page` (optional): Page number (default: 0)
- `page_size` (optional): Results per page (default: 50, max: 100)
- `sort_by` (optional): Sort field (default: created_at)
- `sort_order` (optional): Sort direction (asc, desc)

**Response:**
```json
{
  "subscriptions": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "plan_id": "premium",
      "status": "active",
      "current_period_start": "2024-01-01T00:00:00Z",
      "current_period_end": "2024-02-01T00:00:00Z",
      "cancel_at_period_end": false,
      "stripe_customer_id": "cus_xxx",
      "stripe_subscription_id": "sub_xxx",
      "recent_payments": [...],
      "stripe_details": {
        "customer_id": "cus_xxx",
        "subscription_id": "sub_xxx",
        "next_billing_date": "2024-02-01T00:00:00Z"
      }
    }
  ],
  "total": 150,
  "page": 0,
  "page_size": 50,
  "has_more": true
}
```

#### Get Subscription Details
```http
GET /admin/subscriptions/:id
```

**Response:**
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "plan_id": "premium",
  "status": "active",
  "current_period_start": "2024-01-01T00:00:00Z",
  "current_period_end": "2024-02-01T00:00:00Z",
  "cancel_at_period_end": false,
  "recent_payments": [
    {
      "id": "uuid",
      "amount": 999,
      "currency": "usd",
      "status": "succeeded",
      "paid_at": "2024-01-01T00:00:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "stripe_details": {
    "customer_id": "cus_xxx",
    "subscription_id": "sub_xxx",
    "default_payment_method": "pm_xxx",
    "next_billing_date": "2024-02-01T00:00:00Z"
  }
}
```

#### Manual Subscription Update
```http
POST /admin/subscriptions/:id/manual-update
```

**Request Body:**
```json
{
  "action": "cancel|reactivate|change_plan|extend",
  "reason": "Customer request for cancellation",
  "new_plan_id": "enterprise",    // Required for change_plan
  "extend_days": 30               // Required for extend
}
```

**Supported Actions:**
- `cancel`: Immediately cancel subscription
- `reactivate`: Reactivate a canceled subscription
- `change_plan`: Change to a different plan
- `extend`: Extend subscription by N days

**Response:**
```json
{
  "message": "Update successful",
  "subscription": { /* updated subscription details */ }
}
```

#### Get Subscription Metrics
```http
GET /admin/subscriptions/metrics
```

**Response:**
```json
{
  "total_subscriptions": 1000,
  "active_subscriptions": 800,
  "trial_subscriptions": 50,
  "canceled_subscriptions": 100,
  "past_due_subscriptions": 50,
  "mrr": 79900.0,           // Monthly Recurring Revenue in cents
  "arr": 958800.0,          // Annual Recurring Revenue in cents
  "churn_rate": 3.5,        // Percentage
  "by_plan": {
    "premium": {
      "plan_id": "premium",
      "count": 700,
      "revenue": 69900.0
    },
    "enterprise": {
      "plan_id": "enterprise",
      "count": 100,
      "revenue": 10000.0
    }
  },
  "recent_payments": {
    "last_24_hours": {
      "total_payments": 50,
      "successful_payments": 48,
      "failed_payments": 2,
      "total_amount": 49950.0,
      "average_amount": 999.0
    },
    "last_7_days": { /* ... */ },
    "last_30_days": { /* ... */ }
  }
}
```

### Payment Management

#### Get Payment History
```http
GET /admin/payments?user_id=uuid&limit=50&offset=0
```

**Query Parameters:**
- `user_id` (optional): Filter by user ID
- `limit` (optional): Number of results (default: 50, max: 100)
- `offset` (optional): Pagination offset

**Response:**
```json
{
  "payments": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "subscription_id": "uuid",
      "amount": 999,
      "currency": "usd",
      "status": "succeeded",
      "payment_method": "card",
      "stripe_payment_intent_id": "pi_xxx",
      "description": "Payment for premium subscription",
      "paid_at": "2024-01-01T00:00:00Z",
      "created_at": "2024-01-01T00:00:00Z",
      "subscription": { /* subscription details */ }
    }
  ],
  "total": 1000,
  "page": 0,
  "page_size": 50
}
```

#### Get Failed Payments
```http
GET /admin/payments/failed?limit=100
```

**Response:**
```json
{
  "failed_payments": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "subscription_id": "uuid",
      "amount": 999,
      "currency": "usd",
      "status": "failed",
      "failed_at": "2024-01-01T00:00:00Z",
      "failure_reason": "Card declined - insufficient funds",
      "subscription": {
        "id": "uuid",
        "status": "past_due",
        "plan_id": "premium"
      }
    }
  ],
  "count": 15
}
```

#### Create Refund
```http
POST /admin/payments/:id/refund
```

**Request Body:**
```json
{
  "amount": 999,              // Amount in cents, 0 for full refund
  "reason": "Customer dissatisfaction"
}
```

**Response:**
```json
{
  "message": "Refund created successfully",
  "payment_id": "uuid",
  "amount": 999
}
```

### Monitoring & Alerts

#### Get Health Check
```http
GET /admin/monitoring/health
```

**Response:**
```json
{
  "timestamp": "2024-01-01T12:00:00Z",
  "overall_health": "healthy|degraded|unhealthy",
  "alerts": [
    {
      "id": "payment_failure_1234567890",
      "type": "payment_failure",
      "severity": "warning|critical",
      "title": "High number of payment failures",
      "description": "7 payments failed in the last 24 hours (threshold: 5)",
      "data": {
        "failed_payments": 7,
        "threshold": 5
      },
      "timestamp": "2024-01-01T12:00:00Z",
      "resolved": false
    }
  ],
  "metrics": {
    "total_payments_last_24h": 100,
    "failed_payments_last_24h": 7,
    "failure_rate": 7.0,
    "past_due_count": 12,
    "current_mrr": 79900.0,
    "previous_mrr": 80000.0,
    "mrr_change": -100.0,
    "churn_rate": 3.2,
    "average_payment_amount": 999.0
  },
  "recommendations": [
    "Review payment gateway logs for common failure patterns",
    "Check if card decline rates have increased",
    "Consider implementing retry logic for failed payments"
  ]
}
```

#### Get Active Alerts
```http
GET /admin/monitoring/alerts
```

**Response:**
```json
{
  "alerts": [ /* array of alert objects */ ],
  "count": 3,
  "health": "degraded"
}
```

## Alert Types

### 1. Payment Failure
- **Type**: `payment_failure`
- **Severity**: Warning
- **Trigger**: More than N failed payments in 24 hours
- **Threshold**: 5 payments (configurable)

### 2. High Failure Rate
- **Type**: `high_failure_rate`
- **Severity**: Warning/Critical
- **Trigger**: Payment failure rate exceeds threshold
- **Threshold**: 10% (configurable)

### 3. Past Due Subscriptions
- **Type**: `past_due_subscriptions`
- **Severity**: Warning
- **Trigger**: Subscriptions past due beyond grace period
- **Grace Period**: 72 hours (configurable)

### 4. Revenue Drop
- **Type**: `revenue_drop`
- **Severity**: Critical
- **Trigger**: MRR drops by more than threshold percentage
- **Threshold**: 15% (configurable)

### 5. High Churn Rate
- **Type**: `high_churn_rate`
- **Severity**: Warning/Critical
- **Trigger**: Monthly churn rate exceeds threshold
- **Threshold**: 5% (configurable)

## Configuration

### Alert Thresholds

Configure alert thresholds when creating the PaymentMonitor:

```go
thresholds := &admin.AlertThresholds{
    FailureRatePercent:  10.0,  // 10%
    FailedPaymentsCount: 5,
    PastDueGracePeriod:  72 * time.Hour,  // 3 days
    ChurnRatePercent:    5.0,   // 5%
    RevenueDropPercent:  15.0,  // 15%
}

monitor := admin.NewPaymentMonitor(
    subscriptionRepo,
    logger,
    thresholds,
)
```

## Security & Authentication

### Admin Authentication Required

All admin endpoints require admin authentication. Implement using middleware:

```go
adminRouter := router.Group("/admin")
adminRouter.Use(middleware.RequireAdmin())

subscriptionHandlers.RegisterRoutes(adminRouter)
monitoringHandlers.RegisterRoutes(adminRouter)
```

### Audit Logging

All manual operations are logged:
- Who performed the action
- What action was performed
- When it was performed
- Why (reason field)
- Before/after state

Audit logs are stored in the `subscription_events` table.

## Usage Examples

### Check Payment Health
```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  https://api.example.com/admin/monitoring/health
```

### View All Active Subscriptions
```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "https://api.example.com/admin/subscriptions?status=active&page_size=100"
```

### Cancel Subscription with Reason
```bash
curl -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "cancel",
    "reason": "Customer requested cancellation due to budget constraints"
  }' \
  https://api.example.com/admin/subscriptions/123e4567-e89b-12d3-a456-426614174000/manual-update
```

### Process Refund
```bash
curl -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": 999,
    "reason": "Product not as described"
  }' \
  https://api.example.com/admin/payments/123e4567-e89b-12d3-a456-426614174000/refund
```

### View Failed Payments
```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "https://api.example.com/admin/payments/failed?limit=50"
```

## Metrics Explained

### MRR (Monthly Recurring Revenue)
Normalized monthly revenue from all active subscriptions:
- Monthly subscriptions: Direct monthly price
- Annual subscriptions: Annual price / 12

### ARR (Annual Recurring Revenue)
Annual recurring revenue: MRR × 12

### Churn Rate
Percentage of subscribers who canceled in the last 30 days:
```
Churn Rate = (Canceled in last 30 days / Active at start of period) × 100
```

### Failure Rate
Percentage of failed payments in the monitoring period:
```
Failure Rate = (Failed Payments / Total Payments) × 100
```

## Best Practices

### 1. Regular Monitoring
- Check health dashboard daily
- Review failed payments weekly
- Analyze churn trends monthly

### 2. Alert Response
- Respond to critical alerts within 1 hour
- Investigate warnings within 24 hours
- Document alert resolutions

### 3. Manual Operations
- Always provide detailed reasons
- Coordinate with customer support
- Document edge cases

### 4. Refund Policy
- Review refund requests carefully
- Document reason for each refund
- Track refund patterns

### 5. Data Export
- Export metrics regularly for business analysis
- Maintain historical records
- Create regular reports for stakeholders

## Troubleshooting

### High Failure Rate
1. Check Stripe dashboard for decline codes
2. Review payment method requirements
3. Check for billing address issues
4. Verify card expiration dates

### Past Due Subscriptions
1. Send payment reminder emails
2. Check if payment methods need updating
3. Offer payment plan alternatives
4. Review dunning management settings

### Revenue Drop
1. Analyze recent cancellations
2. Check for seasonal patterns
3. Review pricing changes
4. Investigate product issues

## Future Enhancements

1. **Automated Dunning**: Automatic retry logic for failed payments
2. **Predictive Analytics**: ML-based churn prediction
3. **Automated Alerts**: Email/Slack notifications for critical alerts
4. **Advanced Reporting**: Custom report builder
5. **Customer Segments**: Cohort analysis and segmentation
6. **Revenue Forecasting**: Predictive revenue modeling

## Related Documentation

- [Payment Integration Tests](../testing/PAYMENT_INTEGRATION_TESTS.md)
- [Stripe Webhook Guide](../STRIPE_WEBHOOKS.md)
- [Admin Authentication](../ADMIN_AUTHENTICATION.md)
- [Audit Logging](../AUDIT_LOGGING.md)

package subscription

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Repository handles subscription data persistence
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new subscription repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		db: db,
	}
}

// CreateSubscription creates a new subscription
func (r *Repository) CreateSubscription(ctx context.Context, sub *Subscription) error {
	query := `
		INSERT INTO subscriptions (
			id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		sub.ID, sub.UserID, sub.PlanID, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.CanceledAt, sub.TrialStart, sub.TrialEnd,
		sub.StripeCustomerID, sub.StripeSubscriptionID, sub.Metadata, sub.CreatedAt, sub.UpdatedAt,
	)

	return err
}

// GetSubscription retrieves a subscription by ID
func (r *Repository) GetSubscription(ctx context.Context, id uuid.UUID) (*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		FROM subscriptions
		WHERE id = $1
	`

	var sub Subscription
	err := r.db.GetContext(ctx, &sub, query, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &sub, nil
}

// GetSubscriptionByUserID retrieves the active subscription for a user
func (r *Repository) GetSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		FROM subscriptions
		WHERE user_id = $1 AND status NOT IN ('expired')
		ORDER BY created_at DESC
		LIMIT 1
	`

	var sub Subscription
	err := r.db.GetContext(ctx, &sub, query, userID)
	if err == sql.ErrNoRows {
		return nil, nil // No active subscription
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscription: %w", err)
	}

	return &sub, nil
}

// GetSubscriptionByStripeID retrieves a subscription by Stripe subscription ID
func (r *Repository) GetSubscriptionByStripeID(ctx context.Context, stripeSubID string) (*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		FROM subscriptions
		WHERE stripe_subscription_id = $1
	`

	var sub Subscription
	err := r.db.GetContext(ctx, &sub, query, stripeSubID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by stripe ID: %w", err)
	}

	return &sub, nil
}

// UpdateSubscription updates an existing subscription
func (r *Repository) UpdateSubscription(ctx context.Context, sub *Subscription) error {
	query := `
		UPDATE subscriptions SET
			plan_id = $2, status = $3, current_period_start = $4, current_period_end = $5,
			cancel_at_period_end = $6, canceled_at = $7, trial_start = $8, trial_end = $9,
			stripe_customer_id = $10, stripe_subscription_id = $11, metadata = $12, updated_at = $13
		WHERE id = $1
	`

	sub.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		sub.ID, sub.PlanID, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.CanceledAt, sub.TrialStart, sub.TrialEnd,
		sub.StripeCustomerID, sub.StripeSubscriptionID, sub.Metadata, sub.UpdatedAt,
	)

	return err
}

// UpdateSubscriptionStatus updates only the status of a subscription
func (r *Repository) UpdateSubscriptionStatus(ctx context.Context, id uuid.UUID, status SubscriptionStatus) error {
	query := `
		UPDATE subscriptions SET
			status = $2, updated_at = $3
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, status, time.Now())
	return err
}

// ListSubscriptions retrieves subscriptions based on filter criteria
func (r *Repository) ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		FROM subscriptions
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if filter.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argCount)
		args = append(args, *filter.UserID)
		argCount++
	}

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, *filter.Status)
		argCount++
	}

	if filter.PlanID != nil {
		query += fmt.Sprintf(" AND plan_id = $%d", argCount)
		args = append(args, *filter.PlanID)
		argCount++
	}

	// Sorting
	sortBy := "created_at"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	sortOrder := "DESC"
	if !filter.SortDesc {
		sortOrder = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Pagination
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.Limit)
		argCount++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, filter.Offset)
	}

	var subs []*Subscription
	err := r.db.SelectContext(ctx, &subs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}

	return subs, nil
}

// GetExpiredSubscriptions retrieves subscriptions that have expired
func (r *Repository) GetExpiredSubscriptions(ctx context.Context) ([]*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		FROM subscriptions
		WHERE status IN ('active', 'trial', 'past_due')
			AND current_period_end < $1
	`

	var subs []*Subscription
	err := r.db.SelectContext(ctx, &subs, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get expired subscriptions: %w", err)
	}

	return subs, nil
}

// GetPastDueSubscriptions retrieves subscriptions in past_due status
func (r *Repository) GetPastDueSubscriptions(ctx context.Context, gracePeriodDays int) ([]*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, trial_start, trial_end,
			stripe_customer_id, stripe_subscription_id, metadata, created_at, updated_at
		FROM subscriptions
		WHERE status = 'past_due'
			AND updated_at < $1
	`

	gracePeriodEnd := time.Now().Add(-time.Duration(gracePeriodDays) * 24 * time.Hour)
	var subs []*Subscription
	err := r.db.SelectContext(ctx, &subs, query, gracePeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to get past due subscriptions: %w", err)
	}

	return subs, nil
}

// DeleteSubscription soft-deletes a subscription (actually just expires it)
func (r *Repository) DeleteSubscription(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE subscriptions SET
			status = 'expired', updated_at = $2
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

// CreatePayment creates a new payment record
func (r *Repository) CreatePayment(ctx context.Context, payment *Payment) error {
	query := `
		INSERT INTO payments (
			id, user_id, subscription_id, amount, currency, status, payment_method,
			stripe_payment_intent_id, stripe_charge_id, description, metadata,
			paid_at, failed_at, failure_reason, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		payment.ID, payment.UserID, payment.SubscriptionID, payment.Amount, payment.Currency,
		payment.Status, payment.PaymentMethod, payment.StripePaymentIntentID, payment.StripeChargeID,
		payment.Description, payment.Metadata, payment.PaidAt, payment.FailedAt, payment.FailureReason,
		payment.CreatedAt,
	)

	return err
}

// GetPayment retrieves a payment by ID
func (r *Repository) GetPayment(ctx context.Context, id uuid.UUID) (*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, amount, currency, status, payment_method,
			stripe_payment_intent_id, stripe_charge_id, description, metadata,
			paid_at, failed_at, failure_reason, created_at
		FROM payments
		WHERE id = $1
	`

	var payment Payment
	err := r.db.GetContext(ctx, &payment, query, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	return &payment, nil
}

// GetPaymentByStripePaymentIntentID retrieves a payment by Stripe payment intent ID
func (r *Repository) GetPaymentByStripePaymentIntentID(ctx context.Context, paymentIntentID string) (*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, amount, currency, status, payment_method,
			stripe_payment_intent_id, stripe_charge_id, description, metadata,
			paid_at, failed_at, failure_reason, created_at
		FROM payments
		WHERE stripe_payment_intent_id = $1
	`

	var payment Payment
	err := r.db.GetContext(ctx, &payment, query, paymentIntentID)
	if err == sql.ErrNoRows {
		return nil, nil // Payment not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment by stripe payment intent ID: %w", err)
	}

	return &payment, nil
}

// ListPayments retrieves payments for a user or subscription
func (r *Repository) ListPayments(ctx context.Context, userID *uuid.UUID, subscriptionID *uuid.UUID, limit int) ([]*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, amount, currency, status, payment_method,
			stripe_payment_intent_id, stripe_charge_id, description, metadata,
			paid_at, failed_at, failure_reason, created_at
		FROM payments
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if userID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argCount)
		args = append(args, *userID)
		argCount++
	}

	if subscriptionID != nil {
		query += fmt.Sprintf(" AND subscription_id = $%d", argCount)
		args = append(args, *subscriptionID)
		argCount++
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, limit)
	}

	var payments []*Payment
	err := r.db.SelectContext(ctx, &payments, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list payments: %w", err)
	}

	return payments, nil
}

// UpdatePayment updates a payment record
func (r *Repository) UpdatePayment(ctx context.Context, payment *Payment) error {
	query := `
		UPDATE payments SET
			status = $2, paid_at = $3, failed_at = $4, failure_reason = $5, metadata = $6
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query,
		payment.ID, payment.Status, payment.PaidAt, payment.FailedAt, payment.FailureReason, payment.Metadata,
	)

	return err
}

// CreateSubscriptionEvent creates an audit log entry
func (r *Repository) CreateSubscriptionEvent(ctx context.Context, event *SubscriptionEvent) error {
	query := `
		INSERT INTO subscription_events (
			id, subscription_id, event_type, previous_value, new_value, reason, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		event.ID, event.SubscriptionID, event.EventType, event.PreviousValue, event.NewValue,
		event.Reason, event.CreatedAt,
	)

	return err
}

// ListSubscriptionEvents retrieves events for a subscription
func (r *Repository) ListSubscriptionEvents(ctx context.Context, subscriptionID uuid.UUID, limit int) ([]*SubscriptionEvent, error) {
	query := `
		SELECT id, subscription_id, event_type, previous_value, new_value, reason, created_at
		FROM subscription_events
		WHERE subscription_id = $1
		ORDER BY created_at DESC
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	var events []*SubscriptionEvent
	err := r.db.SelectContext(ctx, &events, query, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscription events: %w", err)
	}

	return events, nil
}

// LogSubscriptionEvent is a helper to create and log a subscription event
func (r *Repository) LogSubscriptionEvent(ctx context.Context, subscriptionID uuid.UUID, eventType string, previousValue, newValue interface{}, reason string) error {
	var prevJSON, newJSON JSONB

	if previousValue != nil {
		prevBytes, err := json.Marshal(previousValue)
		if err == nil {
			json.Unmarshal(prevBytes, &prevJSON)
		}
	}

	if newValue != nil {
		newBytes, err := json.Marshal(newValue)
		if err == nil {
			json.Unmarshal(newBytes, &newJSON)
		}
	}

	event := &SubscriptionEvent{
		ID:             uuid.New(),
		SubscriptionID: subscriptionID,
		EventType:      eventType,
		PreviousValue:  prevJSON,
		NewValue:       newJSON,
		Reason:         &reason,
		CreatedAt:      time.Now(),
	}

	return r.CreateSubscriptionEvent(ctx, event)
}

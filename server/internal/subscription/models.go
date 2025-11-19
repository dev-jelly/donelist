package subscription

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SubscriptionStatus represents the status of a subscription
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

// IsValid checks if the subscription status is valid
func (s SubscriptionStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusActive, StatusTrial, StatusPastDue,
		StatusIncomplete, StatusCanceled, StatusExpired:
		return true
	default:
		return false
	}
}

// String returns the string representation of the status
func (s SubscriptionStatus) String() string {
	return string(s)
}

// PlanID represents subscription plan identifiers
type PlanID string

const (
	PlanFree       PlanID = "free"
	PlanPremium    PlanID = "premium"
	PlanEnterprise PlanID = "enterprise"
)

// IsValid checks if the plan ID is valid
func (p PlanID) IsValid() bool {
	switch p {
	case PlanFree, PlanPremium, PlanEnterprise:
		return true
	default:
		return false
	}
}

// String returns the string representation of the plan
func (p PlanID) String() string {
	return string(p)
}

// BillingInterval represents the billing cycle
type BillingInterval string

const (
	IntervalMonth BillingInterval = "month"
	IntervalYear  BillingInterval = "year"
)

// Subscription represents a user's subscription
type Subscription struct {
	ID                   uuid.UUID          `db:"id" json:"id"`
	UserID               uuid.UUID          `db:"user_id" json:"user_id"`
	PlanID               string             `db:"plan_id" json:"plan_id"`
	Status               SubscriptionStatus `db:"status" json:"status"`
	CurrentPeriodStart   time.Time          `db:"current_period_start" json:"current_period_start"`
	CurrentPeriodEnd     time.Time          `db:"current_period_end" json:"current_period_end"`
	CancelAtPeriodEnd    bool               `db:"cancel_at_period_end" json:"cancel_at_period_end"`
	CanceledAt           *time.Time         `db:"canceled_at" json:"canceled_at,omitempty"`
	TrialStart           *time.Time         `db:"trial_start" json:"trial_start,omitempty"`
	TrialEnd             *time.Time         `db:"trial_end" json:"trial_end,omitempty"`
	StripeCustomerID     *string            `db:"stripe_customer_id" json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID *string            `db:"stripe_subscription_id" json:"stripe_subscription_id,omitempty"`
	Metadata             JSONB              `db:"metadata" json:"metadata,omitempty"`
	CreatedAt            time.Time          `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time          `db:"updated_at" json:"updated_at"`
}

// IsActive returns true if the subscription is currently active
func (s *Subscription) IsActive() bool {
	return s.Status == StatusActive && time.Now().Before(s.CurrentPeriodEnd)
}

// IsTrial returns true if the subscription is in trial period
func (s *Subscription) IsTrial() bool {
	if s.TrialStart == nil || s.TrialEnd == nil {
		return false
	}
	now := time.Now()
	return s.Status == StatusTrial && now.After(*s.TrialStart) && now.Before(*s.TrialEnd)
}

// IsPastDue returns true if the subscription is past due
func (s *Subscription) IsPastDue() bool {
	return s.Status == StatusPastDue
}

// IsCanceled returns true if the subscription is canceled
func (s *Subscription) IsCanceled() bool {
	return s.Status == StatusCanceled
}

// IsExpired returns true if the subscription is expired
func (s *Subscription) IsExpired() bool {
	return s.Status == StatusExpired || time.Now().After(s.CurrentPeriodEnd)
}

// DaysUntilExpiry returns the number of days until the subscription expires
func (s *Subscription) DaysUntilExpiry() int {
	duration := time.Until(s.CurrentPeriodEnd)
	return int(duration.Hours() / 24)
}

// Payment represents a payment transaction
type Payment struct {
	ID                    uuid.UUID  `db:"id" json:"id"`
	UserID                uuid.UUID  `db:"user_id" json:"user_id"`
	SubscriptionID        *uuid.UUID `db:"subscription_id" json:"subscription_id,omitempty"`
	Amount                int        `db:"amount" json:"amount"` // Amount in cents
	Currency              string     `db:"currency" json:"currency"`
	Status                string     `db:"status" json:"status"`
	PaymentMethod         *string    `db:"payment_method" json:"payment_method,omitempty"`
	StripePaymentIntentID *string    `db:"stripe_payment_intent_id" json:"stripe_payment_intent_id,omitempty"`
	StripeChargeID        *string    `db:"stripe_charge_id" json:"stripe_charge_id,omitempty"`
	Description           *string    `db:"description" json:"description,omitempty"`
	Metadata              JSONB      `db:"metadata" json:"metadata,omitempty"`
	PaidAt                *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	FailedAt              *time.Time `db:"failed_at" json:"failed_at,omitempty"`
	FailureReason         *string    `db:"failure_reason" json:"failure_reason,omitempty"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
}

// PaymentStatus constants
const (
	PaymentStatusPending   = "pending"
	PaymentStatusSucceeded = "succeeded"
	PaymentStatusFailed    = "failed"
	PaymentStatusRefunded  = "refunded"
)

// IsSuccessful returns true if the payment was successful
func (p *Payment) IsSuccessful() bool {
	return p.Status == PaymentStatusSucceeded
}

// IsFailed returns true if the payment failed
func (p *Payment) IsFailed() bool {
	return p.Status == PaymentStatusFailed
}

// SubscriptionEvent represents an audit log entry for subscription changes
type SubscriptionEvent struct {
	ID             uuid.UUID     `db:"id" json:"id"`
	SubscriptionID uuid.UUID     `db:"subscription_id" json:"subscription_id"`
	EventType      string        `db:"event_type" json:"event_type"`
	PreviousValue  JSONB         `db:"previous_value" json:"previous_value,omitempty"`
	NewValue       JSONB         `db:"new_value" json:"new_value,omitempty"`
	Reason         *string       `db:"reason" json:"reason,omitempty"`
	CreatedAt      time.Time     `db:"created_at" json:"created_at"`
}

// Event type constants
const (
	EventTypeCreated        = "subscription.created"
	EventTypeActivated      = "subscription.activated"
	EventTypeTrialStarted   = "subscription.trial_started"
	EventTypeTrialEnded     = "subscription.trial_ended"
	EventTypeUpgraded       = "subscription.upgraded"
	EventTypeDowngraded     = "subscription.downgraded"
	EventTypeCanceled       = "subscription.canceled"
	EventTypeReactivated    = "subscription.reactivated"
	EventTypeExpired        = "subscription.expired"
	EventTypePastDue        = "subscription.past_due"
	EventTypePaymentFailed  = "payment.failed"
	EventTypePaymentSucceeded = "payment.succeeded"
	EventTypeRefunded       = "payment.refunded"
)

// Plan represents a subscription plan configuration
type Plan struct {
	ID              PlanID          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Currency        string          `json:"currency"`
	MonthlyPrice    int             `json:"monthly_price"` // Amount in cents
	YearlyPrice     int             `json:"yearly_price"`  // Amount in cents
	TrialDays       int             `json:"trial_days"`
	Features        []string        `json:"features"`
	StripePriceID   *string         `json:"stripe_price_id,omitempty"`
	Active          bool            `json:"active"`
}

// GetPrice returns the price for a given billing interval
func (p *Plan) GetPrice(interval BillingInterval) int {
	if interval == IntervalYear {
		return p.YearlyPrice
	}
	return p.MonthlyPrice
}

// PredefinedPlans contains the configured subscription plans
var PredefinedPlans = map[PlanID]*Plan{
	PlanFree: {
		ID:           PlanFree,
		Name:         "Free",
		Description:  "Basic features for personal use",
		Currency:     "usd",
		MonthlyPrice: 0,
		YearlyPrice:  0,
		TrialDays:    0,
		Features: []string{
			"Unlimited check-ins",
			"2-hour edit window",
			"Basic categories and tags",
			"7-day history",
		},
		Active: true,
	},
	PlanPremium: {
		ID:           PlanPremium,
		Name:         "Premium",
		Description:  "Advanced features for power users",
		Currency:     "usd",
		MonthlyPrice: 999,  // $9.99
		YearlyPrice:  9990, // $99.90 (2 months free)
		TrialDays:    14,
		Features: []string{
			"All Free features",
			"Unlimited edit history",
			"Edit old check-ins anytime",
			"Advanced analytics",
			"Priority support",
			"Export data",
		},
		Active: true,
	},
	PlanEnterprise: {
		ID:           PlanEnterprise,
		Name:         "Enterprise",
		Description:  "Full features for teams",
		Currency:     "usd",
		MonthlyPrice: 2999,  // $29.99
		YearlyPrice:  29990, // $299.90
		TrialDays:    30,
		Features: []string{
			"All Premium features",
			"Team collaboration",
			"Custom integrations",
			"SSO authentication",
			"Dedicated support",
		},
		Active: false, // Not yet available
	},
}

// GetPlan returns a plan by ID
func GetPlan(id PlanID) (*Plan, bool) {
	plan, ok := PredefinedPlans[id]
	return plan, ok
}

// ListActivePlans returns all active plans
func ListActivePlans() []*Plan {
	plans := []*Plan{}
	for _, plan := range PredefinedPlans {
		if plan.Active {
			plans = append(plans, plan)
		}
	}
	return plans
}

// JSONB is a custom type for PostgreSQL JSONB columns
type JSONB map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	result := make(JSONB)
	err := json.Unmarshal(bytes, &result)
	*j = result
	return err
}

// CreateSubscriptionParams contains parameters for creating a subscription
type CreateSubscriptionParams struct {
	UserID          uuid.UUID
	PlanID          PlanID
	BillingInterval BillingInterval
	TrialDays       int
	PaymentMethodID *string
	SuccessURL      string
	CancelURL       string
}

// UpdateSubscriptionParams contains parameters for updating a subscription
type UpdateSubscriptionParams struct {
	PlanID            *PlanID
	Status            *SubscriptionStatus
	CancelAtPeriodEnd *bool
}

// SubscriptionFilter contains filter parameters for querying subscriptions
type SubscriptionFilter struct {
	UserID   *uuid.UUID
	Status   *SubscriptionStatus
	PlanID   *PlanID
	Limit    int
	Offset   int
	SortBy   string
	SortDesc bool
}

package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/webhook"
	"go.uber.org/zap"
)

// Service provides payment processing functionality
type Service struct {
	client *StripeClient
	logger *zap.Logger
}

// NewService creates a new payment service
func NewService(client *StripeClient, logger *zap.Logger) *Service {
	return &Service{
		client: client,
		logger: logger,
	}
}

// CreateCustomer creates a new Stripe customer
func (s *Service) CreateCustomer(ctx context.Context, userID uuid.UUID, email, name string) (*stripe.Customer, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
		Metadata: map[string]string{
			"user_id": userID.String(),
		},
	}

	customer, err := s.client.GetClient().Customers.New(params)
	if err != nil {
		s.logger.Error("Failed to create Stripe customer",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("email", email),
		)
		return nil, fmt.Errorf("failed to create customer: %w", err)
	}

	s.logger.Info("Created Stripe customer",
		zap.String("customer_id", customer.ID),
		zap.String("user_id", userID.String()),
	)

	return customer, nil
}

// UpdateCustomer updates an existing Stripe customer
func (s *Service) UpdateCustomer(ctx context.Context, customerID string, email, name string) (*stripe.Customer, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
	}

	customer, err := s.client.GetClient().Customers.Update(customerID, params)
	if err != nil {
		s.logger.Error("Failed to update Stripe customer",
			zap.Error(err),
			zap.String("customer_id", customerID),
		)
		return nil, fmt.Errorf("failed to update customer: %w", err)
	}

	return customer, nil
}

// CreateCheckoutSession creates a Stripe Checkout session for subscription
func (s *Service) CreateCheckoutSession(ctx context.Context, params *CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	// Build line items based on the plan
	lineItems := []*stripe.CheckoutSessionLineItemParams{
		{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String(params.Currency),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(params.PlanName),
					Description: stripe.String(params.PlanDescription),
				},
				UnitAmount: stripe.Int64(int64(params.Amount)),
				Recurring: &stripe.CheckoutSessionLineItemPriceDataRecurringParams{
					Interval: stripe.String(string(params.BillingInterval)),
				},
			},
			Quantity: stripe.Int64(1),
		},
	}

	// Create checkout session parameters
	checkoutParams := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems:  lineItems,
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
		CustomerEmail: stripe.String(params.CustomerEmail),
		Metadata: map[string]string{
			"user_id": params.UserID.String(),
			"plan_id": params.PlanID,
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id": params.UserID.String(),
				"plan_id": params.PlanID,
			},
		},
	}

	// Add customer ID if provided
	if params.CustomerID != "" {
		checkoutParams.Customer = stripe.String(params.CustomerID)
		checkoutParams.CustomerEmail = nil // Don't set email if customer ID is provided
	}

	// Add trial period if specified
	if params.TrialDays > 0 {
		checkoutParams.SubscriptionData.TrialPeriodDays = stripe.Int64(int64(params.TrialDays))
	}

	// Create the session
	session, err := s.client.GetClient().CheckoutSessions.New(checkoutParams)
	if err != nil {
		s.logger.Error("Failed to create checkout session",
			zap.Error(err),
			zap.String("user_id", params.UserID.String()),
		)
		return nil, fmt.Errorf("failed to create checkout session: %w", err)
	}

	s.logger.Info("Created checkout session",
		zap.String("session_id", session.ID),
		zap.String("user_id", params.UserID.String()),
	)

	return session, nil
}

// CreateSubscription creates a subscription directly (without checkout)
func (s *Service) CreateSubscription(ctx context.Context, params *CreateSubscriptionParams) (*stripe.Subscription, error) {
	subParams := &stripe.SubscriptionParams{
		Customer: stripe.String(params.CustomerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				PriceData: &stripe.SubscriptionItemPriceDataParams{
					Currency: stripe.String(params.Currency),
					Product: stripe.String(params.ProductID),
					UnitAmount: stripe.Int64(int64(params.Amount)),
					Recurring: &stripe.SubscriptionItemPriceDataRecurringParams{
						Interval: stripe.String(string(params.BillingInterval)),
					},
				},
			},
		},
		Metadata: map[string]string{
			"user_id": params.UserID.String(),
			"plan_id": params.PlanID,
		},
	}

	// Add payment method if provided
	if params.PaymentMethodID != "" {
		subParams.DefaultPaymentMethod = stripe.String(params.PaymentMethodID)
	}

	// Add trial period if specified
	if params.TrialDays > 0 {
		trialEnd := time.Now().AddDate(0, 0, params.TrialDays).Unix()
		subParams.TrialEnd = stripe.Int64(trialEnd)
	}

	sub, err := s.client.GetClient().Subscriptions.New(subParams)
	if err != nil {
		s.logger.Error("Failed to create subscription",
			zap.Error(err),
			zap.String("user_id", params.UserID.String()),
		)
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	s.logger.Info("Created subscription",
		zap.String("subscription_id", sub.ID),
		zap.String("user_id", params.UserID.String()),
	)

	return sub, nil
}

// CancelSubscription cancels a subscription
func (s *Service) CancelSubscription(ctx context.Context, subscriptionID string, immediately bool) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{}

	if immediately {
		// Cancel immediately
		params.CancelAtPeriodEnd = stripe.Bool(false)
	} else {
		// Cancel at the end of the current period
		params.CancelAtPeriodEnd = stripe.Bool(true)
	}

	sub, err := s.client.GetClient().Subscriptions.Update(subscriptionID, params)
	if err != nil {
		s.logger.Error("Failed to cancel subscription",
			zap.Error(err),
			zap.String("subscription_id", subscriptionID),
		)
		return nil, fmt.Errorf("failed to cancel subscription: %w", err)
	}

	s.logger.Info("Cancelled subscription",
		zap.String("subscription_id", subscriptionID),
		zap.Bool("immediately", immediately),
	)

	return sub, nil
}

// UpdateSubscription updates a subscription (e.g., changing plans)
func (s *Service) UpdateSubscription(ctx context.Context, subscriptionID string, params *UpdateSubscriptionParams) (*stripe.Subscription, error) {
	updateParams := &stripe.SubscriptionParams{}

	// Update items if new price ID is provided
	if params.NewPriceID != "" {
		// First, get the current subscription to find the item to update
		sub, err := s.client.GetClient().Subscriptions.Get(subscriptionID, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get subscription: %w", err)
		}

		if len(sub.Items.Data) > 0 {
			updateParams.Items = []*stripe.SubscriptionItemsParams{
				{
					ID:    stripe.String(sub.Items.Data[0].ID),
					Price: stripe.String(params.NewPriceID),
				},
			}
		}
	}

	// Update payment method if provided
	if params.PaymentMethodID != "" {
		updateParams.DefaultPaymentMethod = stripe.String(params.PaymentMethodID)
	}

	// Update metadata if provided
	if params.Metadata != nil {
		updateParams.Metadata = params.Metadata
	}

	sub, err := s.client.GetClient().Subscriptions.Update(subscriptionID, updateParams)
	if err != nil {
		s.logger.Error("Failed to update subscription",
			zap.Error(err),
			zap.String("subscription_id", subscriptionID),
		)
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	s.logger.Info("Updated subscription",
		zap.String("subscription_id", subscriptionID),
	)

	return sub, nil
}

// GetSubscription retrieves a subscription from Stripe
func (s *Service) GetSubscription(ctx context.Context, subscriptionID string) (*stripe.Subscription, error) {
	sub, err := s.client.GetClient().Subscriptions.Get(subscriptionID, nil)
	if err != nil {
		s.logger.Error("Failed to get subscription",
			zap.Error(err),
			zap.String("subscription_id", subscriptionID),
		)
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return sub, nil
}

// CreatePaymentIntent creates a payment intent for one-time payments
func (s *Service) CreatePaymentIntent(ctx context.Context, params *PaymentIntentParams) (*stripe.PaymentIntent, error) {
	piParams := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(int64(params.Amount)),
		Currency: stripe.String(params.Currency),
		Metadata: map[string]string{
			"user_id": params.UserID.String(),
		},
	}

	if params.CustomerID != "" {
		piParams.Customer = stripe.String(params.CustomerID)
	}

	if params.Description != "" {
		piParams.Description = stripe.String(params.Description)
	}

	if params.PaymentMethodID != "" {
		piParams.PaymentMethod = stripe.String(params.PaymentMethodID)
		piParams.Confirm = stripe.Bool(true)
	}

	pi, err := s.client.GetClient().PaymentIntents.New(piParams)
	if err != nil {
		s.logger.Error("Failed to create payment intent",
			zap.Error(err),
			zap.String("user_id", params.UserID.String()),
		)
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	s.logger.Info("Created payment intent",
		zap.String("payment_intent_id", pi.ID),
		zap.String("user_id", params.UserID.String()),
	)

	return pi, nil
}

// AttachPaymentMethod attaches a payment method to a customer
func (s *Service) AttachPaymentMethod(ctx context.Context, paymentMethodID, customerID string) (*stripe.PaymentMethod, error) {
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(customerID),
	}

	pm, err := s.client.GetClient().PaymentMethods.Attach(paymentMethodID, params)
	if err != nil {
		s.logger.Error("Failed to attach payment method",
			zap.Error(err),
			zap.String("payment_method_id", paymentMethodID),
			zap.String("customer_id", customerID),
		)
		return nil, fmt.Errorf("failed to attach payment method: %w", err)
	}

	return pm, nil
}

// SetDefaultPaymentMethod sets the default payment method for a customer
func (s *Service) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	params := &stripe.CustomerParams{
		InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(paymentMethodID),
		},
	}

	_, err := s.client.GetClient().Customers.Update(customerID, params)
	if err != nil {
		s.logger.Error("Failed to set default payment method",
			zap.Error(err),
			zap.String("customer_id", customerID),
			zap.String("payment_method_id", paymentMethodID),
		)
		return fmt.Errorf("failed to set default payment method: %w", err)
	}

	return nil
}

// VerifyWebhookSignature verifies a Stripe webhook signature
func (s *Service) VerifyWebhookSignature(payload []byte, signature string) (*stripe.Event, error) {
	event, err := webhook.ConstructEvent(payload, signature, s.client.GetWebhookSecret())
	if err != nil {
		s.logger.Error("Failed to verify webhook signature",
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to verify webhook signature: %w", err)
	}

	return &event, nil
}

// MapStripeStatusToSubscriptionStatus converts Stripe subscription status to our internal status
func MapStripeStatusToSubscriptionStatus(stripeStatus stripe.SubscriptionStatus) subscription.SubscriptionStatus {
	switch stripeStatus {
	case stripe.SubscriptionStatusActive:
		return subscription.StatusActive
	case stripe.SubscriptionStatusPastDue:
		return subscription.StatusPastDue
	case stripe.SubscriptionStatusCanceled:
		return subscription.StatusCanceled
	case stripe.SubscriptionStatusIncomplete:
		return subscription.StatusIncomplete
	case stripe.SubscriptionStatusIncompleteExpired:
		return subscription.StatusExpired
	case stripe.SubscriptionStatusTrialing:
		return subscription.StatusTrial
	case stripe.SubscriptionStatusUnpaid:
		return subscription.StatusPastDue
	default:
		return subscription.StatusPending
	}
}

// CheckoutSessionParams contains parameters for creating a checkout session
type CheckoutSessionParams struct {
	UserID          uuid.UUID
	CustomerID      string // Optional: existing Stripe customer ID
	CustomerEmail   string
	PlanID          string
	PlanName        string
	PlanDescription string
	Amount          int // Amount in cents
	Currency        string
	BillingInterval subscription.BillingInterval
	TrialDays       int
	SuccessURL      string
	CancelURL       string
}

// CreateSubscriptionParams contains parameters for creating a subscription
type CreateSubscriptionParams struct {
	UserID          uuid.UUID
	CustomerID      string
	ProductID       string
	PlanID          string
	Amount          int
	Currency        string
	BillingInterval subscription.BillingInterval
	PaymentMethodID string
	TrialDays       int
}

// UpdateSubscriptionParams contains parameters for updating a subscription
type UpdateSubscriptionParams struct {
	NewPriceID      string
	PaymentMethodID string
	Metadata        map[string]string
}

// PaymentIntentParams contains parameters for creating a payment intent
type PaymentIntentParams struct {
	UserID          uuid.UUID
	CustomerID      string
	Amount          int
	Currency        string
	Description     string
	PaymentMethodID string
}
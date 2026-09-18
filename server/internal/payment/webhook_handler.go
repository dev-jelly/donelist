package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v79"
	"go.uber.org/zap"
)

// WebhookHandler handles Stripe webhook events
type WebhookHandler struct {
	service          *Service
	subscriptionRepo *subscription.Repository
	logger           *zap.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(service *Service, subscriptionRepo *subscription.Repository, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		service:          service,
		subscriptionRepo: subscriptionRepo,
		logger:           logger,
	}
}

// HandleEvent processes a Stripe webhook event
func (h *WebhookHandler) HandleEvent(ctx context.Context, event *stripe.Event) error {
	h.logger.Info("Processing Stripe webhook event",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
	)

	switch event.Type {
	case "customer.subscription.created":
		return h.handleSubscriptionCreated(ctx, event)
	case "customer.subscription.updated":
		return h.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return h.handleSubscriptionDeleted(ctx, event)
	case "customer.subscription.trial_will_end":
		return h.handleSubscriptionTrialWillEnd(ctx, event)
	case "invoice.payment_succeeded":
		return h.handleInvoicePaymentSucceeded(ctx, event)
	case "invoice.payment_failed":
		return h.handleInvoicePaymentFailed(ctx, event)
	case "payment_intent.succeeded":
		return h.handlePaymentIntentSucceeded(ctx, event)
	case "payment_intent.payment_failed":
		return h.handlePaymentIntentFailed(ctx, event)
	case "payment_method.attached":
		return h.handlePaymentMethodAttached(ctx, event)
	case "checkout.session.completed":
		return h.handleCheckoutSessionCompleted(ctx, event)
	default:
		h.logger.Debug("Unhandled webhook event type",
			zap.String("event_type", string(event.Type)),
		)
		return nil
	}
}

// handleSubscriptionCreated handles subscription.created events
func (h *WebhookHandler) handleSubscriptionCreated(ctx context.Context, event *stripe.Event) error {
	var stripeSubscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &stripeSubscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	// Extract user ID from metadata
	userIDStr, ok := stripeSubscription.Metadata["user_id"]
	if !ok {
		return fmt.Errorf("user_id not found in subscription metadata")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	planID, ok := stripeSubscription.Metadata["plan_id"]
	if !ok {
		planID = "premium" // Default plan
	}

	h.logger.Info("Subscription created",
		zap.String("subscription_id", stripeSubscription.ID),
		zap.String("user_id", userID.String()),
		zap.String("plan_id", planID),
		zap.String("status", string(stripeSubscription.Status)),
	)

	// Create subscription record in database
	sub := &subscription.Subscription{
		ID:                   uuid.New(),
		UserID:               userID,
		PlanID:               planID,
		Status:               MapStripeStatusToSubscriptionStatus(stripeSubscription.Status),
		CurrentPeriodStart:   time.Unix(stripeSubscription.CurrentPeriodStart, 0),
		CurrentPeriodEnd:     time.Unix(stripeSubscription.CurrentPeriodEnd, 0),
		CancelAtPeriodEnd:    stripeSubscription.CancelAtPeriodEnd,
		StripeCustomerID:     &stripeSubscription.Customer.ID,
		StripeSubscriptionID: &stripeSubscription.ID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	// Handle trial period
	if stripeSubscription.TrialStart > 0 {
		trialStart := time.Unix(stripeSubscription.TrialStart, 0)
		sub.TrialStart = &trialStart
	}
	if stripeSubscription.TrialEnd > 0 {
		trialEnd := time.Unix(stripeSubscription.TrialEnd, 0)
		sub.TrialEnd = &trialEnd
	}

	if err := h.subscriptionRepo.CreateSubscription(ctx, sub); err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	// Log subscription event
	if err := h.subscriptionRepo.LogSubscriptionEvent(ctx, sub.ID, subscription.EventTypeCreated, nil, sub, "Created via Stripe webhook"); err != nil {
		h.logger.Warn("Failed to log subscription event", zap.Error(err))
	}

	return nil
}

// handleSubscriptionUpdated handles subscription.updated events
func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, event *stripe.Event) error {
	var stripeSubscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &stripeSubscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	h.logger.Info("Subscription updated",
		zap.String("subscription_id", stripeSubscription.ID),
		zap.String("status", string(stripeSubscription.Status)),
		zap.Bool("cancel_at_period_end", stripeSubscription.CancelAtPeriodEnd),
	)

	// Get existing subscription
	existingSub, err := h.subscriptionRepo.GetSubscriptionByStripeID(ctx, stripeSubscription.ID)
	if err != nil {
		return fmt.Errorf("failed to get existing subscription: %w", err)
	}

	// Store previous state for event logging
	previousState := *existingSub

	// Update subscription fields
	existingSub.Status = MapStripeStatusToSubscriptionStatus(stripeSubscription.Status)
	existingSub.CurrentPeriodStart = time.Unix(stripeSubscription.CurrentPeriodStart, 0)
	existingSub.CurrentPeriodEnd = time.Unix(stripeSubscription.CurrentPeriodEnd, 0)
	existingSub.CancelAtPeriodEnd = stripeSubscription.CancelAtPeriodEnd
	existingSub.UpdatedAt = time.Now()

	// Handle trial updates
	if stripeSubscription.TrialStart > 0 {
		trialStart := time.Unix(stripeSubscription.TrialStart, 0)
		existingSub.TrialStart = &trialStart
	}
	if stripeSubscription.TrialEnd > 0 {
		trialEnd := time.Unix(stripeSubscription.TrialEnd, 0)
		existingSub.TrialEnd = &trialEnd
	}

	// Handle cancellation
	if stripeSubscription.CanceledAt > 0 && existingSub.CanceledAt == nil {
		canceledAt := time.Unix(stripeSubscription.CanceledAt, 0)
		existingSub.CanceledAt = &canceledAt
	}

	// Update in database
	if err := h.subscriptionRepo.UpdateSubscription(ctx, existingSub); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log appropriate event
	eventType := subscription.EventTypeActivated
	if previousState.Status != existingSub.Status {
		if existingSub.Status == subscription.StatusCanceled {
			eventType = subscription.EventTypeCanceled
		} else if existingSub.Status == subscription.StatusPastDue {
			eventType = subscription.EventTypePastDue
		} else if existingSub.Status == subscription.StatusExpired {
			eventType = subscription.EventTypeExpired
		}
	}

	if err := h.subscriptionRepo.LogSubscriptionEvent(ctx, existingSub.ID, eventType, previousState, existingSub, "Updated via Stripe webhook"); err != nil {
		h.logger.Warn("Failed to log subscription event", zap.Error(err))
	}

	return nil
}

// handleSubscriptionDeleted handles subscription.deleted events
func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event *stripe.Event) error {
	var stripeSubscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &stripeSubscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	h.logger.Info("Subscription deleted",
		zap.String("subscription_id", stripeSubscription.ID),
	)

	// Get existing subscription
	existingSub, err := h.subscriptionRepo.GetSubscriptionByStripeID(ctx, stripeSubscription.ID)
	if err != nil {
		return fmt.Errorf("failed to get existing subscription: %w", err)
	}

	// Store previous state
	previousState := *existingSub

	// Mark as canceled/expired
	existingSub.Status = subscription.StatusCanceled
	existingSub.UpdatedAt = time.Now()
	if existingSub.CanceledAt == nil {
		now := time.Now()
		existingSub.CanceledAt = &now
	}

	if err := h.subscriptionRepo.UpdateSubscription(ctx, existingSub); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log event
	if err := h.subscriptionRepo.LogSubscriptionEvent(ctx, existingSub.ID, subscription.EventTypeCanceled, previousState, existingSub, "Deleted via Stripe webhook"); err != nil {
		h.logger.Warn("Failed to log subscription event", zap.Error(err))
	}

	return nil
}

// handleSubscriptionTrialWillEnd handles subscription.trial_will_end events
func (h *WebhookHandler) handleSubscriptionTrialWillEnd(ctx context.Context, event *stripe.Event) error {
	var stripeSubscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &stripeSubscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	h.logger.Info("Subscription trial will end",
		zap.String("subscription_id", stripeSubscription.ID),
		zap.Time("trial_end", time.Unix(stripeSubscription.TrialEnd, 0)),
	)

	// TODO: Send notification to user about trial ending
	// Example:
	// return h.notificationService.SendTrialEndingNotification(ctx, userID, trialEndDate)

	return nil
}

// handleInvoicePaymentSucceeded handles invoice.payment_succeeded events
func (h *WebhookHandler) handleInvoicePaymentSucceeded(ctx context.Context, event *stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %w", err)
	}

	h.logger.Info("Invoice payment succeeded",
		zap.String("invoice_id", invoice.ID),
		zap.String("subscription_id", invoice.Subscription.ID),
		zap.Int64("amount_paid", invoice.AmountPaid),
	)

	// Get subscription to find user ID
	if invoice.Subscription == nil {
		h.logger.Warn("Invoice has no subscription attached, skipping payment record")
		return nil
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByStripeID(ctx, invoice.Subscription.ID)
	if err != nil {
		return fmt.Errorf("failed to get subscription for payment: %w", err)
	}

	// Create payment record
	paidAt := time.Unix(invoice.StatusTransitions.PaidAt, 0)
	payment := &subscription.Payment{
		ID:             uuid.New(),
		UserID:         sub.UserID,
		SubscriptionID: &sub.ID,
		Amount:         int(invoice.AmountPaid),
		Currency:       string(invoice.Currency),
		Status:         subscription.PaymentStatusSucceeded,
		PaidAt:         &paidAt,
		CreatedAt:      time.Now(),
	}

	if invoice.PaymentIntent != nil {
		payment.StripePaymentIntentID = &invoice.PaymentIntent.ID
	}
	if invoice.Charge != nil {
		payment.StripeChargeID = &invoice.Charge.ID
	}

	desc := fmt.Sprintf("Payment for %s subscription", sub.PlanID)
	payment.Description = &desc

	if err := h.subscriptionRepo.CreatePayment(ctx, payment); err != nil {
		return fmt.Errorf("failed to create payment record: %w", err)
	}

	// Log subscription event
	if err := h.subscriptionRepo.LogSubscriptionEvent(ctx, sub.ID, subscription.EventTypePaymentSucceeded, nil, payment, "Payment succeeded"); err != nil {
		h.logger.Warn("Failed to log subscription event", zap.Error(err))
	}

	return nil
}

// handleInvoicePaymentFailed handles invoice.payment_failed events
func (h *WebhookHandler) handleInvoicePaymentFailed(ctx context.Context, event *stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %w", err)
	}

	h.logger.Info("Invoice payment failed",
		zap.String("invoice_id", invoice.ID),
		zap.String("subscription_id", invoice.Subscription.ID),
	)

	// Get subscription
	if invoice.Subscription == nil {
		h.logger.Warn("Invoice has no subscription attached, skipping")
		return nil
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByStripeID(ctx, invoice.Subscription.ID)
	if err != nil {
		return fmt.Errorf("failed to get subscription for failed payment: %w", err)
	}

	// Store previous state
	previousState := *sub

	// Update subscription to past_due status
	sub.Status = subscription.StatusPastDue
	sub.UpdatedAt = time.Now()

	if err := h.subscriptionRepo.UpdateSubscription(ctx, sub); err != nil {
		return fmt.Errorf("failed to update subscription status: %w", err)
	}

	// Create failed payment record
	failedAt := time.Now()
	failureReason := "Payment failed"
	if invoice.LastFinalizationError != nil && invoice.LastFinalizationError.Code != "" {
		failureReason = string(invoice.LastFinalizationError.Code)
	}

	payment := &subscription.Payment{
		ID:             uuid.New(),
		UserID:         sub.UserID,
		SubscriptionID: &sub.ID,
		Amount:         int(invoice.AmountDue),
		Currency:       string(invoice.Currency),
		Status:         subscription.PaymentStatusFailed,
		FailedAt:       &failedAt,
		FailureReason:  &failureReason,
		CreatedAt:      time.Now(),
	}

	if invoice.PaymentIntent != nil {
		payment.StripePaymentIntentID = &invoice.PaymentIntent.ID
	}

	desc := fmt.Sprintf("Failed payment for %s subscription", sub.PlanID)
	payment.Description = &desc

	if err := h.subscriptionRepo.CreatePayment(ctx, payment); err != nil {
		h.logger.Warn("Failed to create failed payment record", zap.Error(err))
	}

	// Log event
	if err := h.subscriptionRepo.LogSubscriptionEvent(ctx, sub.ID, subscription.EventTypePaymentFailed, previousState, sub, failureReason); err != nil {
		h.logger.Warn("Failed to log subscription event", zap.Error(err))
	}

	// TODO: Send notification to user about payment failure
	// This should be handled by subtask 10.7

	return nil
}

// handlePaymentIntentSucceeded handles payment_intent.succeeded events
func (h *WebhookHandler) handlePaymentIntentSucceeded(ctx context.Context, event *stripe.Event) error {
	var paymentIntent stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
		return fmt.Errorf("failed to unmarshal payment intent: %w", err)
	}

	h.logger.Info("Payment intent succeeded",
		zap.String("payment_intent_id", paymentIntent.ID),
		zap.Int64("amount", paymentIntent.Amount),
	)

	// TODO: Process successful payment
	// This might be a one-time payment or subscription payment

	return nil
}

// handlePaymentIntentFailed handles payment_intent.payment_failed events
func (h *WebhookHandler) handlePaymentIntentFailed(ctx context.Context, event *stripe.Event) error {
	var paymentIntent stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &paymentIntent); err != nil {
		return fmt.Errorf("failed to unmarshal payment intent: %w", err)
	}

	var failureMessage string
	if paymentIntent.LastPaymentError != nil {
		failureMessage = string(paymentIntent.LastPaymentError.DeclineCode)
		if paymentIntent.LastPaymentError.Code != "" {
			failureMessage = string(paymentIntent.LastPaymentError.Code)
		}
	}

	h.logger.Info("Payment intent failed",
		zap.String("payment_intent_id", paymentIntent.ID),
		zap.String("failure_message", failureMessage),
	)

	// TODO: Handle payment failure

	return nil
}

// handlePaymentMethodAttached handles payment_method.attached events
func (h *WebhookHandler) handlePaymentMethodAttached(ctx context.Context, event *stripe.Event) error {
	var paymentMethod stripe.PaymentMethod
	if err := json.Unmarshal(event.Data.Raw, &paymentMethod); err != nil {
		return fmt.Errorf("failed to unmarshal payment method: %w", err)
	}

	h.logger.Info("Payment method attached",
		zap.String("payment_method_id", paymentMethod.ID),
		zap.String("customer_id", paymentMethod.Customer.ID),
	)

	// TODO: Update customer's default payment method if needed

	return nil
}

// handleCheckoutSessionCompleted handles checkout.session.completed events
func (h *WebhookHandler) handleCheckoutSessionCompleted(ctx context.Context, event *stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("failed to unmarshal checkout session: %w", err)
	}

	h.logger.Info("Checkout session completed",
		zap.String("session_id", session.ID),
		zap.String("customer_id", session.Customer.ID),
		zap.String("subscription_id", session.Subscription.ID),
		zap.String("payment_status", string(session.PaymentStatus)),
	)

	// Extract user ID from metadata
	userIDStr, ok := session.Metadata["user_id"]
	if !ok {
		return fmt.Errorf("user_id not found in session metadata")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	planID, ok := session.Metadata["plan_id"]
	if !ok {
		planID = "premium" // Default plan
	}

	// TODO: Create or update subscription in database
	// The subscription.created webhook will also fire, so you might want to handle this differently
	// to avoid duplicate processing

	h.logger.Info("Checkout completed for user",
		zap.String("user_id", userID.String()),
		zap.String("plan_id", planID),
	)

	return nil
}
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v79"
	"go.uber.org/zap"
)

// WebhookHandler handles Stripe webhook events
type WebhookHandler struct {
	service *Service
	logger  *zap.Logger
	// Add your subscription repository here
	// subscriptionRepo subscription.Repository
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(service *Service, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		service: service,
		logger:  logger,
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

	// TODO: Create subscription record in database
	// Example:
	// sub := &subscription.Subscription{
	//     ID:                   uuid.New(),
	//     UserID:               userID,
	//     PlanID:               planID,
	//     Status:               MapStripeStatusToSubscriptionStatus(stripeSubscription.Status),
	//     CurrentPeriodStart:   time.Unix(stripeSubscription.CurrentPeriodStart, 0),
	//     CurrentPeriodEnd:     time.Unix(stripeSubscription.CurrentPeriodEnd, 0),
	//     StripeCustomerID:     &stripeSubscription.Customer.ID,
	//     StripeSubscriptionID: &stripeSubscription.ID,
	// }
	// return h.subscriptionRepo.Create(ctx, sub)

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

	// TODO: Update subscription record in database
	// Example:
	// return h.subscriptionRepo.UpdateByStripeID(ctx, stripeSubscription.ID, &subscription.UpdateSubscriptionParams{
	//     Status:            MapStripeStatusToSubscriptionStatus(stripeSubscription.Status),
	//     CancelAtPeriodEnd: &stripeSubscription.CancelAtPeriodEnd,
	// })

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

	// TODO: Update subscription status to canceled in database
	// Example:
	// status := subscription.StatusCanceled
	// return h.subscriptionRepo.UpdateByStripeID(ctx, stripeSubscription.ID, &subscription.UpdateSubscriptionParams{
	//     Status: &status,
	// })

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

	// TODO: Create payment record in database
	// Example:
	// payment := &subscription.Payment{
	//     ID:                    uuid.New(),
	//     UserID:                userID,
	//     SubscriptionID:        subscriptionID,
	//     Amount:                int(invoice.AmountPaid),
	//     Currency:              string(invoice.Currency),
	//     Status:                subscription.PaymentStatusSucceeded,
	//     StripePaymentIntentID: &invoice.PaymentIntent.ID,
	//     PaidAt:                &paidAt,
	// }
	// return h.paymentRepo.Create(ctx, payment)

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

	// TODO: Update subscription status and send notification
	// Example:
	// status := subscription.StatusPastDue
	// err := h.subscriptionRepo.UpdateByStripeID(ctx, invoice.Subscription.ID, &subscription.UpdateSubscriptionParams{
	//     Status: &status,
	// })
	// if err != nil {
	//     return err
	// }
	// return h.notificationService.SendPaymentFailedNotification(ctx, userID)

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
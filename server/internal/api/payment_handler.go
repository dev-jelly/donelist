package api

import (
	"bytes"
	"io"
	"net/http"

	"github.com/dev-jelly/donelist/internal/payment"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// PaymentHandler handles payment-related HTTP requests
type PaymentHandler struct {
	paymentManager *payment.Manager
	logger         *zap.Logger
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(paymentManager *payment.Manager, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{
		paymentManager: paymentManager,
		logger:         logger,
	}
}

// RegisterRoutes registers payment routes
func (h *PaymentHandler) RegisterRoutes(router *gin.RouterGroup) {
	if h.paymentManager == nil || !h.paymentManager.IsEnabled() {
		h.logger.Warn("Payment manager not enabled, skipping payment route registration")
		return
	}

	// Public webhook endpoint (no auth required)
	router.POST("/stripe/webhook", h.handleStripeWebhook)

	// Protected endpoints (require authentication)
	protected := router.Group("/")
	// Add your auth middleware here
	// protected.Use(authMiddleware)

	protected.POST("/checkout/session", h.createCheckoutSession)
	protected.POST("/subscription/cancel", h.cancelSubscription)
	protected.GET("/subscription/status", h.getSubscriptionStatus)
	protected.POST("/payment-method/attach", h.attachPaymentMethod)
}

// handleStripeWebhook processes incoming Stripe webhooks
func (h *PaymentHandler) handleStripeWebhook(c *gin.Context) {
	// Read the request body
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("Failed to read webhook payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Restore the body for potential re-reading
	c.Request.Body = io.NopCloser(bytes.NewBuffer(payload))

	// Get the Stripe signature header
	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		h.logger.Warn("Missing Stripe signature header")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing signature header"})
		return
	}

	// Verify the webhook signature and construct the event
	event, err := h.paymentManager.Service.VerifyWebhookSignature(payload, signature)
	if err != nil {
		h.logger.Error("Failed to verify webhook signature", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid signature"})
		return
	}

	// Process the event
	if err := h.paymentManager.WebhookHandler.HandleEvent(c.Request.Context(), event); err != nil {
		h.logger.Error("Failed to handle webhook event",
			zap.Error(err),
			zap.String("event_type", string(event.Type)),
			zap.String("event_id", event.ID),
		)
		// Return 200 to prevent Stripe from retrying
		// Log the error for investigation
		c.JSON(http.StatusOK, gin.H{"received": true, "error": "Processing failed"})
		return
	}

	h.logger.Info("Successfully processed webhook event",
		zap.String("event_type", string(event.Type)),
		zap.String("event_id", event.ID),
	)

	c.JSON(http.StatusOK, gin.H{"received": true})
}

// createCheckoutSession creates a new Stripe Checkout session
func (h *PaymentHandler) createCheckoutSession(c *gin.Context) {
	var req struct {
		PlanID          string `json:"plan_id" binding:"required"`
		BillingInterval string `json:"billing_interval" binding:"required,oneof=month year"`
		SuccessURL      string `json:"success_url" binding:"required,url"`
		CancelURL       string `json:"cancel_url" binding:"required,url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Get user ID from context (after auth middleware)
	// userID := getUserIDFromContext(c)
	// email := getUserEmailFromContext(c)

	// For now, return not implemented
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Checkout session creation not yet implemented"})

	// Example implementation:
	// params := &payment.CheckoutSessionParams{
	//     UserID:          userID,
	//     CustomerEmail:   email,
	//     PlanID:          req.PlanID,
	//     PlanName:        plan.Name,
	//     PlanDescription: plan.Description,
	//     Amount:          plan.GetPrice(subscription.BillingInterval(req.BillingInterval)),
	//     Currency:        plan.Currency,
	//     BillingInterval: subscription.BillingInterval(req.BillingInterval),
	//     TrialDays:       plan.TrialDays,
	//     SuccessURL:      req.SuccessURL,
	//     CancelURL:       req.CancelURL,
	// }
	//
	// session, err := h.paymentManager.Service.CreateCheckoutSession(c.Request.Context(), params)
	// if err != nil {
	//     h.logger.Error("Failed to create checkout session", zap.Error(err))
	//     c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create checkout session"})
	//     return
	// }
	//
	// c.JSON(http.StatusOK, gin.H{
	//     "session_id": session.ID,
	//     "url":        session.URL,
	// })
}

// cancelSubscription cancels a user's subscription
func (h *PaymentHandler) cancelSubscription(c *gin.Context) {
	var req struct {
		Immediately bool `json:"immediately"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Get user's subscription ID from database
	// userID := getUserIDFromContext(c)
	// subscription := getSubscriptionByUserID(userID)

	c.JSON(http.StatusNotImplemented, gin.H{"error": "Subscription cancellation not yet implemented"})

	// Example implementation:
	// if subscription.StripeSubscriptionID == nil {
	//     c.JSON(http.StatusBadRequest, gin.H{"error": "No active subscription"})
	//     return
	// }
	//
	// stripeSubscription, err := h.paymentManager.Service.CancelSubscription(
	//     c.Request.Context(),
	//     *subscription.StripeSubscriptionID,
	//     req.Immediately,
	// )
	// if err != nil {
	//     h.logger.Error("Failed to cancel subscription", zap.Error(err))
	//     c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel subscription"})
	//     return
	// }
	//
	// c.JSON(http.StatusOK, gin.H{
	//     "status": string(stripeSubscription.Status),
	//     "cancel_at_period_end": stripeSubscription.CancelAtPeriodEnd,
	//     "current_period_end": stripeSubscription.CurrentPeriodEnd,
	// })
}

// getSubscriptionStatus returns the current subscription status
func (h *PaymentHandler) getSubscriptionStatus(c *gin.Context) {
	// TODO: Get user's subscription from database
	// userID := getUserIDFromContext(c)
	// subscription := getSubscriptionByUserID(userID)

	c.JSON(http.StatusNotImplemented, gin.H{"error": "Subscription status not yet implemented"})

	// Example implementation:
	// if subscription == nil {
	//     c.JSON(http.StatusOK, gin.H{
	//         "has_subscription": false,
	//         "status": "none",
	//     })
	//     return
	// }
	//
	// c.JSON(http.StatusOK, gin.H{
	//     "has_subscription": true,
	//     "status": string(subscription.Status),
	//     "plan_id": subscription.PlanID,
	//     "current_period_end": subscription.CurrentPeriodEnd,
	//     "cancel_at_period_end": subscription.CancelAtPeriodEnd,
	//     "is_active": subscription.IsActive(),
	//     "is_trial": subscription.IsTrial(),
	//     "days_until_expiry": subscription.DaysUntilExpiry(),
	// })
}

// attachPaymentMethod attaches a payment method to the customer
func (h *PaymentHandler) attachPaymentMethod(c *gin.Context) {
	var req struct {
		PaymentMethodID string `json:"payment_method_id" binding:"required"`
		SetAsDefault    bool   `json:"set_as_default"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Get user's Stripe customer ID from database
	// userID := getUserIDFromContext(c)
	// user := getUserByID(userID)
	// customerID := user.StripeCustomerID

	c.JSON(http.StatusNotImplemented, gin.H{"error": "Payment method attachment not yet implemented"})

	// Example implementation:
	// if customerID == "" {
	//     // Create customer first
	//     customer, err := h.paymentManager.Service.CreateCustomer(
	//         c.Request.Context(),
	//         userID,
	//         user.Email,
	//         user.Name,
	//     )
	//     if err != nil {
	//         h.logger.Error("Failed to create customer", zap.Error(err))
	//         c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create customer"})
	//         return
	//     }
	//     customerID = customer.ID
	//     // Save customer ID to user record
	// }
	//
	// paymentMethod, err := h.paymentManager.Service.AttachPaymentMethod(
	//     c.Request.Context(),
	//     req.PaymentMethodID,
	//     customerID,
	// )
	// if err != nil {
	//     h.logger.Error("Failed to attach payment method", zap.Error(err))
	//     c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach payment method"})
	//     return
	// }
	//
	// if req.SetAsDefault {
	//     err = h.paymentManager.Service.SetDefaultPaymentMethod(
	//         c.Request.Context(),
	//         customerID,
	//         req.PaymentMethodID,
	//     )
	//     if err != nil {
	//         h.logger.Error("Failed to set default payment method", zap.Error(err))
	//         c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set default payment method"})
	//         return
	//     }
	// }
	//
	// c.JSON(http.StatusOK, gin.H{
	//     "payment_method_id": paymentMethod.ID,
	//     "type": string(paymentMethod.Type),
	//     "last4": paymentMethod.Card.Last4,
	//     "brand": string(paymentMethod.Card.Brand),
	// })
}
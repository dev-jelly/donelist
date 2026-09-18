package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/dev-jelly/donelist/internal/payment"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/dev-jelly/donelist/internal/user"
	"github.com/google/uuid"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// PaymentHandler handles payment-related HTTP requests
type PaymentHandler struct {
	paymentManager   *payment.Manager
	subscriptionRepo *subscription.Repository
	userRepo         *user.Repository
	logger           *zap.Logger
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(paymentManager *payment.Manager, subscriptionRepo *subscription.Repository, userRepo *user.Repository, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{
		paymentManager:   paymentManager,
		subscriptionRepo: subscriptionRepo,
		userRepo:         userRepo,
		logger:           logger,
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
	protected.POST("/billing-portal", h.createBillingPortal)
	protected.POST("/subscription/cancel", h.cancelSubscription)
	protected.POST("/subscription/reactivate", h.reactivateSubscription)
	protected.GET("/subscription/status", h.getSubscriptionStatus)
	protected.POST("/payment-method/attach", h.attachPaymentMethod)
	protected.GET("/payment-methods", h.listPaymentMethods)
	protected.DELETE("/payment-method/:id", h.detachPaymentMethod)
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

// getUserIDFromContext extracts user ID from the Gin context
func getUserIDFromContext(c *gin.Context) (uuid.UUID, error) {
	userIDInterface, exists := c.Get("userID")
	if !exists {
		return uuid.UUID{}, fmt.Errorf("user ID not found in context")
	}

	userID, ok := userIDInterface.(uuid.UUID)
	if !ok {
		userIDStr, ok := userIDInterface.(string)
		if !ok {
			return uuid.UUID{}, fmt.Errorf("invalid user ID type in context")
		}
		return uuid.Parse(userIDStr)
	}

	return userID, nil
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

	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	user, err := h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err), zap.String("user_id", userID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user details"})
		return
	}

	planID := subscription.PlanID(req.PlanID)
	plan, exists := subscription.GetPlan(planID)
	if !exists || !plan.Active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or inactive plan"})
		return
	}

	billingInterval := subscription.BillingInterval(req.BillingInterval)
	amount := plan.GetPrice(billingInterval)

	existingSub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err == nil && existingSub != nil && existingSub.IsActive() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User already has an active subscription"})
		return
	}

	var customerID string
	if existingSub != nil && existingSub.StripeCustomerID != nil {
		customerID = *existingSub.StripeCustomerID
	} else {
		customer, err := h.paymentManager.Service.CreateCustomer(
			c.Request.Context(),
			userID,
			user.Email,
			func() string {
				if user.DisplayName != nil {
					return *user.DisplayName
				}
				return user.Email
			}(),
		)
		if err != nil {
			h.logger.Error("Failed to create Stripe customer", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create customer"})
			return
		}
		customerID = customer.ID
	}

	params := &payment.CheckoutSessionParams{
		UserID:          userID,
		CustomerID:      customerID,
		CustomerEmail:   user.Email,
		PlanID:          string(planID),
		PlanName:        plan.Name,
		PlanDescription: plan.Description,
		Amount:          amount,
		Currency:        plan.Currency,
		BillingInterval: billingInterval,
		TrialDays:       plan.TrialDays,
		SuccessURL:      req.SuccessURL,
		CancelURL:       req.CancelURL,
	}

	session, err := h.paymentManager.Service.CreateCheckoutSession(c.Request.Context(), params)
	if err != nil {
		h.logger.Error("Failed to create checkout session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create checkout session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"url":        session.URL,
	})
}

// createBillingPortal creates a Stripe billing portal session
func (h *PaymentHandler) createBillingPortal(c *gin.Context) {
	var req struct {
		ReturnURL string `json:"return_url" binding:"required,url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err != nil || sub == nil || sub.StripeCustomerID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No subscription found"})
		return
	}

	session, err := h.paymentManager.Service.CreateBillingPortalSession(
		c.Request.Context(),
		*sub.StripeCustomerID,
		req.ReturnURL,
	)
	if err != nil {
		h.logger.Error("Failed to create billing portal session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create billing portal session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": session.URL})
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

	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err != nil || sub == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active subscription found"})
		return
	}

	if sub.StripeSubscriptionID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Stripe subscription ID"})
		return
	}

	stripeSubscription, err := h.paymentManager.Service.CancelSubscription(
		c.Request.Context(),
		*sub.StripeSubscriptionID,
		req.Immediately,
	)
	if err != nil {
		h.logger.Error("Failed to cancel subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":               string(stripeSubscription.Status),
		"cancel_at_period_end": stripeSubscription.CancelAtPeriodEnd,
		"current_period_end":   stripeSubscription.CurrentPeriodEnd,
	})
}

// reactivateSubscription reactivates a canceled subscription
func (h *PaymentHandler) reactivateSubscription(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err != nil || sub == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No subscription found"})
		return
	}

	if sub.StripeSubscriptionID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Stripe subscription ID"})
		return
	}

	if !sub.CancelAtPeriodEnd {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subscription is not scheduled for cancellation"})
		return
	}

	stripeSubscription, err := h.paymentManager.Service.ReactivateSubscription(
		c.Request.Context(),
		*sub.StripeSubscriptionID,
	)
	if err != nil {
		h.logger.Error("Failed to reactivate subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reactivate subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":               string(stripeSubscription.Status),
		"cancel_at_period_end": stripeSubscription.CancelAtPeriodEnd,
		"current_period_end":   stripeSubscription.CurrentPeriodEnd,
	})
}

// getSubscriptionStatus returns the current subscription status
func (h *PaymentHandler) getSubscriptionStatus(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err != nil || sub == nil {
		c.JSON(http.StatusOK, gin.H{
			"has_subscription": false,
			"status":           "none",
		})
		return
	}

	payments, err := h.subscriptionRepo.ListPayments(c.Request.Context(), &userID, nil, 5)
	if err != nil {
		h.logger.Warn("Failed to get payment history", zap.Error(err))
		payments = []*subscription.Payment{}
	}

	c.JSON(http.StatusOK, gin.H{
		"has_subscription":      true,
		"status":                string(sub.Status),
		"plan_id":               sub.PlanID,
		"current_period_start":  sub.CurrentPeriodStart,
		"current_period_end":    sub.CurrentPeriodEnd,
		"cancel_at_period_end":  sub.CancelAtPeriodEnd,
		"canceled_at":           sub.CanceledAt,
		"is_active":             sub.IsActive(),
		"is_trial":              sub.IsTrial(),
		"is_past_due":           sub.IsPastDue(),
		"days_until_expiry":     sub.DaysUntilExpiry(),
		"trial_start":           sub.TrialStart,
		"trial_end":             sub.TrialEnd,
		"recent_payments":       payments,
	})
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

	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	user, err := h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user details"})
		return
	}

	var customerID string
	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err == nil && sub != nil && sub.StripeCustomerID != nil {
		customerID = *sub.StripeCustomerID
	} else {
		customer, err := h.paymentManager.Service.CreateCustomer(
			c.Request.Context(),
			userID,
			user.Email,
			func() string {
				if user.DisplayName != nil {
					return *user.DisplayName
				}
				return user.Email
			}(),
		)
		if err != nil {
			h.logger.Error("Failed to create Stripe customer", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create customer"})
			return
		}
		customerID = customer.ID
	}

	paymentMethod, err := h.paymentManager.Service.AttachPaymentMethod(
		c.Request.Context(),
		req.PaymentMethodID,
		customerID,
	)
	if err != nil {
		h.logger.Error("Failed to attach payment method", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to attach payment method"})
		return
	}

	if req.SetAsDefault {
		err = h.paymentManager.Service.SetDefaultPaymentMethod(
			c.Request.Context(),
			customerID,
			req.PaymentMethodID,
		)
		if err != nil {
			h.logger.Error("Failed to set default payment method", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set default payment method"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_method_id": paymentMethod.ID,
		"type":              string(paymentMethod.Type),
		"card": gin.H{
			"last4":     paymentMethod.Card.Last4,
			"brand":     string(paymentMethod.Card.Brand),
			"exp_month": paymentMethod.Card.ExpMonth,
			"exp_year":  paymentMethod.Card.ExpYear,
		},
	})
}

// listPaymentMethods lists payment methods for the user
func (h *PaymentHandler) listPaymentMethods(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err != nil || sub == nil || sub.StripeCustomerID == nil {
		c.JSON(http.StatusOK, gin.H{"payment_methods": []interface{}{}})
		return
	}

	methods, err := h.paymentManager.Service.ListCustomerPaymentMethods(
		c.Request.Context(),
		*sub.StripeCustomerID,
	)
	if err != nil {
		h.logger.Error("Failed to list payment methods", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list payment methods"})
		return
	}

	formattedMethods := make([]gin.H, len(methods))
	for i, method := range methods {
		formattedMethods[i] = gin.H{
			"id":   method.ID,
			"type": string(method.Type),
			"card": gin.H{
				"last4":     method.Card.Last4,
				"brand":     string(method.Card.Brand),
				"exp_month": method.Card.ExpMonth,
				"exp_year":  method.Card.ExpYear,
			},
		}
	}

	c.JSON(http.StatusOK, gin.H{"payment_methods": formattedMethods})
}

// detachPaymentMethod detaches a payment method from the customer
func (h *PaymentHandler) detachPaymentMethod(c *gin.Context) {
	paymentMethodID := c.Param("id")
	if paymentMethodID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment method ID required"})
		return
	}

	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sub, err := h.subscriptionRepo.GetSubscriptionByUserID(c.Request.Context(), userID)
	if err != nil || sub == nil || sub.StripeCustomerID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No subscription found"})
		return
	}

	err = h.paymentManager.Service.DetachPaymentMethod(c.Request.Context(), paymentMethodID)
	if err != nil {
		h.logger.Error("Failed to detach payment method", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to detach payment method"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment method detached successfully"})
}
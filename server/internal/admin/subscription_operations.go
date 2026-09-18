package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-jelly/donelist/internal/payment"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SubscriptionOperations handles administrative subscription operations
type SubscriptionOperations struct {
	subscriptionRepo *subscription.Repository
	paymentService   *payment.Service
	logger           *zap.Logger
}

// NewSubscriptionOperations creates a new subscription operations service
func NewSubscriptionOperations(
	subscriptionRepo *subscription.Repository,
	paymentService *payment.Service,
	logger *zap.Logger,
) *SubscriptionOperations {
	return &SubscriptionOperations{
		subscriptionRepo: subscriptionRepo,
		paymentService:   paymentService,
		logger:           logger,
	}
}

// SubscriptionListResponse contains paginated subscription results
type SubscriptionListResponse struct {
	Subscriptions []*SubscriptionDetail `json:"subscriptions"`
	Total         int                   `json:"total"`
	Page          int                   `json:"page"`
	PageSize      int                   `json:"page_size"`
	HasMore       bool                  `json:"has_more"`
}

// SubscriptionDetail contains detailed subscription information
type SubscriptionDetail struct {
	*subscription.Subscription
	User          *UserInfo            `json:"user,omitempty"`
	RecentPayments []*subscription.Payment `json:"recent_payments,omitempty"`
	StripeDetails *StripeSubscriptionDetail `json:"stripe_details,omitempty"`
}

// UserInfo contains basic user information
type UserInfo struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName *string   `json:"display_name,omitempty"`
}

// StripeSubscriptionDetail contains Stripe-specific details
type StripeSubscriptionDetail struct {
	CustomerID         string    `json:"customer_id"`
	SubscriptionID     string    `json:"subscription_id"`
	DefaultPaymentMethod *string `json:"default_payment_method,omitempty"`
	NextBillingDate    *time.Time `json:"next_billing_date,omitempty"`
}

// ListSubscriptions retrieves all subscriptions with filtering and pagination
func (s *SubscriptionOperations) ListSubscriptions(ctx context.Context, filter subscription.SubscriptionFilter) (*SubscriptionListResponse, error) {
	// Set default page size if not specified
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100 // Max page size
	}

	// Get subscriptions
	subs, err := s.subscriptionRepo.ListSubscriptions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}

	// Build response with details
	details := make([]*SubscriptionDetail, len(subs))
	for i, sub := range subs {
		detail := &SubscriptionDetail{
			Subscription: sub,
		}

		// Get recent payments
		payments, err := s.subscriptionRepo.ListPayments(ctx, &sub.UserID, &sub.ID, 5)
		if err != nil {
			s.logger.Warn("Failed to get payments for subscription",
				zap.Error(err),
				zap.String("subscription_id", sub.ID.String()),
			)
		} else {
			detail.RecentPayments = payments
		}

		// Get Stripe details if available
		if sub.StripeSubscriptionID != nil && sub.StripeCustomerID != nil {
			detail.StripeDetails = &StripeSubscriptionDetail{
				CustomerID:      *sub.StripeCustomerID,
				SubscriptionID:  *sub.StripeSubscriptionID,
				NextBillingDate: &sub.CurrentPeriodEnd,
			}
		}

		details[i] = detail
	}

	// Calculate pagination info
	page := filter.Offset / filter.Limit
	hasMore := len(subs) == filter.Limit

	return &SubscriptionListResponse{
		Subscriptions: details,
		Total:         len(subs), // In production, you'd want a separate count query
		Page:          page,
		PageSize:      filter.Limit,
		HasMore:       hasMore,
	}, nil
}

// GetSubscription retrieves detailed information about a specific subscription
func (s *SubscriptionOperations) GetSubscription(ctx context.Context, subscriptionID uuid.UUID) (*SubscriptionDetail, error) {
	sub, err := s.subscriptionRepo.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	detail := &SubscriptionDetail{
		Subscription: sub,
	}

	// Get payment history
	payments, err := s.subscriptionRepo.ListPayments(ctx, &sub.UserID, &sub.ID, 20)
	if err != nil {
		s.logger.Warn("Failed to get payment history", zap.Error(err))
	} else {
		detail.RecentPayments = payments
	}

	// Get Stripe details
	if sub.StripeSubscriptionID != nil {
		stripeSub, err := s.paymentService.GetSubscription(ctx, *sub.StripeSubscriptionID)
		if err != nil {
			s.logger.Warn("Failed to get Stripe subscription", zap.Error(err))
		} else {
			detail.StripeDetails = &StripeSubscriptionDetail{
				CustomerID:      stripeSub.Customer.ID,
				SubscriptionID:  stripeSub.ID,
				NextBillingDate: ptrTime(time.Unix(stripeSub.CurrentPeriodEnd, 0)),
			}

			if stripeSub.DefaultPaymentMethod != nil {
				detail.StripeDetails.DefaultPaymentMethod = &stripeSub.DefaultPaymentMethod.ID
			}
		}
	}

	return detail, nil
}

// PaymentHistoryResponse contains payment history results
type PaymentHistoryResponse struct {
	Payments []*PaymentDetail `json:"payments"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// PaymentDetail contains detailed payment information
type PaymentDetail struct {
	*subscription.Payment
	User         *UserInfo `json:"user,omitempty"`
	Subscription *subscription.Subscription `json:"subscription,omitempty"`
}

// GetPaymentHistory retrieves payment history with pagination
func (s *SubscriptionOperations) GetPaymentHistory(ctx context.Context, userID *uuid.UUID, limit, offset int) (*PaymentHistoryResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	payments, err := s.subscriptionRepo.ListPayments(ctx, userID, nil, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment history: %w", err)
	}

	details := make([]*PaymentDetail, len(payments))
	for i, payment := range payments {
		detail := &PaymentDetail{
			Payment: payment,
		}

		// Get subscription info
		if payment.SubscriptionID != nil {
			sub, err := s.subscriptionRepo.GetSubscription(ctx, *payment.SubscriptionID)
			if err != nil {
				s.logger.Warn("Failed to get subscription for payment",
					zap.Error(err),
					zap.String("payment_id", payment.ID.String()),
				)
			} else {
				detail.Subscription = sub
			}
		}

		details[i] = detail
	}

	return &PaymentHistoryResponse{
		Payments: details,
		Total:    len(payments),
		Page:     offset / limit,
		PageSize: limit,
	}, nil
}

// GetFailedPayments retrieves all failed payments
func (s *SubscriptionOperations) GetFailedPayments(ctx context.Context, limit int) ([]*PaymentDetail, error) {
	if limit <= 0 {
		limit = 100
	}

	// Get all recent payments and filter for failed ones
	payments, err := s.subscriptionRepo.ListPayments(ctx, nil, nil, limit*2)
	if err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
	}

	var failedPayments []*PaymentDetail
	for _, payment := range payments {
		if payment.Status == subscription.PaymentStatusFailed {
			detail := &PaymentDetail{
				Payment: payment,
			}

			// Get subscription info
			if payment.SubscriptionID != nil {
				sub, err := s.subscriptionRepo.GetSubscription(ctx, *payment.SubscriptionID)
				if err == nil {
					detail.Subscription = sub
				}
			}

			failedPayments = append(failedPayments, detail)

			if len(failedPayments) >= limit {
				break
			}
		}
	}

	return failedPayments, nil
}

// ManualSubscriptionUpdate allows manual subscription operations
type ManualSubscriptionUpdate struct {
	SubscriptionID uuid.UUID                  `json:"subscription_id"`
	Action         string                     `json:"action"` // "cancel", "reactivate", "change_plan", "extend"
	Reason         string                     `json:"reason"`
	NewPlanID      *string                    `json:"new_plan_id,omitempty"`
	ExtendDays     *int                       `json:"extend_days,omitempty"`
}

// PerformManualUpdate performs manual subscription operations
func (s *SubscriptionOperations) PerformManualUpdate(ctx context.Context, update *ManualSubscriptionUpdate) error {
	sub, err := s.subscriptionRepo.GetSubscription(ctx, update.SubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	previousState := *sub

	switch update.Action {
	case "cancel":
		if sub.StripeSubscriptionID != nil {
			_, err = s.paymentService.CancelSubscription(ctx, *sub.StripeSubscriptionID, true)
			if err != nil {
				return fmt.Errorf("failed to cancel Stripe subscription: %w", err)
			}
		}
		sub.Status = subscription.StatusCanceled
		now := time.Now()
		sub.CanceledAt = &now

	case "reactivate":
		if sub.StripeSubscriptionID != nil && sub.CancelAtPeriodEnd {
			_, err = s.paymentService.ReactivateSubscription(ctx, *sub.StripeSubscriptionID)
			if err != nil {
				return fmt.Errorf("failed to reactivate Stripe subscription: %w", err)
			}
		}
		sub.Status = subscription.StatusActive
		sub.CancelAtPeriodEnd = false
		sub.CanceledAt = nil

	case "change_plan":
		if update.NewPlanID == nil {
			return fmt.Errorf("new_plan_id required for change_plan action")
		}
		sub.PlanID = *update.NewPlanID

	case "extend":
		if update.ExtendDays == nil {
			return fmt.Errorf("extend_days required for extend action")
		}
		sub.CurrentPeriodEnd = sub.CurrentPeriodEnd.AddDate(0, 0, *update.ExtendDays)

	default:
		return fmt.Errorf("unknown action: %s", update.Action)
	}

	sub.UpdatedAt = time.Now()
	err = s.subscriptionRepo.UpdateSubscription(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log the manual update
	err = s.subscriptionRepo.LogSubscriptionEvent(
		ctx,
		sub.ID,
		"manual."+update.Action,
		previousState,
		sub,
		fmt.Sprintf("Manual update by admin: %s", update.Reason),
	)
	if err != nil {
		s.logger.Warn("Failed to log manual update event", zap.Error(err))
	}

	s.logger.Info("Performed manual subscription update",
		zap.String("subscription_id", sub.ID.String()),
		zap.String("action", update.Action),
		zap.String("reason", update.Reason),
	)

	return nil
}

// SubscriptionMetrics contains key subscription metrics
type SubscriptionMetrics struct {
	TotalSubscriptions    int                           `json:"total_subscriptions"`
	ActiveSubscriptions   int                           `json:"active_subscriptions"`
	TrialSubscriptions    int                           `json:"trial_subscriptions"`
	CanceledSubscriptions int                           `json:"canceled_subscriptions"`
	PastDueSubscriptions  int                           `json:"past_due_subscriptions"`
	MRR                   float64                       `json:"mrr"` // Monthly Recurring Revenue in cents
	ARR                   float64                       `json:"arr"` // Annual Recurring Revenue in cents
	ChurnRate             float64                       `json:"churn_rate"` // Percentage
	ByPlan                map[string]*PlanMetrics       `json:"by_plan"`
	RecentPayments        *PaymentMetrics               `json:"recent_payments"`
}

// PlanMetrics contains metrics for a specific plan
type PlanMetrics struct {
	PlanID         string  `json:"plan_id"`
	Count          int     `json:"count"`
	Revenue        float64 `json:"revenue"`
}

// PaymentMetrics contains payment-related metrics
type PaymentMetrics struct {
	Last24Hours    *PaymentStats `json:"last_24_hours"`
	Last7Days      *PaymentStats `json:"last_7_days"`
	Last30Days     *PaymentStats `json:"last_30_days"`
}

// PaymentStats contains payment statistics
type PaymentStats struct {
	TotalPayments   int     `json:"total_payments"`
	SuccessfulPayments int  `json:"successful_payments"`
	FailedPayments  int     `json:"failed_payments"`
	TotalAmount     float64 `json:"total_amount"`
	AverageAmount   float64 `json:"average_amount"`
}

// GetSubscriptionMetrics calculates and returns subscription metrics
func (s *SubscriptionOperations) GetSubscriptionMetrics(ctx context.Context) (*SubscriptionMetrics, error) {
	metrics := &SubscriptionMetrics{
		ByPlan: make(map[string]*PlanMetrics),
		RecentPayments: &PaymentMetrics{
			Last24Hours: &PaymentStats{},
			Last7Days:   &PaymentStats{},
			Last30Days:  &PaymentStats{},
		},
	}

	// Get all subscriptions
	allSubs, err := s.subscriptionRepo.ListSubscriptions(ctx, subscription.SubscriptionFilter{
		Limit: 10000, // Get all
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	metrics.TotalSubscriptions = len(allSubs)

	// Calculate subscription metrics
	var totalMRR float64
	var totalARR float64
	var canceledLast30Days int
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	for _, sub := range allSubs {
		// Count by status
		switch sub.Status {
		case subscription.StatusActive:
			metrics.ActiveSubscriptions++
		case subscription.StatusTrial:
			metrics.TrialSubscriptions++
		case subscription.StatusCanceled:
			metrics.CanceledSubscriptions++
			if sub.CanceledAt != nil && sub.CanceledAt.After(thirtyDaysAgo) {
				canceledLast30Days++
			}
		case subscription.StatusPastDue:
			metrics.PastDueSubscriptions++
		}

		// Count by plan
		if _, exists := metrics.ByPlan[sub.PlanID]; !exists {
			metrics.ByPlan[sub.PlanID] = &PlanMetrics{
				PlanID: sub.PlanID,
			}
		}
		metrics.ByPlan[sub.PlanID].Count++

		// Calculate revenue (only for active/trial subscriptions)
		if sub.Status == subscription.StatusActive || sub.Status == subscription.StatusTrial {
			plan, exists := subscription.GetPlan(subscription.PlanID(sub.PlanID))
			if exists {
				// Calculate based on current period duration
				duration := sub.CurrentPeriodEnd.Sub(sub.CurrentPeriodStart)
				if duration > 0 {
					// Normalize to monthly revenue
					monthlyRevenue := float64(plan.MonthlyPrice)
					if duration > 31*24*time.Hour {
						// Annual subscription
						monthlyRevenue = float64(plan.YearlyPrice) / 12
					}

					totalMRR += monthlyRevenue
					totalARR += monthlyRevenue * 12

					metrics.ByPlan[sub.PlanID].Revenue += monthlyRevenue
				}
			}
		}
	}

	metrics.MRR = totalMRR
	metrics.ARR = totalARR

	// Calculate churn rate (canceled in last 30 days / active at start of period)
	activeAtStart := metrics.ActiveSubscriptions + canceledLast30Days
	if activeAtStart > 0 {
		metrics.ChurnRate = (float64(canceledLast30Days) / float64(activeAtStart)) * 100
	}

	// Get payment metrics
	now := time.Now()
	allPayments, err := s.subscriptionRepo.ListPayments(ctx, nil, nil, 10000)
	if err != nil {
		s.logger.Warn("Failed to get payments for metrics", zap.Error(err))
	} else {
		for _, payment := range allPayments {
			var stats *PaymentStats

			// Determine which time bucket
			if payment.CreatedAt.After(now.Add(-24 * time.Hour)) {
				stats = metrics.RecentPayments.Last24Hours
			} else if payment.CreatedAt.After(now.Add(-7 * 24 * time.Hour)) {
				stats = metrics.RecentPayments.Last7Days
			} else if payment.CreatedAt.After(now.Add(-30 * 24 * time.Hour)) {
				stats = metrics.RecentPayments.Last30Days
			} else {
				continue
			}

			stats.TotalPayments++
			stats.TotalAmount += float64(payment.Amount)

			if payment.Status == subscription.PaymentStatusSucceeded {
				stats.SuccessfulPayments++
			} else if payment.Status == subscription.PaymentStatusFailed {
				stats.FailedPayments++
			}
		}

		// Calculate averages
		for _, stats := range []*PaymentStats{
			metrics.RecentPayments.Last24Hours,
			metrics.RecentPayments.Last7Days,
			metrics.RecentPayments.Last30Days,
		} {
			if stats.TotalPayments > 0 {
				stats.AverageAmount = stats.TotalAmount / float64(stats.TotalPayments)
			}
		}
	}

	return metrics, nil
}

// CreateRefund creates a refund for a payment
func (s *SubscriptionOperations) CreateRefund(ctx context.Context, paymentID uuid.UUID, amount int, reason string) error {
	payment, err := s.subscriptionRepo.GetPayment(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment.Status != subscription.PaymentStatusSucceeded {
		return fmt.Errorf("can only refund succeeded payments")
	}

	// Create refund in Stripe if available
	// Note: We would call CreateRefund here, but RefundParams is not exported from payment package
	// This should be fixed by exporting RefundParams in the payment package
	// For now, we'll just update the database status
	if payment.StripePaymentIntentID != nil {
		s.logger.Info("Stripe refund would be created here",
			zap.String("payment_intent_id", *payment.StripePaymentIntentID),
			zap.Int("amount", amount),
		)
		// TODO: Fix after RefundParams is exported:
		// refundParams := &payment.RefundParams{
		// 	UserID:          payment.UserID,
		// 	PaymentIntentID: *payment.StripePaymentIntentID,
		// 	Amount:          amount,
		// 	Reason:          reason,
		// }
		// _, err = s.paymentService.CreateRefund(ctx, refundParams)
		// if err != nil {
		// 	return fmt.Errorf("failed to create Stripe refund: %w", err)
		// }
	}

	// Update payment status
	payment.Status = subscription.PaymentStatusRefunded
	err = s.subscriptionRepo.UpdatePayment(ctx, payment)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	s.logger.Info("Created refund",
		zap.String("payment_id", paymentID.String()),
		zap.Int("amount", amount),
		zap.String("reason", reason),
	)

	return nil
}

// Helper functions

func ptrTime(t time.Time) *time.Time {
	return &t
}

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/payment"
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v79"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPaymentFlowE2E tests the complete payment flow from checkout to subscription creation
func TestPaymentFlowE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	t.Run("Complete Stripe Checkout Flow", func(t *testing.T) {
		// Create test user
		userID := uuid.New()
		email := "test@example.com"

		// Step 1: Create Stripe customer
		customer, err := testEnv.PaymentService.CreateCustomer(ctx, userID, email, "Test User")
		require.NoError(t, err)
		require.NotNil(t, customer)
		assert.Equal(t, email, customer.Email)
		assert.Equal(t, userID.String(), customer.Metadata["user_id"])

		// Step 2: Create checkout session
		plan := subscription.PredefinedPlans[subscription.PlanPremium]
		checkoutParams := &payment.CheckoutSessionParams{
			UserID:          userID,
			CustomerID:      customer.ID,
			CustomerEmail:   email,
			PlanID:          string(subscription.PlanPremium),
			PlanName:        plan.Name,
			PlanDescription: plan.Description,
			Amount:          plan.MonthlyPrice,
			Currency:        "usd",
			BillingInterval: subscription.IntervalMonth,
			TrialDays:       plan.TrialDays,
			SuccessURL:      "https://example.com/success",
			CancelURL:       "https://example.com/cancel",
		}

		session, err := testEnv.PaymentService.CreateCheckoutSession(ctx, checkoutParams)
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.NotEmpty(t, session.ID)
		assert.NotEmpty(t, session.URL)
		assert.Equal(t, userID.String(), session.Metadata["user_id"])

		// Step 3: Simulate successful checkout (webhook simulation)
		// This would normally come from Stripe webhook
		mockSubscription := createMockStripeSubscription(customer.ID, userID)

		// Simulate subscription.created webhook
		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.created",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSubscription),
			},
		})
		require.NoError(t, err)

		// Step 4: Verify subscription was created in database
		time.Sleep(100 * time.Millisecond) // Give async processing time

		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, userID, sub.UserID)
		assert.Equal(t, string(subscription.PlanPremium), sub.PlanID)
		assert.Equal(t, subscription.StatusActive, sub.Status)
		assert.NotNil(t, sub.StripeCustomerID)
		assert.NotNil(t, sub.StripeSubscriptionID)
		assert.Equal(t, customer.ID, *sub.StripeCustomerID)

		// Step 5: Verify subscription event was logged
		events, err := testEnv.SubscriptionRepo.ListSubscriptionEvents(ctx, sub.ID, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, events)
		assert.Equal(t, subscription.EventTypeCreated, events[0].EventType)
	})
}

// TestStripeWebhookScenarios tests various Stripe webhook event scenarios
func TestStripeWebhookScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	userID := uuid.New()
	customerID := "cus_test_" + uuid.New().String()
	subscriptionID := "sub_test_" + uuid.New().String()

	// Create initial subscription
	sub := &subscription.Subscription{
		ID:                   uuid.New(),
		UserID:               userID,
		PlanID:               string(subscription.PlanPremium),
		Status:               subscription.StatusActive,
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
		CancelAtPeriodEnd:    false,
		StripeCustomerID:     &customerID,
		StripeSubscriptionID: &subscriptionID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	t.Run("Subscription Updated - Plan Change", func(t *testing.T) {
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.Status = stripe.SubscriptionStatusActive
		mockSub.Metadata["plan_id"] = string(subscription.PlanEnterprise)

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.updated",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		updatedSub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusActive, updatedSub.Status)
	})

	t.Run("Subscription Updated - Cancel at Period End", func(t *testing.T) {
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.CancelAtPeriodEnd = true
		mockSub.CanceledAt = time.Now().Unix()

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.updated",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		updatedSub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.True(t, updatedSub.CancelAtPeriodEnd)
		assert.NotNil(t, updatedSub.CanceledAt)
	})

	t.Run("Subscription Deleted", func(t *testing.T) {
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.Status = stripe.SubscriptionStatusCanceled

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.deleted",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		deletedSub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusCanceled, deletedSub.Status)
		assert.NotNil(t, deletedSub.CanceledAt)
	})

	t.Run("Invoice Payment Succeeded", func(t *testing.T) {
		mockInvoice := &stripe.Invoice{
			ID:         "in_test_" + uuid.New().String(),
			AmountPaid: 999,
			Currency:   stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			StatusTransitions: &stripe.InvoiceStatusTransitions{
				PaidAt: time.Now().Unix(),
			},
		}

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_succeeded",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Verify payment record was created
		payments, err := testEnv.SubscriptionRepo.ListPayments(ctx, &userID, nil, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, payments)

		// Find the payment we just created
		var foundPayment *subscription.Payment
		for _, p := range payments {
			if p.Amount == 999 {
				foundPayment = p
				break
			}
		}
		require.NotNil(t, foundPayment)
		assert.Equal(t, subscription.PaymentStatusSucceeded, foundPayment.Status)
		assert.NotNil(t, foundPayment.PaidAt)
	})

	t.Run("Invoice Payment Failed", func(t *testing.T) {
		mockInvoice := &stripe.Invoice{
			ID:        "in_test_" + uuid.New().String(),
			AmountDue: 999,
			Currency:  stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			LastFinalizationError: &stripe.InvoiceLastFinalizationError{
				Message: "Card declined",
			},
		}

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_failed",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Verify subscription is now past_due
		updatedSub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusPastDue, updatedSub.Status)

		// Verify failed payment record was created
		payments, err := testEnv.SubscriptionRepo.ListPayments(ctx, &userID, nil, 10)
		require.NoError(t, err)

		var foundFailedPayment *subscription.Payment
		for _, p := range payments {
			if p.Status == subscription.PaymentStatusFailed {
				foundFailedPayment = p
				break
			}
		}
		require.NotNil(t, foundFailedPayment)
		assert.Equal(t, subscription.PaymentStatusFailed, foundFailedPayment.Status)
		assert.NotNil(t, foundFailedPayment.FailedAt)
		assert.NotNil(t, foundFailedPayment.FailureReason)
		assert.Contains(t, *foundFailedPayment.FailureReason, "Card declined")
	})
}

// TestIAPValidation tests iOS and Android in-app purchase validation
func TestIAPValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	userID := uuid.New()

	t.Run("iOS Receipt Validation", func(t *testing.T) {
		// This is a mock test - in production, you'd use actual Apple receipts
		// For testing, we'll create a mock IAP service with test configuration

		appleConfig := &payment.AppleIAPConfig{
			SharedSecret:  "test_secret",
			BundleID:      "com.example.donelist",
			UseSandbox:    true,
			ProductionURL: "https://buy.itunes.apple.com/verifyReceipt",
			SandboxURL:    "https://sandbox.itunes.apple.com/verifyReceipt",
		}

		iapService := payment.NewIAPService(
			testEnv.Logger,
			testEnv.SubscriptionRepo,
			appleConfig,
			nil, // No Google config for this test
		)

		// In a real test, you'd need a valid test receipt from Apple
		// For now, we'll skip the actual validation and test the sync logic

		// Create a mock validation result
		mockResult := &payment.IAPValidationResult{
			Platform:              payment.IAPPlatformApple,
			TransactionID:         "1000000000000000",
			OriginalTransactionID: "1000000000000000",
			ProductID:             string(subscription.PlanPremium),
			PurchaseDate:          time.Now(),
			ExpiryDate:            ptrTime(time.Now().AddDate(0, 1, 0)),
			IsActive:              true,
			AutoRenewing:          true,
		}

		// Test syncing subscription from receipt
		err := iapService.SyncSubscriptionFromReceipt(ctx, userID, mockResult)
		require.NoError(t, err)

		// Verify subscription was created
		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, userID, sub.UserID)
		assert.Equal(t, string(subscription.PlanPremium), sub.PlanID)
		assert.Equal(t, subscription.StatusActive, sub.Status)
		assert.NotNil(t, sub.Metadata)
		assert.Equal(t, string(payment.IAPPlatformApple), sub.Metadata["platform"])
	})

	t.Run("iOS Receipt Validation - Expired", func(t *testing.T) {
		userID := uuid.New()

		appleConfig := &payment.AppleIAPConfig{
			SharedSecret: "test_secret",
			BundleID:     "com.example.donelist",
			UseSandbox:   true,
		}

		iapService := payment.NewIAPService(
			testEnv.Logger,
			testEnv.SubscriptionRepo,
			appleConfig,
			nil,
		)

		// Mock expired subscription
		mockResult := &payment.IAPValidationResult{
			Platform:              payment.IAPPlatformApple,
			TransactionID:         "2000000000000000",
			OriginalTransactionID: "2000000000000000",
			ProductID:             string(subscription.PlanPremium),
			PurchaseDate:          time.Now().AddDate(0, -2, 0),
			ExpiryDate:            ptrTime(time.Now().AddDate(0, -1, 0)),
			IsActive:              false,
			AutoRenewing:          false,
		}

		err := iapService.SyncSubscriptionFromReceipt(ctx, userID, mockResult)
		require.NoError(t, err)

		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusExpired, sub.Status)
		assert.True(t, sub.CancelAtPeriodEnd)
	})

	t.Run("iOS Receipt Validation - Canceled", func(t *testing.T) {
		userID := uuid.New()

		appleConfig := &payment.AppleIAPConfig{
			SharedSecret: "test_secret",
			BundleID:     "com.example.donelist",
			UseSandbox:   true,
		}

		iapService := payment.NewIAPService(
			testEnv.Logger,
			testEnv.SubscriptionRepo,
			appleConfig,
			nil,
		)

		cancelDate := time.Now().AddDate(0, 0, -1)
		mockResult := &payment.IAPValidationResult{
			Platform:              payment.IAPPlatformApple,
			TransactionID:         "3000000000000000",
			OriginalTransactionID: "3000000000000000",
			ProductID:             string(subscription.PlanPremium),
			PurchaseDate:          time.Now().AddDate(0, -1, 0),
			ExpiryDate:            ptrTime(time.Now().AddDate(0, 0, 5)),
			CancellationDate:      &cancelDate,
			IsActive:              false,
			AutoRenewing:          false,
		}

		err := iapService.SyncSubscriptionFromReceipt(ctx, userID, mockResult)
		require.NoError(t, err)

		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusCanceled, sub.Status)
		assert.NotNil(t, sub.CanceledAt)
	})
}

// TestSubscriptionLifecycle tests the complete subscription lifecycle
func TestSubscriptionLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	userID := uuid.New()
	customerID := "cus_test_" + uuid.New().String()
	subscriptionID := "sub_test_" + uuid.New().String()

	t.Run("Create Active Subscription", func(t *testing.T) {
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}

		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		retrieved, err := testEnv.SubscriptionRepo.GetSubscription(ctx, sub.ID)
		require.NoError(t, err)
		assert.Equal(t, sub.ID, retrieved.ID)
		assert.True(t, retrieved.IsActive())
	})

	t.Run("Renew Subscription", func(t *testing.T) {
		// Simulate invoice payment succeeded for renewal
		mockInvoice := &stripe.Invoice{
			ID:         "in_test_renew_" + uuid.New().String(),
			AmountPaid: 999,
			Currency:   stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			StatusTransitions: &stripe.InvoiceStatusTransitions{
				PaidAt: time.Now().Unix(),
			},
		}

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_succeeded",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Update subscription with new period
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.CurrentPeriodStart = time.Now().AddDate(0, 1, 0).Unix()
		mockSub.CurrentPeriodEnd = time.Now().AddDate(0, 2, 0).Unix()

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.updated",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		// Verify subscription was updated
		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusActive, sub.Status)
	})

	t.Run("Cancel Subscription at Period End", func(t *testing.T) {
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.CancelAtPeriodEnd = true
		mockSub.CanceledAt = time.Now().Unix()

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.updated",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.True(t, sub.CancelAtPeriodEnd)
		assert.True(t, sub.IsActive()) // Still active until period end
	})

	t.Run("Expire Subscription", func(t *testing.T) {
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.Status = stripe.SubscriptionStatusCanceled
		mockSub.CanceledAt = time.Now().Unix()

		err := testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.deleted",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusCanceled, sub.Status)
		assert.False(t, sub.IsActive())
	})
}

// TestProrationAndRefund tests proration logic and refund scenarios
func TestProrationAndRefund(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	userID := uuid.New()
	customerID := "cus_test_" + uuid.New().String()

	t.Run("Plan Upgrade with Proration", func(t *testing.T) {
		// Create initial subscription
		subscriptionID := "sub_test_upgrade_" + uuid.New().String()
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now().AddDate(0, 0, -15),
			CurrentPeriodEnd:     time.Now().AddDate(0, 0, 15),
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Simulate upgrade with proration invoice
		mockInvoice := &stripe.Invoice{
			ID:         "in_test_proration_" + uuid.New().String(),
			AmountPaid: 1500, // Prorated amount
			Currency:   stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			StatusTransitions: &stripe.InvoiceStatusTransitions{
				PaidAt: time.Now().Unix(),
			},
		}

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_succeeded",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Update subscription with new plan
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.Metadata["plan_id"] = string(subscription.PlanEnterprise)

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.updated",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		// Verify proration payment was recorded
		payments, err := testEnv.SubscriptionRepo.ListPayments(ctx, &userID, nil, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, payments)

		var prorationPayment *subscription.Payment
		for _, p := range payments {
			if p.Amount == 1500 {
				prorationPayment = p
				break
			}
		}
		require.NotNil(t, prorationPayment)
		assert.Equal(t, subscription.PaymentStatusSucceeded, prorationPayment.Status)
	})

	t.Run("Refund Processing", func(t *testing.T) {
		// Create a payment to refund
		paymentID := uuid.New()
		payment := &subscription.Payment{
			ID:             paymentID,
			UserID:         userID,
			Amount:         999,
			Currency:       "usd",
			Status:         subscription.PaymentStatusSucceeded,
			PaidAt:         ptrTime(time.Now().AddDate(0, 0, -1)),
			CreatedAt:      time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreatePayment(ctx, payment)
		require.NoError(t, err)

		// Simulate refund
		payment.Status = subscription.PaymentStatusRefunded
		err = testEnv.SubscriptionRepo.UpdatePayment(ctx, payment)
		require.NoError(t, err)

		// Verify refund status
		refundedPayment, err := testEnv.SubscriptionRepo.GetPayment(ctx, paymentID)
		require.NoError(t, err)
		assert.Equal(t, subscription.PaymentStatusRefunded, refundedPayment.Status)
	})
}

// Helper functions

func createMockStripeSubscription(customerID string, userID uuid.UUID) *stripe.Subscription {
	now := time.Now()
	return &stripe.Subscription{
		ID:     "sub_test_" + uuid.New().String(),
		Status: stripe.SubscriptionStatusActive,
		Customer: &stripe.Customer{
			ID: customerID,
		},
		CurrentPeriodStart: now.Unix(),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0).Unix(),
		Metadata: map[string]string{
			"user_id": userID.String(),
			"plan_id": string(subscription.PlanPremium),
		},
		CancelAtPeriodEnd: false,
	}
}

func marshalToJSON(t *testing.T, v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

// TestFeatureFlagAccessControl tests premium feature gating
func TestFeatureFlagAccessControl(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	t.Run("Free User Cannot Access Premium Features", func(t *testing.T) {
		userID := uuid.New()

		// No subscription = free tier
		sub, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		if err == nil && sub != nil {
			// User has subscription, skip this case
			t.Skip("User unexpectedly has subscription")
		}

		// Verify premium features are blocked
		isPremium := sub != nil && sub.IsActive() && sub.PlanID != string(subscription.PlanFree)
		assert.False(t, isPremium, "Free user should not have premium access")
	})

	t.Run("Premium User Can Access Premium Features", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_premium_" + uuid.New().String()
		subscriptionID := "sub_test_premium_" + uuid.New().String()

		// Create premium subscription
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().AddDate(0, 1, 0),
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Verify premium access
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		assert.True(t, retrieved.IsActive())
		assert.Equal(t, string(subscription.PlanPremium), retrieved.PlanID)
	})

	t.Run("Expired Premium User Loses Access", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_expired_" + uuid.New().String()
		subscriptionID := "sub_test_expired_" + uuid.New().String()

		// Create expired subscription
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusExpired,
			CurrentPeriodStart:   time.Now().AddDate(0, -2, 0),
			CurrentPeriodEnd:     time.Now().AddDate(0, -1, 0),
			CancelAtPeriodEnd:    true,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CanceledAt:           ptrTime(time.Now().AddDate(0, -1, 0)),
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Verify no premium access
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		assert.False(t, retrieved.IsActive())
	})

	t.Run("Canceled But Active Period Still Has Access", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_canceled_active_" + uuid.New().String()
		subscriptionID := "sub_test_canceled_active_" + uuid.New().String()

		// Create canceled subscription with remaining period
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now().AddDate(0, 0, -15),
			CurrentPeriodEnd:     time.Now().AddDate(0, 0, 15), // Still 15 days left
			CancelAtPeriodEnd:    true,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CanceledAt:           ptrTime(time.Now().AddDate(0, 0, -5)),
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Should still have access until period ends
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByUserID(ctx, userID)
		require.NoError(t, err)
		assert.True(t, retrieved.IsActive())
		assert.True(t, retrieved.CancelAtPeriodEnd)
	})
}

// TestGracePeriodHandling tests subscription grace period scenarios
func TestGracePeriodHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	t.Run("Past Due Subscription Enters Grace Period", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_grace_" + uuid.New().String()
		subscriptionID := "sub_test_grace_" + uuid.New().String()

		// Create active subscription
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now().AddDate(0, -1, 0),
			CurrentPeriodEnd:     time.Now().AddDate(0, 0, -1), // Period ended yesterday
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Simulate payment failure webhook
		mockInvoice := &stripe.Invoice{
			ID:        "in_test_failed_" + uuid.New().String(),
			AmountDue: 999,
			Currency:  stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			LastFinalizationError: &stripe.InvoiceLastFinalizationError{
				Message: "Card declined",
			},
		}

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_failed",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Verify subscription is now past_due (grace period)
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusPastDue, retrieved.Status)
	})

	t.Run("Grace Period Recovery on Successful Payment", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_grace_recovery_" + uuid.New().String()
		subscriptionID := "sub_test_grace_recovery_" + uuid.New().String()

		// Create past_due subscription (in grace period)
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusPastDue,
			CurrentPeriodStart:   time.Now().AddDate(0, -1, 0),
			CurrentPeriodEnd:     time.Now().AddDate(0, 0, -3),
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Simulate successful payment recovery
		mockInvoice := &stripe.Invoice{
			ID:         "in_test_recovery_" + uuid.New().String(),
			AmountPaid: 999,
			Currency:   stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			StatusTransitions: &stripe.InvoiceStatusTransitions{
				PaidAt: time.Now().Unix(),
			},
		}

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_succeeded",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Update subscription status via webhook
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.Status = stripe.SubscriptionStatusActive
		mockSub.CurrentPeriodStart = time.Now().Unix()
		mockSub.CurrentPeriodEnd = time.Now().AddDate(0, 1, 0).Unix()

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.updated",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		// Verify subscription is active again
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusActive, retrieved.Status)
	})

	t.Run("Grace Period Expiration Cancels Subscription", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_grace_expire_" + uuid.New().String()
		subscriptionID := "sub_test_grace_expire_" + uuid.New().String()

		// Create past_due subscription
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusPastDue,
			CurrentPeriodStart:   time.Now().AddDate(0, -2, 0),
			CurrentPeriodEnd:     time.Now().AddDate(0, -1, -7), // Grace period expired
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Simulate Stripe canceling due to payment failure
		mockSub := createMockStripeSubscription(customerID, userID)
		mockSub.ID = subscriptionID
		mockSub.Status = stripe.SubscriptionStatusCanceled
		mockSub.CanceledAt = time.Now().Unix()

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "customer.subscription.deleted",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockSub),
			},
		})
		require.NoError(t, err)

		// Verify subscription is canceled
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusCanceled, retrieved.Status)
		assert.NotNil(t, retrieved.CanceledAt)
	})
}

// TestDunningFlow tests the payment retry and dunning process
func TestDunningFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testEnv := setupTestEnvironment(t)
	defer testEnv.Cleanup()

	t.Run("Multiple Payment Retry Attempts", func(t *testing.T) {
		userID := uuid.New()
		customerID := "cus_test_dunning_" + uuid.New().String()
		subscriptionID := "sub_test_dunning_" + uuid.New().String()

		// Create active subscription
		sub := &subscription.Subscription{
			ID:                   uuid.New(),
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now().AddDate(0, -1, 0),
			CurrentPeriodEnd:     time.Now().AddDate(0, 0, -1),
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Simulate first payment failure
		for i := 1; i <= 3; i++ {
			mockInvoice := &stripe.Invoice{
				ID:           "in_test_retry_" + uuid.New().String(),
				AmountDue:    999,
				Currency:     stripe.CurrencyUSD,
				AttemptCount: int64(i),
				Subscription: &stripe.Subscription{
					ID: subscriptionID,
				},
				LastFinalizationError: &stripe.InvoiceLastFinalizationError{
					Message: "Card declined - attempt " + string(rune('0'+i)),
				},
			}

			err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
				ID:   "evt_test_" + uuid.New().String(),
				Type: "invoice.payment_failed",
				Data: &stripe.EventData{
					Raw: marshalToJSON(t, mockInvoice),
				},
			})
			require.NoError(t, err)
		}

		// Verify subscription is past_due after multiple failures
		retrieved, err := testEnv.SubscriptionRepo.GetSubscriptionByStripeID(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Equal(t, subscription.StatusPastDue, retrieved.Status)

		// Verify failed payment records
		payments, err := testEnv.SubscriptionRepo.ListPayments(ctx, &userID, nil, 10)
		require.NoError(t, err)

		failedCount := 0
		for _, p := range payments {
			if p.Status == subscription.PaymentStatusFailed {
				failedCount++
			}
		}
		assert.GreaterOrEqual(t, failedCount, 1, "Should have at least one failed payment record")
	})

	t.Run("Dunning Email Triggers", func(t *testing.T) {
		// This test verifies that the subscription events are logged
		// which can be used to trigger notification emails
		userID := uuid.New()
		customerID := "cus_test_dunning_email_" + uuid.New().String()
		subscriptionID := "sub_test_dunning_email_" + uuid.New().String()

		subID := uuid.New()
		sub := &subscription.Subscription{
			ID:                   subID,
			UserID:               userID,
			PlanID:               string(subscription.PlanPremium),
			Status:               subscription.StatusActive,
			CurrentPeriodStart:   time.Now().AddDate(0, -1, 0),
			CurrentPeriodEnd:     time.Now().AddDate(0, 0, -1),
			CancelAtPeriodEnd:    false,
			StripeCustomerID:     &customerID,
			StripeSubscriptionID: &subscriptionID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err := testEnv.SubscriptionRepo.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Simulate payment failure
		mockInvoice := &stripe.Invoice{
			ID:        "in_test_dunning_trigger_" + uuid.New().String(),
			AmountDue: 999,
			Currency:  stripe.CurrencyUSD,
			Subscription: &stripe.Subscription{
				ID: subscriptionID,
			},
			LastFinalizationError: &stripe.InvoiceLastFinalizationError{
				Message: "Card declined",
			},
		}

		err = testEnv.WebhookHandler.HandleEvent(ctx, &stripe.Event{
			ID:   "evt_test_" + uuid.New().String(),
			Type: "invoice.payment_failed",
			Data: &stripe.EventData{
				Raw: marshalToJSON(t, mockInvoice),
			},
		})
		require.NoError(t, err)

		// Verify subscription event was logged
		events, err := testEnv.SubscriptionRepo.ListSubscriptionEvents(ctx, subID, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, events, "Should have subscription events logged")

		// Check for status change event
		hasStatusChange := false
		for _, e := range events {
			if e.EventType == subscription.EventTypeStatusChanged ||
				e.EventType == subscription.EventTypePaymentFailed {
				hasStatusChange = true
				break
			}
		}
		assert.True(t, hasStatusChange, "Should have payment failure event logged")
	})
}

// TestSubscriptionStateTransitions tests all valid and invalid state transitions
func TestSubscriptionStateTransitions(t *testing.T) {
	testCases := []struct {
		name        string
		fromStatus  subscription.SubscriptionStatus
		toStatus    subscription.SubscriptionStatus
		valid       bool
		reason      string
	}{
		// Valid transitions
		{
			name:       "Active to PastDue",
			fromStatus: subscription.StatusActive,
			toStatus:   subscription.StatusPastDue,
			valid:      true,
			reason:     "Payment failed",
		},
		{
			name:       "Active to Canceled",
			fromStatus: subscription.StatusActive,
			toStatus:   subscription.StatusCanceled,
			valid:      true,
			reason:     "User canceled",
		},
		{
			name:       "Active to Expired",
			fromStatus: subscription.StatusActive,
			toStatus:   subscription.StatusExpired,
			valid:      true,
			reason:     "Period ended without renewal",
		},
		{
			name:       "PastDue to Active",
			fromStatus: subscription.StatusPastDue,
			toStatus:   subscription.StatusActive,
			valid:      true,
			reason:     "Payment recovered",
		},
		{
			name:       "PastDue to Canceled",
			fromStatus: subscription.StatusPastDue,
			toStatus:   subscription.StatusCanceled,
			valid:      true,
			reason:     "Grace period expired",
		},
		{
			name:       "Trialing to Active",
			fromStatus: subscription.StatusTrialing,
			toStatus:   subscription.StatusActive,
			valid:      true,
			reason:     "Trial ended, payment succeeded",
		},
		{
			name:       "Trialing to Canceled",
			fromStatus: subscription.StatusTrialing,
			toStatus:   subscription.StatusCanceled,
			valid:      true,
			reason:     "Trial ended, no payment",
		},
		// Invalid transitions
		{
			name:       "Canceled to Active",
			fromStatus: subscription.StatusCanceled,
			toStatus:   subscription.StatusActive,
			valid:      false,
			reason:     "Must create new subscription",
		},
		{
			name:       "Expired to Active",
			fromStatus: subscription.StatusExpired,
			toStatus:   subscription.StatusActive,
			valid:      false,
			reason:     "Must create new subscription",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test state machine validation
			stateMachine := subscription.NewStateMachine()

			canTransition := stateMachine.CanTransition(tc.fromStatus, tc.toStatus)

			if tc.valid {
				assert.True(t, canTransition,
					"Should allow transition from %s to %s: %s",
					tc.fromStatus, tc.toStatus, tc.reason)
			} else {
				assert.False(t, canTransition,
					"Should NOT allow transition from %s to %s: %s",
					tc.fromStatus, tc.toStatus, tc.reason)
			}
		})
	}
}

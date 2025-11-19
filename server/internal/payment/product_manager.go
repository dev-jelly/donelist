package payment

import (
	"context"
	"fmt"

	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/stripe/stripe-go/v79"
	"go.uber.org/zap"
)

// ProductManager manages Stripe products and prices
type ProductManager struct {
	client *StripeClient
	logger *zap.Logger
}

// NewProductManager creates a new product manager
func NewProductManager(client *StripeClient, logger *zap.Logger) *ProductManager {
	return &ProductManager{
		client: client,
		logger: logger,
	}
}

// SyncProducts syncs local plan definitions with Stripe products and prices
func (pm *ProductManager) SyncProducts(ctx context.Context) error {
	pm.logger.Info("Starting product sync with Stripe")

	for planID, plan := range subscription.PredefinedPlans {
		if !plan.Active {
			pm.logger.Debug("Skipping inactive plan", zap.String("plan_id", string(planID)))
			continue
		}

		// Create or update product
		productID := fmt.Sprintf("prod_%s", planID)
		stripeProduct, err := pm.createOrUpdateProduct(ctx, productID, plan)
		if err != nil {
			return fmt.Errorf("failed to sync product %s: %w", planID, err)
		}

		// Create or update prices (monthly and yearly)
		if plan.MonthlyPrice > 0 {
			monthlyPriceID := fmt.Sprintf("price_%s_monthly", planID)
			_, err = pm.createOrUpdatePrice(ctx, monthlyPriceID, stripeProduct.ID, plan.MonthlyPrice, "month", plan.Currency)
			if err != nil {
				return fmt.Errorf("failed to sync monthly price for %s: %w", planID, err)
			}
		}

		if plan.YearlyPrice > 0 {
			yearlyPriceID := fmt.Sprintf("price_%s_yearly", planID)
			_, err = pm.createOrUpdatePrice(ctx, yearlyPriceID, stripeProduct.ID, plan.YearlyPrice, "year", plan.Currency)
			if err != nil {
				return fmt.Errorf("failed to sync yearly price for %s: %w", planID, err)
			}
		}
	}

	pm.logger.Info("Product sync completed successfully")
	return nil
}

// createOrUpdateProduct creates or updates a Stripe product
func (pm *ProductManager) createOrUpdateProduct(ctx context.Context, productID string, plan *subscription.Plan) (*stripe.Product, error) {
	// Try to retrieve existing product
	_, err := pm.client.GetClient().Products.Get(productID, nil)
	if err == nil {
		// Update existing product
		params := &stripe.ProductParams{
			Name:        stripe.String(plan.Name),
			Description: stripe.String(plan.Description),
			Active:      stripe.Bool(plan.Active),
			Metadata: map[string]string{
				"plan_id": string(plan.ID),
			},
		}

		updatedProduct, err := pm.client.GetClient().Products.Update(productID, params)
		if err != nil {
			return nil, fmt.Errorf("failed to update product: %w", err)
		}

		pm.logger.Info("Updated Stripe product",
			zap.String("product_id", productID),
			zap.String("plan_id", string(plan.ID)),
		)

		return updatedProduct, nil
	}

	// Create new product
	params := &stripe.ProductParams{
		ID:          stripe.String(productID),
		Name:        stripe.String(plan.Name),
		Description: stripe.String(plan.Description),
		Active:      stripe.Bool(plan.Active),
		Metadata: map[string]string{
			"plan_id": string(plan.ID),
		},
	}

	newProduct, err := pm.client.GetClient().Products.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create product: %w", err)
	}

	pm.logger.Info("Created Stripe product",
		zap.String("product_id", productID),
		zap.String("plan_id", string(plan.ID)),
	)

	return newProduct, nil
}

// createOrUpdatePrice creates or updates a Stripe price
func (pm *ProductManager) createOrUpdatePrice(ctx context.Context, priceID, productID string, amount int, interval, currency string) (*stripe.Price, error) {
	// Try to retrieve existing price
	existingPrice, err := pm.client.GetClient().Prices.Get(priceID, nil)
	if err == nil {
		// Prices cannot be updated, only deactivated and recreated
		// Check if the price details match
		if existingPrice.UnitAmount == int64(amount) &&
			string(existingPrice.Recurring.Interval) == interval &&
			string(existingPrice.Currency) == currency &&
			existingPrice.Active {
			pm.logger.Debug("Price already exists and is up to date",
				zap.String("price_id", priceID),
			)
			return existingPrice, nil
		}

		// Deactivate old price if details don't match
		_, err = pm.client.GetClient().Prices.Update(priceID, &stripe.PriceParams{
			Active: stripe.Bool(false),
		})
		if err != nil {
			pm.logger.Warn("Failed to deactivate old price",
				zap.String("price_id", priceID),
				zap.Error(err),
			)
		}
	}

	// Create new price
	params := &stripe.PriceParams{
		Product:    stripe.String(productID),
		UnitAmount: stripe.Int64(int64(amount)),
		Currency:   stripe.String(currency),
		Recurring: &stripe.PriceRecurringParams{
			Interval: stripe.String(interval),
		},
		LookupKey: stripe.String(priceID), // Use lookup key for easy retrieval
		Metadata: map[string]string{
			"price_id": priceID,
			"interval": interval,
		},
	}

	newPrice, err := pm.client.GetClient().Prices.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create price: %w", err)
	}

	pm.logger.Info("Created Stripe price",
		zap.String("price_id", newPrice.ID),
		zap.String("lookup_key", priceID),
		zap.String("product_id", productID),
		zap.Int("amount", amount),
		zap.String("interval", interval),
	)

	return newPrice, nil
}

// GetPriceByLookupKey retrieves a price by its lookup key
func (pm *ProductManager) GetPriceByLookupKey(ctx context.Context, lookupKey string) (*stripe.Price, error) {
	params := &stripe.PriceListParams{
		LookupKeys: []*string{stripe.String(lookupKey)},
		Active:     stripe.Bool(true),
	}
	params.Limit = stripe.Int64(1)

	iter := pm.client.GetClient().Prices.List(params)
	if iter.Next() {
		return iter.Price(), nil
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to get price by lookup key: %w", err)
	}

	return nil, fmt.Errorf("price not found for lookup key: %s", lookupKey)
}

// ListProducts lists all active products from Stripe
func (pm *ProductManager) ListProducts(ctx context.Context) ([]*stripe.Product, error) {
	params := &stripe.ProductListParams{
		Active: stripe.Bool(true),
	}
	params.Limit = stripe.Int64(100)

	var products []*stripe.Product
	iter := pm.client.GetClient().Products.List(params)

	for iter.Next() {
		products = append(products, iter.Product())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list products: %w", err)
	}

	return products, nil
}

// ListPricesForProduct lists all active prices for a product
func (pm *ProductManager) ListPricesForProduct(ctx context.Context, productID string) ([]*stripe.Price, error) {
	params := &stripe.PriceListParams{
		Product: stripe.String(productID),
		Active:  stripe.Bool(true),
	}
	params.Limit = stripe.Int64(100)

	var prices []*stripe.Price
	iter := pm.client.GetClient().Prices.List(params)

	for iter.Next() {
		prices = append(prices, iter.Price())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list prices: %w", err)
	}

	return prices, nil
}
package subscriptionjobs

import (
	"github.com/dev-jelly/donelist/internal/subscription"
	"github.com/jmoiron/sqlx"
)

// Subscription is an alias to the subscription package's Subscription type
type Subscription = subscription.Subscription

// Repository wraps the subscription.Repository to provide additional methods
type Repository struct {
	*subscription.Repository
}

// NewRepository creates a repository wrapper for subscriptionjobs
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		Repository: subscription.NewRepository(db),
	}
}

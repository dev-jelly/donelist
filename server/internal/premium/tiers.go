package premium

// Tier represents a subscription tier
type Tier string

const (
	// TierFree is the free tier with basic features
	TierFree Tier = "free"

	// TierPremium is the premium tier with advanced features
	TierPremium Tier = "premium"

	// TierEnterprise is the enterprise tier with all features
	TierEnterprise Tier = "enterprise"
)

// IsValid checks if a tier is valid
func (t Tier) IsValid() bool {
	switch t {
	case TierFree, TierPremium, TierEnterprise:
		return true
	default:
		return false
	}
}

// String returns the string representation of a tier
func (t Tier) String() string {
	return string(t)
}

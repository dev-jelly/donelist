package ratelimit

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/redis"
)

// TierLimits defines rate limits for different user tiers
type TierLimits struct {
	Free       limiter.Rate
	Premium    limiter.Rate
	Enterprise limiter.Rate
}

// BurstConfig defines burst control settings
type BurstConfig struct {
	AllowBurst   bool  // Whether to allow burst
	BurstSize    int64 // Maximum burst size (tokens to accumulate)
	BurstPeriod  time.Duration // Period to track burst
}

// EndpointConfig defines rate limit configuration for specific endpoints
type EndpointConfig struct {
	Path  string
	Rate  limiter.Rate
	Tiers *TierLimits // Optional tier-specific overrides
	Burst *BurstConfig // Optional burst configuration
}

// Service provides rate limiting functionality
type Service struct {
	store           limiter.Store
	defaultRate     limiter.Rate
	tierLimits      TierLimits
	endpointConfigs map[string]*EndpointConfig
	redisClient     *goredis.Client
}

// Config holds the rate limiter configuration
type Config struct {
	RedisClient *goredis.Client
	DefaultRate limiter.Rate
	TierLimits  TierLimits
	Endpoints   []EndpointConfig
}

// NewService creates a new rate limiting service
func NewService(cfg Config) (*Service, error) {
	// Create Redis store
	store, err := redis.NewStore(cfg.RedisClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis store: %w", err)
	}

	// Build endpoint config map
	endpointConfigs := make(map[string]*EndpointConfig)
	for i := range cfg.Endpoints {
		endpointConfigs[cfg.Endpoints[i].Path] = &cfg.Endpoints[i]
	}

	return &Service{
		store:           store,
		defaultRate:     cfg.DefaultRate,
		tierLimits:      cfg.TierLimits,
		endpointConfigs: endpointConfigs,
		redisClient:     cfg.RedisClient,
	}, nil
}

// CheckLimit checks if a request is allowed based on rate limits
func (s *Service) CheckLimit(ctx context.Context, userID, endpoint, tier string) (allowed bool, remaining int, resetTime time.Time, err error) {
	// Determine the rate to use
	rate := s.getRate(endpoint, tier)

	// Create limiter instance
	limiterInstance := limiter.New(s.store, rate)

	// Create context key
	key := s.buildKey(userID, endpoint)

	// Get the limit context
	limitCtx, err := limiterInstance.Get(ctx, key)
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to get limit: %w", err)
	}

	// Check if limit is reached
	allowed = limitCtx.Remaining > 0
	remaining = int(limitCtx.Remaining)
	resetTime = time.Unix(0, limitCtx.Reset)

	return allowed, remaining, resetTime, nil
}

// CheckLimitForIP checks rate limit for IP address (anonymous users)
func (s *Service) CheckLimitForIP(ctx context.Context, ip, endpoint string) (allowed bool, remaining int, resetTime time.Time, err error) {
	// Use lower rate for anonymous users
	rate := s.getAnonymousRate(endpoint)

	// Create limiter instance
	limiterInstance := limiter.New(s.store, rate)

	// Create context key
	key := fmt.Sprintf("ip:%s:%s", ip, endpoint)

	// Get the limit context
	limitCtx, err := limiterInstance.Get(ctx, key)
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to get IP limit: %w", err)
	}

	// Check if limit is reached
	allowed = limitCtx.Remaining > 0
	remaining = int(limitCtx.Remaining)
	resetTime = time.Unix(0, limitCtx.Reset)

	return allowed, remaining, resetTime, nil
}

// ResetLimit resets the rate limit for a user
func (s *Service) ResetLimit(ctx context.Context, userID, endpoint string) error {
	key := s.buildKey(userID, endpoint)
	return s.redisClient.Del(ctx, key).Err()
}

// ResetAllLimits resets all limits for a user
func (s *Service) ResetAllLimits(ctx context.Context, userID string) error {
	pattern := fmt.Sprintf("rate:user:%s:*", userID)

	// Find all keys matching the pattern
	keys, err := s.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to find keys: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	// Delete all found keys
	return s.redisClient.Del(ctx, keys...).Err()
}

// getRate determines the rate limit based on endpoint and tier
func (s *Service) getRate(endpoint, tier string) limiter.Rate {
	// Check for endpoint-specific config first
	if cfg, exists := s.endpointConfigs[endpoint]; exists {
		// Check for tier-specific override for this endpoint
		if cfg.Tiers != nil {
			switch tier {
			case "premium":
				return cfg.Tiers.Premium
			case "enterprise":
				return cfg.Tiers.Enterprise
			case "free":
				return cfg.Tiers.Free
			}
		}
		return cfg.Rate
	}

	// Fall back to tier-based limits
	switch tier {
	case "premium":
		return s.tierLimits.Premium
	case "enterprise":
		return s.tierLimits.Enterprise
	case "free":
		return s.tierLimits.Free
	default:
		return s.defaultRate
	}
}

// getAnonymousRate returns rate limit for anonymous users
func (s *Service) getAnonymousRate(endpoint string) limiter.Rate {
	// Check for endpoint-specific config
	if cfg, exists := s.endpointConfigs[endpoint]; exists {
		// Use half the rate for anonymous users
		rate := cfg.Rate
		rate.Limit = rate.Limit / 2
		return rate
	}

	// Use half of default rate for anonymous
	rate := s.defaultRate
	rate.Limit = rate.Limit / 2
	return rate
}

// buildKey creates a Redis key for rate limiting
func (s *Service) buildKey(userID, endpoint string) string {
	if endpoint == "" {
		return fmt.Sprintf("rate:user:%s:global", userID)
	}
	return fmt.Sprintf("rate:user:%s:%s", userID, endpoint)
}

// GetLimitInfo returns limit information without consuming
func (s *Service) GetLimitInfo(ctx context.Context, userID, endpoint, tier string) (*limiter.Context, error) {
	rate := s.getRate(endpoint, tier)
	limiterInstance := limiter.New(s.store, rate)
	key := s.buildKey(userID, endpoint)

	result, err := limiterInstance.Peek(ctx, key)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CheckBurst checks if a burst is allowed for the given key
func (s *Service) CheckBurst(ctx context.Context, userID, endpoint string) (allowed bool, err error) {
	// Check if endpoint has burst configuration
	cfg, exists := s.endpointConfigs[endpoint]
	if !exists || cfg.Burst == nil || !cfg.Burst.AllowBurst {
		return true, nil // No burst config, allow by default
	}

	// Build burst key
	burstKey := fmt.Sprintf("burst:%s:%s", userID, endpoint)

	// Get current burst count
	val, err := s.redisClient.Get(ctx, burstKey).Int64()
	if err != nil && err.Error() != "redis: nil" {
		return false, fmt.Errorf("failed to get burst count: %w", err)
	}

	// Check if burst limit is exceeded
	if val >= cfg.Burst.BurstSize {
		return false, nil
	}

	// Increment burst counter
	pipe := s.redisClient.Pipeline()
	pipe.Incr(ctx, burstKey)
	pipe.Expire(ctx, burstKey, cfg.Burst.BurstPeriod)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to increment burst counter: %w", err)
	}

	return true, nil
}

// GetBurstInfo returns current burst information
func (s *Service) GetBurstInfo(ctx context.Context, userID, endpoint string) (current int64, limit int64, err error) {
	cfg, exists := s.endpointConfigs[endpoint]
	if !exists || cfg.Burst == nil {
		return 0, 0, nil
	}

	burstKey := fmt.Sprintf("burst:%s:%s", userID, endpoint)
	val, err := s.redisClient.Get(ctx, burstKey).Int64()
	if err != nil && err.Error() != "redis: nil" {
		return 0, 0, fmt.Errorf("failed to get burst info: %w", err)
	}

	return val, cfg.Burst.BurstSize, nil
}

// ResetBurst resets the burst counter for a user
func (s *Service) ResetBurst(ctx context.Context, userID, endpoint string) error {
	burstKey := fmt.Sprintf("burst:%s:%s", userID, endpoint)
	return s.redisClient.Del(ctx, burstKey).Err()
}

// CreateDefaultConfig creates a default rate limiter configuration
func CreateDefaultConfig(redisClient *goredis.Client) Config {
	return Config{
		RedisClient: redisClient,
		DefaultRate: limiter.Rate{
			Period: 1 * time.Hour,
			Limit:  100, // 100 requests per hour by default
		},
		TierLimits: TierLimits{
			Free: limiter.Rate{
				Period: 1 * time.Hour,
				Limit:  100,
			},
			Premium: limiter.Rate{
				Period: 1 * time.Hour,
				Limit:  1000,
			},
			Enterprise: limiter.Rate{
				Period: 1 * time.Hour,
				Limit:  10000,
			},
		},
		Endpoints: []EndpointConfig{
			{
				Path: "/api/v1/auth/login",
				Rate: limiter.Rate{
					Period: 5 * time.Minute,
					Limit:  10, // 10 login attempts per 5 minutes
				},
				Burst: &BurstConfig{
					AllowBurst:  true,
					BurstSize:   3, // Allow only 3 burst requests
					BurstPeriod: 30 * time.Second, // Within 30 seconds
				},
			},
			{
				Path: "/api/v1/auth/register",
				Rate: limiter.Rate{
					Period: 1 * time.Hour,
					Limit:  5, // 5 registrations per hour
				},
				Burst: &BurstConfig{
					AllowBurst:  true,
					BurstSize:   2, // Allow only 2 burst requests
					BurstPeriod: 1 * time.Minute, // Within 1 minute
				},
			},
			{
				Path: "/api/v1/auth/reset-password",
				Rate: limiter.Rate{
					Period: 1 * time.Hour,
					Limit:  3, // 3 reset attempts per hour
				},
				Burst: &BurstConfig{
					AllowBurst:  true,
					BurstSize:   1, // Allow only 1 burst request
					BurstPeriod: 1 * time.Minute, // Within 1 minute
				},
			},
			{
				Path: "/api/v1/checkins",
				Rate: limiter.Rate{
					Period: 1 * time.Minute,
					Limit:  60, // 60 checkins per minute
				},
				Tiers: &TierLimits{
					Free: limiter.Rate{
						Period: 1 * time.Minute,
						Limit:  30,
					},
					Premium: limiter.Rate{
						Period: 1 * time.Minute,
						Limit:  100,
					},
					Enterprise: limiter.Rate{
						Period: 1 * time.Minute,
						Limit:  1000,
					},
				},
			},
		},
	}
}
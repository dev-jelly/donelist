package signing

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NonceTracker tracks used nonces to prevent replay attacks
type NonceTracker struct {
	client *redis.Client
	ttl    time.Duration
}

// NewNonceTracker creates a new nonce tracker with Redis backend
func NewNonceTracker(client *redis.Client, ttl time.Duration) *NonceTracker {
	return &NonceTracker{
		client: client,
		ttl:    ttl,
	}
}

// CheckAndRecordNonce checks if a nonce has been used and records it if not
// Returns nil if the nonce is valid and has been recorded
// Returns an error if the nonce has already been used
func (nt *NonceTracker) CheckAndRecordNonce(ctx context.Context, nonce string) error {
	if nonce == "" {
		return fmt.Errorf("nonce cannot be empty")
	}

	// Create a unique key for this nonce
	key := fmt.Sprintf("signing:nonce:%s", nonce)

	// Try to set the key only if it doesn't exist (NX flag)
	// This is atomic in Redis
	result, err := nt.client.SetNX(ctx, key, "1", nt.ttl).Result()
	if err != nil {
		return fmt.Errorf("failed to check nonce in Redis: %w", err)
	}

	// If result is false, the key already existed (nonce was already used)
	if !result {
		return fmt.Errorf("nonce has already been used (replay attack detected)")
	}

	return nil
}

// IsNonceUsed checks if a nonce has been used without recording it
func (nt *NonceTracker) IsNonceUsed(ctx context.Context, nonce string) (bool, error) {
	if nonce == "" {
		return false, fmt.Errorf("nonce cannot be empty")
	}

	key := fmt.Sprintf("signing:nonce:%s", nonce)

	result, err := nt.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check nonce existence in Redis: %w", err)
	}

	return result > 0, nil
}

// GenerateNonce generates a cryptographically secure random nonce
func GenerateNonce() (string, error) {
	// Generate 16 bytes (128 bits) of random data
	nonce := make([]byte, 16)
	if _, err := cryptoRandRead(nonce); err != nil {
		return "", fmt.Errorf("failed to generate random nonce: %w", err)
	}

	// Return hex-encoded nonce
	return fmt.Sprintf("%x", nonce), nil
}

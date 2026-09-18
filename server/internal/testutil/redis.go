package testutil

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestRedis represents a test Redis instance using miniredis
type TestRedis struct {
	Server *miniredis.Miniredis
	Client *redis.Client
}

// SetupTestRedis creates a new miniredis instance and returns a test Redis client
func SetupTestRedis(t *testing.T) *TestRedis {
	// Create miniredis server
	server := miniredis.RunT(t)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})

	return &TestRedis{
		Server: server,
		Client: client,
	}
}

// Close closes the Redis client and server
func (tr *TestRedis) Close() {
	if tr.Client != nil {
		tr.Client.Close()
	}
	if tr.Server != nil {
		tr.Server.Close()
	}
}

// Flush clears all data from Redis
func (tr *TestRedis) Flush(t *testing.T) {
	tr.Server.FlushAll()
}

// FastForward advances time in Redis (useful for TTL testing)
func (tr *TestRedis) FastForward(t *testing.T, d time.Duration) {
	tr.Server.FastForward(d)
}

// SetTime sets the current time in Redis
func (tr *TestRedis) SetTime(t *testing.T, timestamp time.Time) {
	tr.Server.SetTime(timestamp)
}

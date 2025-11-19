package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// NewRedis creates a new Redis client
func NewRedis(cfg RedisConfig, logger *zap.Logger) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	logger.Info("Connecting to Redis",
		zap.String("address", addr),
		zap.Int("db", cfg.DB),
	)

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	logger.Info("Successfully connected to Redis")
	return client, nil
}

// CloseRedis closes the Redis connection
func CloseRedis(client *redis.Client, logger *zap.Logger) error {
	if client == nil {
		return nil
	}
	logger.Info("Closing Redis connection")
	return client.Close()
}

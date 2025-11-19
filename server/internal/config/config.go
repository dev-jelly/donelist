package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	CORS     CORSConfig
	RateLimit RateLimitConfig
	Stripe   StripeConfig
}

// AppConfig holds application-level configuration
type AppConfig struct {
	Env      string
	LogLevel string
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port           string
	WSPort         string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret              string
	AccessTokenExpiry   time.Duration
	RefreshTokenExpiry  time.Duration
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int // Preflight cache duration in seconds
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int
}

// StripeConfig holds Stripe payment configuration
type StripeConfig struct {
	SecretKey     string
	WebhookSecret string
	// Product sync settings
	SyncProductsOnStartup bool
	// Test mode detection is automatic based on key prefix
}

// Load loads configuration from environment variables and .env file
func Load() (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Read from .env file
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("..")
	v.AddConfigPath("../..")

	// Read config file (optional, will not error if not found)
	_ = v.ReadInConfig()

	// Read from environment variables (overrides file)
	v.AutomaticEnv()

	// Parse configuration
	cfg := &Config{
		App: AppConfig{
			Env:      v.GetString("APP_ENV"),
			LogLevel: v.GetString("LOG_LEVEL"),
		},
		Server: ServerConfig{
			Port:           v.GetString("APP_PORT"),
			WSPort:         v.GetString("WS_PORT"),
			ReadTimeout:    v.GetDuration("SERVER_READ_TIMEOUT"),
			WriteTimeout:   v.GetDuration("SERVER_WRITE_TIMEOUT"),
			MaxHeaderBytes: v.GetInt("SERVER_MAX_HEADER_BYTES"),
		},
		Database: DatabaseConfig{
			Host:     v.GetString("POSTGRES_HOST"),
			Port:     v.GetInt("POSTGRES_PORT"),
			User:     v.GetString("POSTGRES_USER"),
			Password: v.GetString("POSTGRES_PASSWORD"),
			Name:     v.GetString("POSTGRES_DB"),
			SSLMode:  v.GetString("POSTGRES_SSLMODE"),
		},
		Redis: RedisConfig{
			Host:     v.GetString("REDIS_HOST"),
			Port:     v.GetInt("REDIS_PORT"),
			Password: v.GetString("REDIS_PASSWORD"),
			DB:       v.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			Secret:              v.GetString("JWT_SECRET"),
			AccessTokenExpiry:   v.GetDuration("JWT_ACCESS_TOKEN_EXPIRY"),
			RefreshTokenExpiry:  v.GetDuration("JWT_REFRESH_TOKEN_EXPIRY"),
		},
		CORS: CORSConfig{
			AllowedOrigins:   v.GetStringSlice("CORS_ALLOWED_ORIGINS"),
			AllowedMethods:   v.GetStringSlice("CORS_ALLOWED_METHODS"),
			AllowedHeaders:   v.GetStringSlice("CORS_ALLOWED_HEADERS"),
			ExposedHeaders:   v.GetStringSlice("CORS_EXPOSE_HEADERS"),
			AllowCredentials: v.GetBool("CORS_ALLOW_CREDENTIALS"),
			MaxAge:           v.GetInt("CORS_MAX_AGE"),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: v.GetInt("RATE_LIMIT_REQUESTS_PER_MINUTE"),
		},
		Stripe: StripeConfig{
			SecretKey:             v.GetString("STRIPE_SECRET_KEY"),
			WebhookSecret:         v.GetString("STRIPE_WEBHOOK_SECRET"),
			SyncProductsOnStartup: v.GetBool("STRIPE_SYNC_PRODUCTS_ON_STARTUP"),
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("LOG_LEVEL", "info")

	// Server defaults
	v.SetDefault("APP_PORT", "8080")
	v.SetDefault("WS_PORT", "8081")
	v.SetDefault("SERVER_READ_TIMEOUT", 10*time.Second)
	v.SetDefault("SERVER_WRITE_TIMEOUT", 10*time.Second)
	v.SetDefault("SERVER_MAX_HEADER_BYTES", 1<<20) // 1 MB

	// Database defaults
	v.SetDefault("POSTGRES_HOST", "localhost")
	v.SetDefault("POSTGRES_PORT", 5432)
	v.SetDefault("POSTGRES_SSLMODE", "disable")

	// Redis defaults
	v.SetDefault("REDIS_HOST", "localhost")
	v.SetDefault("REDIS_PORT", 6379)
	v.SetDefault("REDIS_DB", 0)

	// JWT defaults
	v.SetDefault("JWT_ACCESS_TOKEN_EXPIRY", 15*time.Minute)
	v.SetDefault("JWT_REFRESH_TOKEN_EXPIRY", 168*time.Hour) // 7 days

	// CORS defaults
	v.SetDefault("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://localhost:5173"})
	v.SetDefault("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"})
	v.SetDefault("CORS_ALLOWED_HEADERS", []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"})
	v.SetDefault("CORS_EXPOSE_HEADERS", []string{"Link"})
	v.SetDefault("CORS_ALLOW_CREDENTIALS", true)
	v.SetDefault("CORS_MAX_AGE", 300) // 5 minutes

	// Rate limit defaults
	v.SetDefault("RATE_LIMIT_REQUESTS_PER_MINUTE", 60)

	// Stripe defaults
	v.SetDefault("STRIPE_SYNC_PRODUCTS_ON_STARTUP", false)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate JWT secret
	if c.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}

	// Validate database configuration
	if c.Database.Host == "" {
		return fmt.Errorf("POSTGRES_HOST is required")
	}
	if c.Database.User == "" {
		return fmt.Errorf("POSTGRES_USER is required")
	}
	if c.Database.Password == "" {
		return fmt.Errorf("POSTGRES_PASSWORD is required")
	}
	if c.Database.Name == "" {
		return fmt.Errorf("POSTGRES_DB is required")
	}

	return nil
}

// DSN returns PostgreSQL connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}

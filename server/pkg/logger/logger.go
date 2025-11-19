package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ContextKey is the type for context keys
type contextKey string

const (
	// LoggerKey is the context key for the logger
	LoggerKey contextKey = "logger"
	// CorrelationIDKey is the context key for correlation IDs
	CorrelationIDKey contextKey = "correlation_id"
	// RequestIDKey is the context key for request IDs
	RequestIDKey contextKey = "request_id"
	// UserIDKey is the context key for user IDs
	UserIDKey contextKey = "user_id"
)

// New creates a new logger instance
func New(env, level string) (*zap.Logger, error) {
	var config zap.Config

	// Configure based on environment
	if env == "production" {
		config = zap.NewProductionConfig()
		// Use JSON encoding in production for better log aggregation
		config.Encoding = "json"
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Set log level
	config.Level = zap.NewAtomicLevelAt(parseLogLevel(level))

	// Build logger
	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return logger, nil
}

// parseLogLevel converts string log level to zapcore.Level
func parseLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// NewNop returns a no-op logger (for testing)
func NewNop() *zap.Logger {
	return zap.NewNop()
}

// WithFields returns a logger with additional fields
func WithFields(logger *zap.Logger, fields map[string]interface{}) *zap.Logger {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	return logger.With(zapFields...)
}

// WithCorrelationID returns a logger with correlation ID field
func WithCorrelationID(logger *zap.Logger, correlationID string) *zap.Logger {
	return logger.With(zap.String("correlation_id", correlationID))
}

// WithRequestID returns a logger with request ID field
func WithRequestID(logger *zap.Logger, requestID string) *zap.Logger {
	return logger.With(zap.String("request_id", requestID))
}

// WithUserID returns a logger with user ID field
func WithUserID(logger *zap.Logger, userID string) *zap.Logger {
	return logger.With(zap.String("user_id", userID))
}

// FromContext extracts logger from context
func FromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(LoggerKey).(*zap.Logger); ok {
		return logger
	}
	// Return global logger as fallback
	return zap.L()
}

// ToContext adds logger to context
func ToContext(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, logger)
}

// GetCorrelationID extracts correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return correlationID
	}
	return ""
}

// SetCorrelationID adds correlation ID to context
func SetCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// GetRequestID extracts request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// SetRequestID adds request ID to context
func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// SetUserID adds user ID to context
func SetUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// WithContext creates a child logger with context information
func WithContext(logger *zap.Logger, ctx context.Context) *zap.Logger {
	fields := make([]zap.Field, 0, 3)

	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		fields = append(fields, zap.String("correlation_id", correlationID))
	}

	if requestID := GetRequestID(ctx); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}

	if userID := GetUserID(ctx); userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}

	if len(fields) > 0 {
		return logger.With(fields...)
	}

	return logger
}

// Sync flushes any buffered log entries
func Sync(logger *zap.Logger) {
	if logger != nil {
		_ = logger.Sync()
	}
}

// Fatal logs a fatal message and exits
func Fatal(logger *zap.Logger, msg string, fields ...zap.Field) {
	logger.Fatal(msg, fields...)
	os.Exit(1)
}

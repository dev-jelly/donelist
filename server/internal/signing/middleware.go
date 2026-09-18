package signing

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// HeaderSignature is the header containing the request signature
	HeaderSignature = "X-Signature"

	// HeaderTimestamp is the header containing the request timestamp
	HeaderTimestamp = "X-Timestamp"

	// HeaderNonce is the header containing the request nonce
	HeaderNonce = "X-Nonce"

	// HeaderKeyID is the header containing the signing key ID
	HeaderKeyID = "X-Key-ID"

	// DefaultMaxAgeSeconds is the default maximum age for signatures (5 minutes)
	DefaultMaxAgeSeconds = 300
)

// SignatureMiddlewareConfig holds configuration for signature verification
type SignatureMiddlewareConfig struct {
	KeyManager   *KeyManager
	NonceTracker *NonceTracker
	Signer       *Signer
	MaxAgeSeconds int64
	Logger       *zap.Logger
	// SkipPaths allows certain paths to skip signature verification
	SkipPaths []string
}

// SignatureMiddleware creates a Gin middleware that verifies request signatures
func SignatureMiddleware(config SignatureMiddlewareConfig) gin.HandlerFunc {
	// Set defaults
	if config.MaxAgeSeconds == 0 {
		config.MaxAgeSeconds = DefaultMaxAgeSeconds
	}

	if config.Signer == nil {
		config.Signer = NewSigner(AlgorithmHMACSHA256)
	}

	return func(c *gin.Context) {
		// Check if path should skip signature verification
		for _, path := range config.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Extract signature headers
		signature := c.GetHeader(HeaderSignature)
		timestampStr := c.GetHeader(HeaderTimestamp)
		nonce := c.GetHeader(HeaderNonce)
		keyIDStr := c.GetHeader(HeaderKeyID)

		// Validate required headers
		if signature == "" {
			config.Logger.Warn("Missing signature header",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing_signature",
				"message": "Request signature is required for this endpoint",
			})
			return
		}

		if timestampStr == "" {
			config.Logger.Warn("Missing timestamp header",
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing_timestamp",
				"message": "Request timestamp is required",
			})
			return
		}

		if nonce == "" {
			config.Logger.Warn("Missing nonce header",
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing_nonce",
				"message": "Request nonce is required",
			})
			return
		}

		if keyIDStr == "" {
			config.Logger.Warn("Missing key ID header",
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing_key_id",
				"message": "Signing key ID is required",
			})
			return
		}

		// Parse timestamp
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			config.Logger.Warn("Invalid timestamp format",
				zap.String("timestamp", timestampStr),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "invalid_timestamp",
				"message": "Timestamp must be a valid Unix timestamp",
			})
			return
		}

		// Verify timestamp is within allowed window
		if err := config.Signer.VerifyTimestamp(timestamp, config.MaxAgeSeconds); err != nil {
			config.Logger.Warn("Timestamp verification failed",
				zap.Int64("timestamp", timestamp),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_timestamp",
				"message": "Request timestamp is outside allowed time window",
			})
			return
		}

		// Check and record nonce to prevent replay attacks
		if err := config.NonceTracker.CheckAndRecordNonce(c.Request.Context(), nonce); err != nil {
			config.Logger.Warn("Nonce verification failed",
				zap.String("nonce", nonce),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_nonce",
				"message": "Nonce has already been used or is invalid",
			})
			return
		}

		// Parse key ID
		keyID, err := uuid.Parse(keyIDStr)
		if err != nil {
			config.Logger.Warn("Invalid key ID format",
				zap.String("key_id", keyIDStr),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "invalid_key_id",
				"message": "Key ID must be a valid UUID",
			})
			return
		}

		// Retrieve signing key
		signingKey, err := config.KeyManager.GetKeyByID(c.Request.Context(), keyID)
		if err != nil {
			config.Logger.Warn("Signing key not found",
				zap.String("key_id", keyIDStr),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_key",
				"message": "Invalid signing key",
			})
			return
		}

		// Check if key is active
		if !signingKey.IsActive {
			config.Logger.Warn("Inactive signing key used",
				zap.String("key_id", keyIDStr),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "key_revoked",
				"message": "Signing key has been revoked",
			})
			return
		}

		// Check if key is expired
		if signingKey.ExpiresAt != nil && signingKey.ExpiresAt.Before(now()) {
			config.Logger.Warn("Expired signing key used",
				zap.String("key_id", keyIDStr),
				zap.Time("expires_at", *signingKey.ExpiresAt),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "key_expired",
				"message": "Signing key has expired",
			})
			return
		}

		// Read request body (we need to read it for signature verification)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			config.Logger.Error("Failed to read request body",
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "internal_error",
				"message": "Failed to process request",
			})
			return
		}

		// Restore body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// Verify signature using the key hash
		if err := config.Signer.VerifySignature(
			signingKey.KeyHash,
			c.Request.Method,
			c.Request.URL.Path,
			body,
			timestamp,
			nonce,
			signature,
		); err != nil {
			config.Logger.Warn("Signature verification failed",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("key_id", keyIDStr),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_signature",
				"message": "Request signature is invalid",
			})
			return
		}

		// Update last used timestamp (async, don't block request)
		go func() {
			if err := config.KeyManager.UpdateLastUsed(c.Request.Context(), keyID); err != nil {
				config.Logger.Error("Failed to update key last used timestamp",
					zap.String("key_id", keyIDStr),
					zap.Error(err),
				)
			}
		}()

		config.Logger.Debug("Request signature verified",
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.String("key_id", keyIDStr),
		)

		// Add signing key info to context for downstream handlers
		c.Set("signing_key_id", keyID)
		c.Set("signing_key_user_id", signingKey.UserID)

		c.Next()
	}
}

// now returns current time (extracted for testing)
var now = func() time.Time {
	return time.Now()
}

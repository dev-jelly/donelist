# Request Signing Package

This package provides HMAC-SHA256 request signing for the Donelist API to ensure request integrity and authenticity for sensitive operations.

## Components

### Core Files

- **signer.go**: HMAC-SHA256 signature generation and verification
- **key_manager.go**: Signing key lifecycle management with database storage
- **nonce_tracker.go**: Redis-based nonce tracking for replay attack prevention
- **middleware.go**: Gin middleware for automatic signature verification
- **crypto.go**: Cryptographic utilities

### Test Files

- **signer_test.go**: Tests for signature operations
- **key_manager_test.go**: Tests for key management
- **nonce_tracker_test.go**: Tests for nonce validation
- **middleware_test.go**: Integration tests for HTTP middleware

## Usage

### 1. Initialize Components

```go
import (
    "github.com/dev-jelly/donelist/internal/signing"
    "github.com/redis/go-redis/v9"
    "github.com/jmoiron/sqlx"
)

// Initialize key manager
keyManager := signing.NewKeyManager(db)

// Initialize nonce tracker
nonceTracker := signing.NewNonceTracker(redisClient, 5*time.Minute)

// Initialize signer
signer := signing.NewSigner(signing.AlgorithmHMACSHA256)
```

### 2. Add Middleware to Routes

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/dev-jelly/donelist/internal/signing"
)

// Configure signature middleware
signatureConfig := signing.SignatureMiddlewareConfig{
    KeyManager:    keyManager,
    NonceTracker:  nonceTracker,
    Signer:        signer,
    MaxAgeSeconds: 300, // 5 minutes
    Logger:        logger,
    SkipPaths:     []string{"/health", "/metrics"},
}

// Apply to sensitive routes
router.DELETE("/api/v1/users/me",
    authMiddleware,
    signing.SignatureMiddleware(signatureConfig),
    userHandler.DeleteMe,
)
```

### 3. Create Signing Keys

```go
userID := uuid.New()
keyName := "Production Key"
expiresAt := time.Now().Add(90 * 24 * time.Hour) // 90 days

key, keyValue, err := keyManager.CreateKey(ctx, userID, keyName, &expiresAt)
if err != nil {
    return err
}

// keyValue is only returned once - client must save it
fmt.Println("Key ID:", key.ID)
fmt.Println("Key Value:", keyValue) // Show to client once
```

### 4. Client-Side Signing

See `/docs/REQUEST_SIGNING.md` for complete client implementation examples.

## Security Features

### Request Integrity
- SHA-256 body hashing prevents tampering
- Path and method included in signature
- Base64-encoded signatures

### Replay Attack Prevention
- Unique nonces tracked in Redis
- 5-minute timestamp window
- 30-second clock skew tolerance

### Key Management
- SHA-256 hashed keys in database
- Key rotation and revocation support
- Optional expiration dates
- Audit logging for all operations

### Constant-Time Operations
- Signature comparison uses `subtle.ConstantTimeCompare`
- Prevents timing attacks

## Configuration

### Middleware Options

```go
type SignatureMiddlewareConfig struct {
    KeyManager    *KeyManager      // Required: Key management
    NonceTracker  *NonceTracker    // Required: Nonce tracking
    Signer        *Signer          // Optional: Defaults to HMAC-SHA256
    MaxAgeSeconds int64            // Optional: Defaults to 300 (5 min)
    Logger        *zap.Logger      // Required: Logging
    SkipPaths     []string         // Optional: Paths to skip
}
```

### Default Values

- **Algorithm**: HMAC-SHA256
- **Max Age**: 300 seconds (5 minutes)
- **Clock Skew**: 30 seconds
- **Nonce TTL**: Same as Max Age
- **Key Size**: 256 bits (32 bytes)

## Error Handling

The middleware returns structured error responses:

```json
{
  "error": "error_code",
  "message": "Human-readable message"
}
```

### Error Codes

- `missing_signature`: Signature header not present
- `missing_timestamp`: Timestamp header not present
- `missing_nonce`: Nonce header not present
- `missing_key_id`: Key ID header not present
- `invalid_timestamp`: Timestamp format or value invalid
- `invalid_nonce`: Nonce already used (replay attack)
- `invalid_key_id`: Key ID format invalid
- `invalid_key`: Key not found or validation failed
- `key_revoked`: Key has been revoked
- `key_expired`: Key has expired
- `invalid_signature`: Signature verification failed

## Testing

Run all tests:

```bash
go test ./internal/signing/... -v
```

Run specific test:

```bash
go test ./internal/signing/... -run TestSigner_SignRequest -v
```

Run with coverage:

```bash
go test ./internal/signing/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Database Schema

The package requires the `signing_keys` table (migration `000045_signing_keys.up.sql`):

```sql
CREATE TABLE signing_keys (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    key_hash VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    algorithm VARCHAR(50) NOT NULL DEFAULT 'hmac-sha256',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_used TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP
);
```

## Redis Keys

Nonces are stored with the key format:

```
signing:nonce:{nonce_value}
```

TTL matches the configured max age (default 5 minutes).

## Best Practices

### Key Management

1. **Rotate keys regularly** (e.g., every 90 days)
2. **Use different keys per environment**
3. **Store keys securely** (never in code or logs)
4. **Revoke compromised keys immediately**
5. **Monitor key usage statistics**

### Nonce Generation

1. **Use cryptographic random sources**
2. **Never reuse nonces**
3. **Generate 16 bytes minimum**
4. **Hex or base64 encode for transmission**

### Timestamp Handling

1. **Synchronize clocks with NTP**
2. **Use Unix timestamps (seconds)**
3. **Handle clock skew appropriately**
4. **Log timestamp validation failures**

### Audit Logging

1. **Log all signature failures**
2. **Monitor for abnormal patterns**
3. **Alert on replay attacks**
4. **Track key usage statistics**

## Performance Considerations

### Redis Operations
- Nonce checks are atomic (SetNX)
- O(1) complexity for nonce lookup
- TTL automatically cleans up old nonces

### Database Operations
- Key lookups use indexed queries
- Active keys cached if needed
- Last used updates are async

### Signature Verification
- Constant-time comparison prevents timing attacks
- HMAC-SHA256 is fast (<1ms typically)
- Body hashing is computed once per request

## OWASP Compliance

This implementation follows OWASP ASVS standards:

- ✅ V3.2: Replay attack prevention
- ✅ V6.2: Secure key generation and storage
- ✅ V6.2: Constant-time comparison
- ✅ V7.1: Audit logging
- ✅ V9.1: Request integrity verification
- ✅ V9.2: HMAC-based authentication

## Documentation

For complete API documentation and client implementation guides, see:

- `/docs/REQUEST_SIGNING.md` - Complete protocol documentation
- `/docs/API.md` - General API documentation

## Support

For issues or questions:
- GitHub: https://github.com/dev-jelly/donelist/issues
- Email: support@donelist.io

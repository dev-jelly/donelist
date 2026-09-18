# Request Signing Protocol

## Overview

The Donelist API implements HMAC-SHA256 request signing for sensitive operations to ensure request integrity and authenticity. This prevents tampering, replay attacks, and unauthorized access to critical endpoints.

## Use Cases

Request signing is **required** for the following sensitive operations:

- **Account Management**
  - Changing account password
  - Deleting user account
  - Updating sensitive profile settings

- **Administrative Operations**
  - Managing API keys
  - Revoking access tokens
  - Team ownership transfers

- **Payment Operations**
  - Payment method updates
  - Subscription changes
  - Billing information updates

## How It Works

### 1. Signing Key Generation

Clients must first generate a signing key through the API:

```bash
POST /api/v1/api-keys
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "name": "My Signing Key",
  "scopes": ["signing"],
  "expires_at": "2025-12-31T23:59:59Z"  // Optional
}
```

**Response:**
```json
{
  "id": "key-uuid",
  "key": "base64-encoded-signing-key",  // ⚠️ Save this! It's only shown once
  "name": "My Signing Key",
  "algorithm": "hmac-sha256",
  "is_active": true,
  "created_at": "2024-11-19T12:00:00Z"
}
```

**Important:** The actual signing key value is only returned once during creation. Store it securely.

### 2. Signing a Request

To sign a request, you need to:

1. **Generate a nonce**: A unique 16-byte random value (hex-encoded)
2. **Get current timestamp**: Unix timestamp in seconds
3. **Compute the signature**: HMAC-SHA256 over canonical request data

#### Canonical String Format

The signature is computed over the following data (newline-separated):

```
METHOD\nPATH\nTIMESTAMP\nNONCE\nBODY_HASH
```

Where:
- `METHOD`: HTTP method (e.g., `POST`, `DELETE`)
- `PATH`: Full request path (e.g., `/api/v1/users/me`)
- `TIMESTAMP`: Unix timestamp in seconds
- `NONCE`: Unique random value
- `BODY_HASH`: SHA-256 hex hash of request body (use empty string hash for GET/DELETE)

#### Example Implementation (JavaScript/Node.js)

```javascript
const crypto = require('crypto');

function signRequest(signingKey, method, path, body, timestamp, nonce) {
  // Hash the request body
  const bodyHash = crypto
    .createHash('sha256')
    .update(body || '')
    .digest('hex');

  // Create canonical string
  const stringToSign = `${method}\n${path}\n${timestamp}\n${nonce}\n${bodyHash}`;

  // Decode base64 signing key
  const keyBuffer = Buffer.from(signingKey, 'base64');

  // Compute HMAC-SHA256 signature
  const signature = crypto
    .createHmac('sha256', keyBuffer)
    .update(stringToSign)
    .digest('base64');

  return signature;
}

// Example usage
const signingKey = 'your-base64-signing-key';
const method = 'DELETE';
const path = '/api/v1/users/me';
const body = '';  // Empty for DELETE
const timestamp = Math.floor(Date.now() / 1000);
const nonce = crypto.randomBytes(16).toString('hex');

const signature = signRequest(signingKey, method, path, body, timestamp, nonce);
```

#### Example Implementation (Python)

```python
import hmac
import hashlib
import base64
import time
import secrets

def sign_request(signing_key, method, path, body, timestamp, nonce):
    # Hash the request body
    body_hash = hashlib.sha256((body or '').encode()).hexdigest()

    # Create canonical string
    string_to_sign = f"{method}\n{path}\n{timestamp}\n{nonce}\n{body_hash}"

    # Decode base64 signing key
    key_bytes = base64.b64decode(signing_key)

    # Compute HMAC-SHA256 signature
    signature = hmac.new(
        key_bytes,
        string_to_sign.encode(),
        hashlib.sha256
    ).digest()

    return base64.b64encode(signature).decode()

# Example usage
signing_key = 'your-base64-signing-key'
method = 'DELETE'
path = '/api/v1/users/me'
body = ''
timestamp = int(time.time())
nonce = secrets.token_hex(16)

signature = sign_request(signing_key, method, path, body, timestamp, nonce)
```

#### Example Implementation (Go)

```go
package main

import (
    "crypto/hmac"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "fmt"
    "time"
)

func signRequest(signingKey, method, path string, body []byte, timestamp int64, nonce string) (string, error) {
    // Hash the request body
    bodyHash := sha256.Sum256(body)
    bodyHashHex := hex.EncodeToString(bodyHash[:])

    // Create canonical string
    stringToSign := fmt.Sprintf("%s\n%s\n%d\n%s\n%s",
        method, path, timestamp, nonce, bodyHashHex)

    // Decode base64 signing key
    keyBytes, err := base64.StdEncoding.DecodeString(signingKey)
    if err != nil {
        return "", err
    }

    // Compute HMAC-SHA256 signature
    h := hmac.New(sha256.New, keyBytes)
    h.Write([]byte(stringToSign))
    signature := h.Sum(nil)

    return base64.StdEncoding.EncodeToString(signature), nil
}

func generateNonce() (string, error) {
    nonce := make([]byte, 16)
    if _, err := rand.Read(nonce); err != nil {
        return "", err
    }
    return hex.EncodeToString(nonce), nil
}

// Example usage
func main() {
    signingKey := "your-base64-signing-key"
    method := "DELETE"
    path := "/api/v1/users/me"
    body := []byte{}
    timestamp := time.Now().Unix()
    nonce, _ := generateNonce()

    signature, _ := signRequest(signingKey, method, path, body, timestamp, nonce)
    fmt.Println("Signature:", signature)
}
```

### 3. Making a Signed Request

Include the following headers with your request:

| Header | Description | Example |
|--------|-------------|---------|
| `X-Signature` | Base64-encoded HMAC-SHA256 signature | `abc123...` |
| `X-Timestamp` | Unix timestamp in seconds | `1700000000` |
| `X-Nonce` | Unique random value (hex) | `a1b2c3d4...` |
| `X-Key-ID` | UUID of the signing key | `550e8400-...` |
| `Authorization` | JWT bearer token (still required) | `Bearer eyJ...` |

**Example cURL Request:**

```bash
curl -X DELETE https://api.donelist.io/api/v1/users/me \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "X-Signature: ${SIGNATURE}" \
  -H "X-Timestamp: ${TIMESTAMP}" \
  -H "X-Nonce: ${NONCE}" \
  -H "X-Key-ID: ${KEY_ID}"
```

## Security Features

### 1. Replay Attack Prevention

**Nonce Tracking:** Each nonce can only be used once within the time window. The server tracks used nonces in Redis.

**Time Window:** Requests must be signed within 5 minutes. Timestamps outside this window are rejected.

### 2. Request Integrity

**Body Hashing:** The request body is hashed and included in the signature, preventing tampering.

**Path Protection:** The request path is signed, preventing path manipulation.

### 3. Key Management

**Key Rotation:** Keys can be rotated by creating new keys and revoking old ones.

**Expiration:** Keys can have an optional expiration date.

**Revocation:** Keys can be immediately revoked if compromised.

## Error Responses

### Missing Signature
```json
{
  "error": "missing_signature",
  "message": "Request signature is required for this endpoint"
}
```
**HTTP Status:** `401 Unauthorized`

### Invalid Signature
```json
{
  "error": "invalid_signature",
  "message": "Request signature is invalid"
}
```
**HTTP Status:** `401 Unauthorized`

### Timestamp Too Old
```json
{
  "error": "invalid_timestamp",
  "message": "Request timestamp is outside allowed time window"
}
```
**HTTP Status:** `401 Unauthorized`

### Nonce Already Used (Replay Attack)
```json
{
  "error": "invalid_nonce",
  "message": "Nonce has already been used or is invalid"
}
```
**HTTP Status:** `401 Unauthorized`

### Key Revoked
```json
{
  "error": "key_revoked",
  "message": "Signing key has been revoked"
}
```
**HTTP Status:** `401 Unauthorized`

### Key Expired
```json
{
  "error": "key_expired",
  "message": "Signing key has expired"
}
```
**HTTP Status:** `401 Unauthorized`

## Best Practices

### 1. Key Storage

- **Never commit keys to version control**
- **Store keys in secure environment variables or secret managers**
- **Rotate keys regularly (e.g., every 90 days)**
- **Use different keys for different environments (dev/staging/prod)**

### 2. Clock Synchronization

- **Ensure system clocks are synchronized with NTP**
- **The server allows 30 seconds of clock skew**
- **Test your implementation with various timestamps**

### 3. Nonce Generation

- **Use cryptographically secure random number generators**
- **Never reuse nonces within the 5-minute window**
- **Generate fresh nonces for each request**

### 4. Error Handling

- **Implement retry logic with exponential backoff**
- **Log signature failures for debugging**
- **Monitor for abnormal failure rates**

### 5. Testing

```bash
# Test signature verification with a known good signature
# Use the examples above to generate test signatures

# Test replay attack prevention
# Try sending the same signature twice

# Test timestamp validation
# Send requests with old timestamps

# Test nonce validation
# Reuse nonces within the time window
```

## Troubleshooting

### Signature Mismatch

**Common causes:**
1. Incorrect canonical string format (check newlines)
2. Body not matching what was signed (encoding issues)
3. Wrong signing key or incorrect base64 decoding
4. Timestamp or nonce not matching headers

**Debug steps:**
1. Log the canonical string being signed
2. Verify body hash matches on both sides
3. Check that all components match exactly

### Timestamp Rejected

**Common causes:**
1. System clock not synchronized
2. Network latency causing delays
3. Using milliseconds instead of seconds

**Debug steps:**
1. Check system time: `date +%s`
2. Compare with server time from response headers
3. Use NTP to synchronize clocks

### Nonce Already Used

**Common causes:**
1. Retry logic reusing nonces
2. Load balancer duplicating requests
3. Client bugs sending duplicate requests

**Debug steps:**
1. Generate new nonce for each request
2. Check for duplicate request logic
3. Review retry mechanisms

## API Key Management

### List Signing Keys

```bash
GET /api/v1/api-keys
Authorization: Bearer {access_token}
```

### Get Key Details

```bash
GET /api/v1/api-keys/{key_id}
Authorization: Bearer {access_token}
```

### Revoke a Key

```bash
POST /api/v1/api-keys/{key_id}/revoke
Authorization: Bearer {access_token}
```

### Rotate a Key

```bash
POST /api/v1/api-keys/{key_id}/rotate
Authorization: Bearer {access_token}
```

**Response includes new key value (save it!):**
```json
{
  "id": "new-key-uuid",
  "key": "new-base64-signing-key",
  "name": "My Signing Key (Rotated)",
  "algorithm": "hmac-sha256",
  "is_active": true,
  "created_at": "2024-11-19T14:00:00Z"
}
```

## Compliance

This implementation follows OWASP best practices for API security:

- ✅ HMAC-based request signing (OWASP ASVS V9.2)
- ✅ Replay attack prevention with nonces (OWASP ASVS V3.2)
- ✅ Timestamp validation (OWASP ASVS V3.2)
- ✅ Secure key generation (OWASP ASVS V6.2)
- ✅ Constant-time comparison (OWASP ASVS V6.2)
- ✅ Request integrity verification (OWASP ASVS V9.1)
- ✅ Audit logging for failures (OWASP ASVS V7.1)

## Support

For issues or questions about request signing:

- GitHub Issues: https://github.com/dev-jelly/donelist/issues
- Email: support@donelist.io
- Documentation: https://docs.donelist.io

---

**Last Updated:** 2024-11-19
**Protocol Version:** 1.0
**Algorithm:** HMAC-SHA256

# User Settings and Profile Management API Documentation

## Overview

The User Settings and Profile Management API provides comprehensive endpoints for managing user profiles, privacy settings, security features (including 2FA), data exports, and account management.

## Table of Contents

1. [Authentication](#authentication)
2. [Profile Management](#profile-management)
3. [Two-Factor Authentication](#two-factor-authentication)
4. [Password Management](#password-management)
5. [Account Management](#account-management)
6. [Data Export](#data-export)
7. [Security Events](#security-events)
8. [Error Responses](#error-responses)

## Authentication

All endpoints require JWT authentication unless otherwise specified. Include the JWT token in the Authorization header:

```
Authorization: Bearer <jwt_token>
```

## Profile Management

### Get User Profile

Retrieve a user's profile with privacy settings applied.

**Endpoint:** `GET /api/users/:userId/profile`

**Response:**
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "bio": "Software developer passionate about productivity",
  "avatar_url": "https://storage.donelist.com/avatars/user123.jpg",
  "timezone": "America/New_York",
  "timezone_auto_detected": false,
  "locale": "en-US",
  "privacy_level": "public",
  "show_email": false,
  "show_activity": true,
  "searchable": true,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-11-24T00:00:00Z",
  "version": 3
}
```

**Privacy Levels:**
- `public`: Profile visible to all users
- `private`: Profile visible only to the owner
- `friends`: Profile visible to connected users (future feature)

### Update Profile

Update user profile information and privacy settings.

**Endpoint:** `PUT /api/settings/profile`

**Request:**
```json
{
  "bio": "Updated bio text",
  "timezone": "Europe/London",
  "locale": "en-GB",
  "privacy_level": "public",
  "show_email": false,
  "show_activity": true,
  "searchable": true
}
```

**Response:** Updated profile object

### Upload Avatar

Upload a new profile avatar image.

**Endpoint:** `POST /api/settings/profile/avatar`

**Request:** Multipart form data with `avatar` field

**Constraints:**
- Max file size: 5MB
- Supported formats: JPG, PNG, GIF, WebP
- Images are automatically resized and optimized

**Response:**
```json
{
  "avatar_url": "https://storage.donelist.com/avatars/uuid/filename.jpg"
}
```

### Remove Avatar

Delete the current profile avatar.

**Endpoint:** `DELETE /api/settings/profile/avatar`

**Response:**
```json
{
  "success": true
}
```

## Two-Factor Authentication

### Setup 2FA

Initialize 2FA setup and generate TOTP secret.

**Endpoint:** `POST /api/settings/2fa/setup`

**Response:**
```json
{
  "secret": "ABCDEFGHIJ123456",
  "qr_code": "base64_encoded_png_image",
  "backup_codes": [
    "ABC12345",
    "DEF67890",
    "GHI13579",
    "JKL24680",
    "MNO36912",
    "PQR48024",
    "STU59136",
    "VWX60248",
    "YZA71350",
    "BCD82462"
  ],
  "recovery_url": "otpauth://totp/DoneList:user@example.com?secret=..."
}
```

### Verify and Enable 2FA

Verify the TOTP code and enable 2FA.

**Endpoint:** `POST /api/settings/2fa/verify`

**Request:**
```json
{
  "code": "123456"
}
```

**Response:**
```json
{
  "success": true
}
```

### Disable 2FA

Disable 2FA (requires current password).

**Endpoint:** `POST /api/settings/2fa/disable`

**Request:**
```json
{
  "password": "current_password"
}
```

**Response:**
```json
{
  "success": true
}
```

### Regenerate Backup Codes

Generate new set of backup codes (requires password).

**Endpoint:** `POST /api/settings/2fa/backup-codes`

**Request:**
```json
{
  "password": "current_password"
}
```

**Response:**
```json
{
  "backup_codes": [
    "NEW12345",
    "NEW67890",
    ...
  ]
}
```

## Password Management

### Change Password

Change the current user's password.

**Endpoint:** `POST /api/settings/password/change`

**Request:**
```json
{
  "current_password": "old_password",
  "new_password": "new_secure_password"
}
```

**Password Requirements:**
- Minimum 8 characters
- Must contain uppercase, lowercase, and numbers
- Cannot reuse last 5 passwords
- Cannot contain common weak passwords

**Response:**
```json
{
  "success": true
}
```

**Side Effects:**
- All refresh tokens are invalidated
- Password history is updated
- Security event is logged

### Request Password Reset

Initiate password reset process (no authentication required).

**Endpoint:** `POST /api/auth/password/reset`

**Request:**
```json
{
  "email": "user@example.com"
}
```

**Response:**
```json
{
  "message": "If the email exists, a reset link has been sent"
}
```

**Note:** Always returns success to prevent email enumeration

### Complete Password Reset

Complete password reset using token from email.

**Endpoint:** `POST /api/auth/password/reset/complete`

**Request:**
```json
{
  "token": "reset_token_from_email",
  "new_password": "new_secure_password"
}
```

**Response:**
```json
{
  "success": true
}
```

## Account Management

### Update Recovery Email

Set or update recovery email for account recovery.

**Endpoint:** `PUT /api/settings/account/recovery-email`

**Request:**
```json
{
  "email": "recovery@example.com",
  "password": "current_password"
}
```

**Response:**
```json
{
  "success": true
}
```

### Request Account Deletion

Initiate account deletion with 30-day grace period.

**Endpoint:** `POST /api/settings/account/delete`

**Request:**
```json
{
  "password": "current_password",
  "reason": "No longer using the service"
}
```

**Response:**
```json
{
  "message": "Account deletion scheduled. You have 30 days to cancel this request."
}
```

**Deletion Process:**
1. Request created with 30-day grace period
2. Warning email sent 7 days before deletion
3. Data archived for 90 days (GDPR compliance)
4. User record anonymized (not hard deleted)
5. All personal data removed

### Cancel Account Deletion

Cancel a pending account deletion request.

**Endpoint:** `POST /api/settings/account/delete/cancel`

**Response:**
```json
{
  "success": true
}
```

## Data Export

### Request Data Export

Request an export of all user data.

**Endpoint:** `POST /api/settings/export`

**Request:**
```json
{
  "format": "json"  // Options: "json", "csv", "pdf"
}
```

**Response:**
```json
{
  "id": "export_request_id",
  "user_id": "user_id",
  "status": "pending",
  "format": "json",
  "requested_at": "2024-11-24T00:00:00Z"
}
```

### Check Export Status

Get the status of a data export request.

**Endpoint:** `GET /api/settings/export/:requestId`

**Response:**
```json
{
  "id": "export_request_id",
  "user_id": "user_id",
  "status": "completed",
  "format": "json",
  "file_url": "https://storage.donelist.com/exports/file.json",
  "expires_at": "2024-12-01T00:00:00Z",
  "requested_at": "2024-11-24T00:00:00Z",
  "completed_at": "2024-11-24T00:05:00Z"
}
```

**Status Values:**
- `pending`: Request queued
- `processing`: Export being generated
- `completed`: Export ready for download
- `failed`: Export failed (check error field)
- `expired`: Export link expired

### Get Export History

Retrieve history of data export requests.

**Endpoint:** `GET /api/settings/export/history`

**Response:**
```json
[
  {
    "id": "export_request_id",
    "status": "completed",
    "format": "json",
    "requested_at": "2024-11-24T00:00:00Z",
    "completed_at": "2024-11-24T00:05:00Z"
  }
]
```

## Security Events

### Get Security Events

Retrieve recent security events for the user's account.

**Endpoint:** `GET /api/settings/security/events?limit=20`

**Response:**
```json
[
  {
    "id": "event_id",
    "user_id": "user_id",
    "event_type": "login",
    "event_details": {
      "method": "password",
      "location": "New York, US"
    },
    "ip_address": "192.168.1.1",
    "user_agent": "Mozilla/5.0...",
    "success": true,
    "created_at": "2024-11-24T00:00:00Z"
  }
]
```

**Event Types:**
- `login`: Login attempt
- `2fa_enabled`: 2FA enabled
- `2fa_disabled`: 2FA disabled
- `password_changed`: Password changed
- `password_reset_requested`: Password reset initiated
- `password_reset_completed`: Password reset completed
- `account_deletion_requested`: Account deletion requested
- `account_deletion_cancelled`: Account deletion cancelled
- `suspicious_activity`: Suspicious activity detected

## Settings Management

### Get User Settings

Retrieve all user settings (UI preferences, notifications, etc.).

**Endpoint:** `GET /api/settings`

**Response:**
```json
{
  "theme": "dark",
  "language": "en",
  "date_format": "YYYY-MM-DD",
  "time_format": "24h",
  "week_start_day": 1,
  "notifications": {
    "email_enabled": true,
    "push_enabled": false,
    "reminder_interval_minutes": 120,
    "dnd_enabled": false,
    "dnd_start_time": null,
    "dnd_end_time": null,
    "dnd_days": [1, 2, 3, 4, 5]
  },
  "data_retention": {
    "auto_delete_enabled": false,
    "retention_days": null,
    "delete_after_inactivity_days": 365
  }
}
```

### Update Settings

Update user settings.

**Endpoint:** `PUT /api/settings`

**Request:**
```json
{
  "theme": "light",
  "language": "es",
  "date_format": "DD/MM/YYYY",
  "time_format": "12h",
  "week_start_day": 0
}
```

**Response:** Updated settings object

## Error Responses

All endpoints use consistent error response format:

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": {
    "field": "Additional error context"
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `VALIDATION_ERROR` | 400 | Request validation failed |
| `CONFLICT` | 409 | Resource conflict (e.g., email already exists) |
| `RATE_LIMIT` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Internal server error |

### Rate Limiting

API endpoints are rate-limited to prevent abuse:

- General endpoints: 100 requests per minute
- Authentication endpoints: 5 requests per minute
- Export requests: 3 per hour
- Password reset: 3 per hour

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1700000000
```

## Webhooks

Users can configure webhooks to receive notifications about account events:

### Webhook Events

- `profile.updated`: Profile information changed
- `security.login`: New login detected
- `security.password_changed`: Password was changed
- `security.2fa_changed`: 2FA settings modified
- `account.deletion_requested`: Account deletion initiated
- `export.completed`: Data export ready for download

### Webhook Payload

```json
{
  "event": "profile.updated",
  "user_id": "uuid",
  "timestamp": "2024-11-24T00:00:00Z",
  "data": {
    "changes": ["bio", "timezone"]
  }
}
```

## Best Practices

1. **Security:**
   - Always use HTTPS
   - Implement proper CORS headers
   - Validate all input data
   - Use strong passwords
   - Enable 2FA for sensitive accounts

2. **Performance:**
   - Cache profile data when possible
   - Use ETags for conditional requests
   - Implement pagination for lists
   - Compress responses with gzip

3. **Error Handling:**
   - Handle rate limits gracefully
   - Implement exponential backoff for retries
   - Log errors for debugging
   - Provide meaningful error messages

4. **Privacy:**
   - Respect privacy settings
   - Minimize data collection
   - Implement data retention policies
   - Provide data export functionality
   - Support account deletion

## Migration Guide

For users migrating from older API versions:

1. Update authentication headers to use Bearer tokens
2. Update profile endpoints to use new privacy fields
3. Implement 2FA verification in login flow
4. Handle new error response format
5. Update webhook payloads parsing

## Support

For API support, contact:
- Email: api-support@donelist.com
- Documentation: https://docs.donelist.com/api
- Status Page: https://status.donelist.com
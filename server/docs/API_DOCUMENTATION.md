# Donelist API Documentation

## Overview

The Donelist API is a comprehensive REST API for managing check-ins, timelines, categories, tags, and user data. The API uses JWT-based authentication and provides real-time updates via WebSocket connections.

**Version:** 1.0
**Base URL:** `/api/v1`
**Swagger UI:** `/swagger/index.html`

## Authentication

All protected endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer <your_access_token>
```

### Authentication Flow

1. **Register** - Create a new user account
2. **Login** - Get access and refresh tokens
3. **Use Access Token** - Include in Authorization header for API requests
4. **Refresh Token** - Get a new access token when it expires
5. **Logout** - Revoke refresh token

## Example Requests and Responses

### Authentication

#### Register New User

```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!",
  "display_name": "John Doe"
}
```

**Response (201 Created):**

```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "email": "user@example.com",
    "display_name": "John Doe",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2024-01-15T11:30:00Z"
}
```

#### Login

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!"
}
```

**Response (200 OK):**

```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "email": "user@example.com",
    "display_name": "John Doe",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2024-01-15T11:30:00Z"
}
```

### Check-ins

#### Create Check-in

```http
POST /api/v1/checkins
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "content": "Completed morning workout routine",
  "category_id": "234e5678-e89b-12d3-a456-426614174001",
  "duration_minutes": 30,
  "tags": ["fitness", "morning", "routine"]
}
```

**Response (201 Created):**

```json
{
  "id": "345e6789-e89b-12d3-a456-426614174002",
  "user_id": "123e4567-e89b-12d3-a456-426614174000",
  "category_id": "234e5678-e89b-12d3-a456-426614174001",
  "content": "Completed morning workout routine",
  "checkin_time": "2024-01-15T10:45:00Z",
  "duration_minutes": 30,
  "tags": [
    {
      "id": "456e7890-e89b-12d3-a456-426614174003",
      "name": "fitness"
    },
    {
      "id": "567e8901-e89b-12d3-a456-426614174004",
      "name": "morning"
    },
    {
      "id": "678e9012-e89b-12d3-a456-426614174005",
      "name": "routine"
    }
  ],
  "created_at": "2024-01-15T10:45:00Z",
  "updated_at": "2024-01-15T10:45:00Z"
}
```

#### List Check-ins with Filtering

```http
GET /api/v1/checkins?start_date=2024-01-01T00:00:00Z&end_date=2024-01-31T23:59:59Z&limit=20&offset=0
Authorization: Bearer <access_token>
```

**Response (200 OK):**

```json
{
  "checkins": [
    {
      "id": "345e6789-e89b-12d3-a456-426614174002",
      "user_id": "123e4567-e89b-12d3-a456-426614174000",
      "category_id": "234e5678-e89b-12d3-a456-426614174001",
      "content": "Completed morning workout routine",
      "checkin_time": "2024-01-15T10:45:00Z",
      "duration_minutes": 30,
      "tags": [
        {
          "id": "456e7890-e89b-12d3-a456-426614174003",
          "name": "fitness"
        }
      ],
      "created_at": "2024-01-15T10:45:00Z",
      "updated_at": "2024-01-15T10:45:00Z"
    }
  ],
  "total": 45,
  "limit": 20,
  "offset": 0
}
```

### Categories

#### Create Category

```http
POST /api/v1/categories
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "name": "Fitness",
  "color": "#FF5733",
  "icon": "💪"
}
```

**Response (201 Created):**

```json
{
  "id": "234e5678-e89b-12d3-a456-426614174001",
  "user_id": "123e4567-e89b-12d3-a456-426614174000",
  "name": "Fitness",
  "color": "#FF5733",
  "icon": "💪",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

### Tags

#### Autocomplete Tags

```http
GET /api/v1/tags/autocomplete?q=fit&limit=5
Authorization: Bearer <access_token>
```

**Response (200 OK):**

```json
{
  "tags": [
    {
      "id": "456e7890-e89b-12d3-a456-426614174003",
      "name": "fitness",
      "usage_count": 25
    },
    {
      "id": "567e8901-e89b-12d3-a456-426614174004",
      "name": "fit",
      "usage_count": 12
    }
  ],
  "query": "fit",
  "count": 2
}
```

## Rate Limiting

The API implements rate limiting to ensure fair usage:

- **Rate Limit:** 100 requests per minute per user
- **Burst:** 20 requests
- **Headers:** Rate limit information is returned in response headers

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642251600
```

## Error Codes

| Status Code | Description |
|-------------|-------------|
| 200 | OK - Request successful |
| 201 | Created - Resource created successfully |
| 400 | Bad Request - Invalid request parameters |
| 401 | Unauthorized - Missing or invalid authentication |
| 403 | Forbidden - Insufficient permissions |
| 404 | Not Found - Resource not found |
| 409 | Conflict - Resource conflict (e.g., duplicate check-in) |
| 429 | Too Many Requests - Rate limit exceeded |
| 500 | Internal Server Error - Server error |

## Error Response Format

All error responses follow this format:

```json
{
  "error": "Error message describing what went wrong"
}
```

For validation errors with more context:

```json
{
  "error": "Duplicate check-in",
  "details": {
    "existing_checkin_id": "345e6789-e89b-12d3-a456-426614174002",
    "checkin_time": "2024-01-15T10:45:00Z",
    "allowed_interval_minutes": 15
  }
}
```

## Data Types

### Duration Minutes

Check-ins support the following duration values:
- `15` - 15 minutes
- `30` - 30 minutes
- `45` - 45 minutes
- `120` - 2 hours

### Date Format

All timestamps use RFC3339 format:
```
2024-01-15T10:45:00Z
```

### UUID Format

All IDs use UUID v4 format:
```
123e4567-e89b-12d3-a456-426614174000
```

## WebSocket Connection

For real-time updates, connect to the WebSocket endpoint:

```
ws://localhost:8080/ws?token=<access_token>
```

Messages will be pushed for:
- New check-ins
- Check-in updates
- Check-in deletions

## SDK Generation

You can generate client SDKs for various languages using the OpenAPI specification:

```bash
# Download the OpenAPI spec
curl http://localhost:8080/swagger/doc.json > openapi.json

# Generate TypeScript client
npx @openapitools/openapi-generator-cli generate \
  -i openapi.json \
  -g typescript-axios \
  -o ./sdk/typescript

# Generate Python client
npx @openapitools/openapi-generator-cli generate \
  -i openapi.json \
  -g python \
  -o ./sdk/python

# Generate Go client
npx @openapitools/openapi-generator-cli generate \
  -i openapi.json \
  -g go \
  -o ./sdk/go
```

## Postman Collection

Import the OpenAPI spec into Postman:

1. Open Postman
2. Click "Import"
3. Select "Link" tab
4. Enter: `http://localhost:8080/swagger/doc.json`
5. Click "Continue" and "Import"

## Testing the API

### Using cURL

```bash
# Register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Password123!","display_name":"Test User"}'

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Password123!"}'

# Create check-in (replace TOKEN with your access token)
curl -X POST http://localhost:8080/api/v1/checkins \
  -H "Authorization: Bearer TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"content":"Test check-in","duration_minutes":30,"tags":["test"]}'
```

## Best Practices

1. **Always use HTTPS in production**
2. **Store refresh tokens securely** (e.g., httpOnly cookies)
3. **Implement token refresh logic** before access tokens expire
4. **Handle rate limits** with exponential backoff
5. **Validate input** before sending requests
6. **Use WebSockets** for real-time updates instead of polling
7. **Implement proper error handling** for all error codes
8. **Cache responses** where appropriate to reduce API calls

## Support

For issues or questions:
- GitHub: https://github.com/dev-jelly/donelist
- Email: support@donelist.io

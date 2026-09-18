# API Request/Response Examples

This document provides comprehensive, realistic examples for all API endpoints including success and error scenarios.

## Table of Contents

- [Authentication](#authentication)
- [User Management](#user-management)
- [Check-ins](#check-ins)
- [Categories](#categories)
- [Tags](#tags)
- [Timeline](#timeline)
- [Search](#search)
- [Error Handling](#error-handling)

---

## Authentication

### Register User

**Request:**
```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "john.doe@example.com",
  "password": "SecureP@ssw0rd123!",
  "display_name": "John Doe"
}
```

**Success Response (201 Created):**
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john.doe@example.com",
    "display_name": "John Doe",
    "role": "user",
    "tier": "free",
    "mode_preference": "dark",
    "created_at": "2024-11-20T10:30:00Z",
    "updated_at": "2024-11-20T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiI1NTBlODQwMC1lMjliLTQxZDQtYTcxNi00NDY2NTU0NDAwMDAiLCJleHAiOjE3MDA0ODU4MDAsImlhdCI6MTcwMDQ4NDkwMH0.abc123",
  "refresh_token": "def456ghi789jkl012mno345pqr678stu901vwx234yz",
  "expires_at": "2024-11-20T11:30:00Z"
}
```

**Error Response - Weak Password (400 Bad Request):**
```json
{
  "error": "password must be at least 12 characters long"
}
```

**Error Response - Email Already Exists (409 Conflict):**
```json
{
  "error": "email already registered"
}
```

---

### Login

**Request:**
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "john.doe@example.com",
  "password": "SecureP@ssw0rd123!"
}
```

**Success Response (200 OK):**
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john.doe@example.com",
    "display_name": "John Doe",
    "role": "user",
    "tier": "premium",
    "tier_expires_at": "2025-11-20T10:30:00Z",
    "mode_preference": "dark",
    "created_at": "2024-11-20T10:30:00Z",
    "updated_at": "2024-11-20T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "def456ghi789jkl012mno345pqr678stu901vwx234yz",
  "expires_at": "2024-11-20T11:30:00Z"
}
```

**Error Response - Invalid Credentials (401 Unauthorized):**
```json
{
  "error": "invalid email or password"
}
```

---

### Refresh Token

**Request:**
```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "def456ghi789jkl012mno345pqr678stu901vwx234yz"
}
```

**Success Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "new789refresh012token345here678xyz",
  "expires_at": "2024-11-20T12:30:00Z"
}
```

**Error Response - Invalid Token (401 Unauthorized):**
```json
{
  "error": "invalid or expired refresh token"
}
```

---

### Logout

**Request:**
```http
POST /api/v1/auth/logout
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

{
  "refresh_token": "def456ghi789jkl012mno345pqr678stu901vwx234yz"
}
```

**Success Response (200 OK):**
```json
{
  "message": "logged out successfully"
}
```

---

## User Management

### Get Current User Profile

**Request:**
```http
GET /api/v1/users/me
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Success Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "john.doe@example.com",
  "display_name": "John Doe",
  "role": "user",
  "tier": "premium",
  "tier_expires_at": "2025-11-20T10:30:00Z",
  "mode_preference": "dark",
  "created_at": "2024-11-20T10:30:00Z",
  "updated_at": "2024-11-20T10:30:00Z"
}
```

**Error Response - Not Authenticated (401 Unauthorized):**
```json
{
  "error": "missing or invalid authorization header"
}
```

---

### Update User Profile

**Request:**
```http
PUT /api/v1/users/me
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

{
  "display_name": "Johnny Doe",
  "mode_preference": "light"
}
```

**Success Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "john.doe@example.com",
  "display_name": "Johnny Doe",
  "role": "user",
  "tier": "premium",
  "mode_preference": "light",
  "created_at": "2024-11-20T10:30:00Z",
  "updated_at": "2024-11-20T11:45:00Z"
}
```

---

## Check-ins

### Create Check-in

**Request:**
```http
POST /api/v1/checkins
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

{
  "title": "Morning workout completed",
  "description": "30-minute run in the park. Felt great!",
  "category_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "tags": ["fitness", "morning", "running"],
  "priority": "high",
  "location": {
    "latitude": 37.7749,
    "longitude": -122.4194,
    "name": "Golden Gate Park"
  },
  "mood": "energized",
  "weather": "sunny"
}
```

**Success Response (201 Created):**
```json
{
  "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "title": "Morning workout completed",
  "description": "30-minute run in the park. Felt great!",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "category_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "category": {
    "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
    "name": "Fitness",
    "color": "#FF6B6B",
    "icon": "dumbbell"
  },
  "tags": ["fitness", "morning", "running"],
  "priority": "high",
  "status": "completed",
  "location": {
    "latitude": 37.7749,
    "longitude": -122.4194,
    "name": "Golden Gate Park"
  },
  "mood": "energized",
  "weather": "sunny",
  "created_at": "2024-11-20T06:30:00Z",
  "updated_at": "2024-11-20T06:30:00Z"
}
```

**Error Response - Invalid Category (400 Bad Request):**
```json
{
  "error": "category not found or does not belong to user"
}
```

**Error Response - Validation Error (400 Bad Request):**
```json
{
  "error": "title is required and cannot be empty"
}
```

---

### List Check-ins with Pagination

**Request:**
```http
GET /api/v1/checkins?page=1&limit=20&category_id=6ba7b810-9dad-11d1-80b4-00c04fd430c8&status=completed
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Success Response (200 OK):**
```json
{
  "checkins": [
    {
      "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "title": "Morning workout completed",
      "description": "30-minute run in the park. Felt great!",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "category_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
      "category": {
        "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
        "name": "Fitness",
        "color": "#FF6B6B",
        "icon": "dumbbell"
      },
      "tags": ["fitness", "morning", "running"],
      "priority": "high",
      "status": "completed",
      "created_at": "2024-11-20T06:30:00Z",
      "updated_at": "2024-11-20T06:30:00Z"
    },
    {
      "id": "8d0f7780-8536-51ef-a055-f18ed2f91bf8",
      "title": "Evening yoga session",
      "description": "Relaxing 45-minute yoga practice",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "category_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
      "category": {
        "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
        "name": "Fitness",
        "color": "#FF6B6B",
        "icon": "dumbbell"
      },
      "tags": ["fitness", "evening", "yoga"],
      "priority": "normal",
      "status": "completed",
      "created_at": "2024-11-20T18:00:00Z",
      "updated_at": "2024-11-20T18:00:00Z"
    }
  ],
  "pagination": {
    "total": 45,
    "limit": 20,
    "offset": 0,
    "page": 1,
    "total_pages": 3
  }
}
```

---

### Get Single Check-in

**Request:**
```http
GET /api/v1/checkins/7c9e6679-7425-40de-944b-e07fc1f90ae7
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Success Response (200 OK):**
```json
{
  "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "title": "Morning workout completed",
  "description": "30-minute run in the park. Felt great!",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "category_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "category": {
    "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
    "name": "Fitness",
    "color": "#FF6B6B",
    "icon": "dumbbell"
  },
  "tags": ["fitness", "morning", "running"],
  "priority": "high",
  "status": "completed",
  "location": {
    "latitude": 37.7749,
    "longitude": -122.4194,
    "name": "Golden Gate Park"
  },
  "mood": "energized",
  "weather": "sunny",
  "created_at": "2024-11-20T06:30:00Z",
  "updated_at": "2024-11-20T06:30:00Z"
}
```

**Error Response - Not Found (404 Not Found):**
```json
{
  "error": "checkin not found"
}
```

---

### Update Check-in

**Request:**
```http
PUT /api/v1/checkins/7c9e6679-7425-40de-944b-e07fc1f90ae7
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

{
  "title": "Morning workout completed - 5K run!",
  "description": "30-minute run in the park. Beat my personal record!",
  "mood": "accomplished"
}
```

**Success Response (200 OK):**
```json
{
  "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "title": "Morning workout completed - 5K run!",
  "description": "30-minute run in the park. Beat my personal record!",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "category_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "category": {
    "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
    "name": "Fitness",
    "color": "#FF6B6B",
    "icon": "dumbbell"
  },
  "tags": ["fitness", "morning", "running"],
  "priority": "high",
  "status": "completed",
  "mood": "accomplished",
  "created_at": "2024-11-20T06:30:00Z",
  "updated_at": "2024-11-20T06:35:00Z"
}
```

---

### Delete Check-in

**Request:**
```http
DELETE /api/v1/checkins/7c9e6679-7425-40de-944b-e07fc1f90ae7
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Success Response (204 No Content):**
```
(empty body)
```

**Error Response - Not Found (404 Not Found):**
```json
{
  "error": "checkin not found"
}
```

---

## Categories

### Create Category

**Request:**
```http
POST /api/v1/categories
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

{
  "name": "Work",
  "color": "#4A90E2",
  "icon": "briefcase",
  "description": "Work-related tasks and achievements"
}
```

**Success Response (201 Created):**
```json
{
  "id": "9ea1b891-a647-62f0-b166-g29fe3g02cg9",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Work",
  "color": "#4A90E2",
  "icon": "briefcase",
  "description": "Work-related tasks and achievements",
  "checkin_count": 0,
  "created_at": "2024-11-20T10:00:00Z",
  "updated_at": "2024-11-20T10:00:00Z"
}
```

**Error Response - Duplicate Name (409 Conflict):**
```json
{
  "error": "category with this name already exists"
}
```

---

### List Categories

**Request:**
```http
GET /api/v1/categories
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Success Response (200 OK):**
```json
{
  "categories": [
    {
      "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Fitness",
      "color": "#FF6B6B",
      "icon": "dumbbell",
      "description": "Health and fitness activities",
      "checkin_count": 42,
      "created_at": "2024-11-01T10:00:00Z",
      "updated_at": "2024-11-01T10:00:00Z"
    },
    {
      "id": "9ea1b891-a647-62f0-b166-g29fe3g02cg9",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Work",
      "color": "#4A90E2",
      "icon": "briefcase",
      "description": "Work-related tasks and achievements",
      "checkin_count": 15,
      "created_at": "2024-11-20T10:00:00Z",
      "updated_at": "2024-11-20T10:00:00Z"
    }
  ]
}
```

---

## Timeline

### Get Daily Timeline

**Request:**
```http
GET /api/v1/timeline/daily?date=2024-11-20
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Success Response (200 OK):**
```json
{
  "date": "2024-11-20",
  "checkins": [
    {
      "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "title": "Morning workout completed",
      "category": {
        "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
        "name": "Fitness",
        "color": "#FF6B6B",
        "icon": "dumbbell"
      },
      "tags": ["fitness", "morning"],
      "created_at": "2024-11-20T06:30:00Z"
    },
    {
      "id": "8d0f7780-8536-51ef-a055-f18ed2f91bf8",
      "title": "Project milestone achieved",
      "category": {
        "id": "9ea1b891-a647-62f0-b166-g29fe3g02cg9",
        "name": "Work",
        "color": "#4A90E2",
        "icon": "briefcase"
      },
      "tags": ["work", "milestone"],
      "created_at": "2024-11-20T14:00:00Z"
    }
  ],
  "summary": {
    "total_checkins": 2,
    "categories": {
      "Fitness": 1,
      "Work": 1
    }
  }
}
```

---

## Error Handling

### Common Error Responses

**400 Bad Request - Validation Error:**
```json
{
  "error": "validation failed",
  "details": {
    "email": "must be a valid email address",
    "password": "must be at least 12 characters long"
  }
}
```

**401 Unauthorized - Missing Token:**
```json
{
  "error": "missing or invalid authorization header"
}
```

**401 Unauthorized - Expired Token:**
```json
{
  "error": "token has expired"
}
```

**403 Forbidden - Insufficient Permissions:**
```json
{
  "error": "insufficient permissions to access this resource"
}
```

**404 Not Found:**
```json
{
  "error": "resource not found"
}
```

**409 Conflict:**
```json
{
  "error": "resource already exists"
}
```

**429 Too Many Requests - Rate Limit:**
```json
{
  "error": "rate limit exceeded",
  "retry_after": 60
}
```

**500 Internal Server Error:**
```json
{
  "error": "internal server error",
  "request_id": "req_abc123xyz789"
}
```

---

## cURL Examples

### Register and Login Flow

```bash
# Register new user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "demo@example.com",
    "password": "SecureP@ssw0rd123!",
    "display_name": "Demo User"
  }'

# Login (save the access token)
TOKEN=$(curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "demo@example.com",
    "password": "SecureP@ssw0rd123!"
  }' | jq -r '.access_token')

# Get profile
curl http://localhost:8080/api/v1/users/me \
  -H "Authorization: Bearer $TOKEN"

# Create category
CATEGORY_ID=$(curl -X POST http://localhost:8080/api/v1/categories \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Personal",
    "color": "#95E1D3",
    "icon": "star"
  }' | jq -r '.id')

# Create check-in
curl -X POST http://localhost:8080/api/v1/checkins \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"title\": \"First check-in\",
    \"description\": \"Testing the API\",
    \"category_id\": \"$CATEGORY_ID\",
    \"tags\": [\"test\", \"demo\"]
  }"

# List check-ins
curl "http://localhost:8080/api/v1/checkins?page=1&limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

---

## HTTP Status Code Summary

| Code | Meaning | When Used |
|------|---------|-----------|
| 200 | OK | Successful GET, PUT requests |
| 201 | Created | Successful POST creating a resource |
| 204 | No Content | Successful DELETE |
| 400 | Bad Request | Invalid input, validation errors |
| 401 | Unauthorized | Missing or invalid authentication |
| 403 | Forbidden | Authenticated but not authorized |
| 404 | Not Found | Resource doesn't exist |
| 409 | Conflict | Resource already exists, concurrent modification |
| 422 | Unprocessable Entity | Valid syntax but semantic errors |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server-side error |
| 503 | Service Unavailable | Temporary server issue |

---

## Rate Limits

| Endpoint | Limit | Window |
|----------|-------|--------|
| `/auth/register` | 3 requests | 1 hour |
| `/auth/login` | 5 requests | 15 minutes |
| `/auth/refresh` | 10 requests | 1 hour |
| All other endpoints | 1000 requests | 1 hour |

**Rate Limit Headers:**
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1700488800
```

---

## Notes

- All timestamps are in ISO 8601 format (UTC)
- UUIDs are in standard format (8-4-4-4-12)
- All request/response bodies use JSON
- Authentication uses Bearer tokens in the Authorization header
- Pagination starts at page 1 (not 0)
- Soft deletes are used (deleted_at field, not exposed in API)

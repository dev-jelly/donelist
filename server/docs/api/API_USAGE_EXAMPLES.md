# API Usage Examples

Complete examples for using the DoneList API with curl, demonstrating all major features and workflows.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Authentication](#authentication)
3. [User Management](#user-management)
4. [Check-ins](#check-ins)
5. [Categories](#categories)
6. [Tags](#tags)
7. [Timeline Views](#timeline-views)
8. [Analytics](#analytics)
9. [Premium Features](#premium-features)
10. [Webhooks](#webhooks)
11. [Error Handling](#error-handling)

## Getting Started

### Base URL

```bash
# Development
export API_URL="http://localhost:8080/api/v1"

# Production
export API_URL="https://api.donelist.io/api/v1"
```

### Response Format

All responses follow this structure:

```json
{
  "data": { /* response payload */ },
  "meta": {
    "total": 100,
    "count": 20
  }
}
```

### Common Headers

```bash
# Content-Type for JSON requests
-H "Content-Type: application/json"

# Authentication
-H "Authorization: Bearer ${ACCESS_TOKEN}"

# Request tracing
-H "X-Request-ID: $(uuidgen)"
```

## Authentication

### 1. Register New User

```bash
curl -X POST "${API_URL}/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePass123!",
    "display_name": "John Doe"
  }'
```

**Response (201 Created):**
```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "email": "user@example.com",
    "display_name": "John Doe",
    "premium_tier": "free",
    "created_at": "2025-11-24T10:00:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2025-11-24T11:00:00Z"
}
```

### 2. Login

```bash
curl -X POST "${API_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePass123!"
  }'
```

**Store tokens for subsequent requests:**
```bash
# Extract and save tokens
ACCESS_TOKEN=$(curl -s -X POST "${API_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"SecurePass123!"}' \
  | jq -r '.access_token')

REFRESH_TOKEN=$(curl -s -X POST "${API_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"SecurePass123!"}' \
  | jq -r '.refresh_token')

echo "Access Token: ${ACCESS_TOKEN}"
```

### 3. Refresh Access Token

```bash
curl -X POST "${API_URL}/auth/refresh" \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "'${REFRESH_TOKEN}'"
  }'
```

### 4. Logout

```bash
# Logout from current device
curl -X POST "${API_URL}/auth/logout" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Logout from all devices
curl -X POST "${API_URL}/auth/logout-all" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## User Management

### 1. Get Current User Profile

```bash
curl -X GET "${API_URL}/users/me" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "email": "user@example.com",
  "display_name": "John Doe",
  "premium_tier": "free",
  "created_at": "2025-11-24T10:00:00Z",
  "updated_at": "2025-11-24T10:00:00Z"
}
```

### 2. Update User Profile

```bash
curl -X PATCH "${API_URL}/users/me" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "John Smith",
    "avatar_url": "https://example.com/avatar.jpg"
  }'
```

### 3. Delete User Account

```bash
curl -X DELETE "${API_URL}/users/me" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "password": "SecurePass123!",
    "confirmation": "DELETE"
  }'
```

## Check-ins

### 1. Create Check-in

```bash
curl -X POST "${API_URL}/checkins" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Completed morning workout",
    "description": "30 minutes of cardio and strength training",
    "category_id": "cat-uuid-here",
    "tags": ["fitness", "morning", "health"],
    "checkin_time": "2025-11-24T07:00:00Z",
    "duration_minutes": 30,
    "visibility": "private"
  }'
```

**Response (201 Created):**
```json
{
  "id": "checkin-uuid",
  "user_id": "user-uuid",
  "title": "Completed morning workout",
  "description": "30 minutes of cardio and strength training",
  "category_id": "cat-uuid-here",
  "tags": ["fitness", "morning", "health"],
  "checkin_time": "2025-11-24T07:00:00Z",
  "duration_minutes": 30,
  "visibility": "private",
  "created_at": "2025-11-24T07:05:00Z",
  "updated_at": "2025-11-24T07:05:00Z",
  "version": 0
}
```

### 2. List Check-ins

```bash
# List with pagination
curl -X GET "${API_URL}/checkins?limit=20&offset=0" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Filter by category
curl -X GET "${API_URL}/checkins?category_id=${CATEGORY_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Filter by tags
curl -X GET "${API_URL}/checkins?tags=fitness,health" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Filter by date range
curl -X GET "${API_URL}/checkins?start_date=2025-11-01&end_date=2025-11-30" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Combined filters with sorting
curl -X GET "${API_URL}/checkins?category_id=${CATEGORY_ID}&tags=fitness&start_date=2025-11-01&sort_by=checkin_time&sort_order=desc&limit=50" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 3. Get Single Check-in

```bash
curl -X GET "${API_URL}/checkins/${CHECKIN_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 4. Update Check-in

```bash
curl -X PATCH "${API_URL}/checkins/${CHECKIN_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Completed extended morning workout",
    "duration_minutes": 45,
    "version": 0
  }'
```

**Note:** Include `version` for optimistic locking (prevents concurrent edit conflicts)

### 5. Delete Check-in

```bash
curl -X DELETE "${API_URL}/checkins/${CHECKIN_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 6. Get Edit History (Premium)

```bash
curl -X GET "${API_URL}/checkins/${CHECKIN_ID}/history" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

**Response:**
```json
{
  "history": [
    {
      "id": "history-uuid",
      "checkin_id": "checkin-uuid",
      "version": 1,
      "previous_title": "Completed morning workout",
      "previous_description": "30 minutes",
      "edit_reason": "Updated duration",
      "edited_at": "2025-11-24T08:00:00Z",
      "edited_by": "user-uuid"
    }
  ]
}
```

## Categories

### 1. Create Category

```bash
curl -X POST "${API_URL}/categories" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Fitness",
    "color": "#FF5733",
    "icon": "💪",
    "display_order": 1
  }'
```

**Response:**
```json
{
  "id": "cat-uuid",
  "user_id": "user-uuid",
  "name": "Fitness",
  "color": "#FF5733",
  "icon": "💪",
  "display_order": 1,
  "created_at": "2025-11-24T10:00:00Z"
}
```

### 2. List Categories

```bash
curl -X GET "${API_URL}/categories" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 3. Get Category by ID

```bash
curl -X GET "${API_URL}/categories/${CATEGORY_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 4. Update Category

```bash
curl -X PATCH "${API_URL}/categories/${CATEGORY_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Health & Fitness",
    "color": "#00FF00",
    "display_order": 2
  }'
```

### 5. Delete Category

```bash
curl -X DELETE "${API_URL}/categories/${CATEGORY_ID}" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 6. Merge Categories

```bash
curl -X POST "${API_URL}/categories/${SOURCE_ID}/merge" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "target_category_id": "'${TARGET_ID}'",
    "delete_source": true
  }'
```

### 7. Get Recommended Colors

```bash
curl -X GET "${API_URL}/categories/colors/recommend?count=10" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

**Response:**
```json
{
  "colors": [
    "#FF5733",
    "#33FF57",
    "#3357FF",
    "#FF33F5"
  ]
}
```

## Tags

### 1. Create Tag

```bash
curl -X POST "${API_URL}/tags" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "productivity"
  }'
```

### 2. List All Tags

```bash
curl -X GET "${API_URL}/tags" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 3. Tag Autocomplete

```bash
curl -X GET "${API_URL}/tags/autocomplete?q=prod&limit=10" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

**Response:**
```json
{
  "suggestions": [
    {
      "id": "tag-uuid",
      "name": "productivity",
      "usage_count": 42
    },
    {
      "id": "tag-uuid-2",
      "name": "product",
      "usage_count": 15
    }
  ]
}
```

### 4. Get Popular Tags

```bash
curl -X GET "${API_URL}/tags/popular?limit=20" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## Timeline Views

### 1. Daily Timeline (Simple)

```bash
curl -X GET "${API_URL}/timeline/daily?date=2025-11-24" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

**Response:**
```json
{
  "date": "2025-11-24",
  "checkins": [
    {
      "id": "checkin-uuid",
      "title": "Morning workout",
      "checkin_time": "2025-11-24T07:00:00Z",
      "duration_minutes": 30
    }
  ],
  "total": 1
}
```

### 2. Enhanced Daily Timeline

```bash
# Basic enhanced timeline
curl -X GET "${API_URL}/timeline/daily/enhanced?date=2025-11-24" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# With custom time blocks
curl -X GET "${API_URL}/timeline/daily/enhanced?date=2025-11-24&block=30&timezone=America/New_York" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# With caching (ETag support)
curl -X GET "${API_URL}/timeline/daily/enhanced?date=2025-11-24" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "If-None-Match: \"etag-value\""

# With pagination
curl -X GET "${API_URL}/timeline/daily/enhanced?date=2025-11-24&limit=24&cursor=eyJwYWdlIjoxfQ==" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

**Response:**
```json
{
  "date": "2025-11-24",
  "timezone": "UTC",
  "block_granularity": 30,
  "blocks": [
    {
      "start_time": "2025-11-24T07:00:00Z",
      "end_time": "2025-11-24T07:30:00Z",
      "duration_mins": 30,
      "checkins": [
        {
          "id": "checkin-uuid",
          "title": "Morning workout",
          "category_name": "Fitness",
          "category_color": "#FF5733"
        }
      ],
      "is_empty": false,
      "is_gap": false
    }
  ],
  "gaps": [
    {
      "start_time": "2025-11-24T08:00:00Z",
      "end_time": "2025-11-24T12:00:00Z",
      "duration_mins": 240
    }
  ],
  "summary": {
    "date": "2025-11-24",
    "total_checkins": 5,
    "total_minutes": 150,
    "first_checkin_time": "2025-11-24T07:00:00Z",
    "last_checkin_time": "2025-11-24T18:00:00Z",
    "active_hours": 11,
    "gap_count": 2,
    "total_gap_minutes": 90,
    "completion_percent": 10.42,
    "average_gap_minutes": 45.0
  },
  "category_legend": [
    {
      "category_id": "cat-uuid",
      "category_name": "Fitness",
      "color": "#FF5733",
      "count": 2
    }
  ],
  "previous_day": "2025-11-23",
  "next_day": "2025-11-25",
  "generated_at": "2025-11-24T10:30:00Z"
}
```

### 3. Weekly Timeline

```bash
curl -X GET "${API_URL}/timeline/weekly?date=2025-11-24" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 4. Monthly Timeline

```bash
curl -X GET "${API_URL}/timeline/monthly?date=2025-11-24" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## Analytics

### 1. User Activity Summary

```bash
curl -X GET "${API_URL}/analytics/activity?start_date=2025-11-01&end_date=2025-11-30" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 2. Category Distribution

```bash
curl -X GET "${API_URL}/analytics/categories?period=month" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 3. Tag Statistics

```bash
curl -X GET "${API_URL}/analytics/tags?limit=50" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 4. Productivity Trends

```bash
curl -X GET "${API_URL}/analytics/trends?metric=checkins&period=week&weeks=12" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## Premium Features

### 1. Export Data

```bash
# Export as CSV
curl -X POST "${API_URL}/premium/export" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "format": "csv",
    "start_date": "2025-01-01",
    "end_date": "2025-12-31",
    "include_deleted": false
  }' \
  --output checkins_export.csv

# Export as JSON
curl -X POST "${API_URL}/premium/export" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "format": "json",
    "start_date": "2025-01-01",
    "end_date": "2025-12-31"
  }' \
  --output checkins_export.json

# Export as PDF
curl -X POST "${API_URL}/premium/export" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "format": "pdf",
    "start_date": "2025-01-01",
    "end_date": "2025-12-31"
  }' \
  --output checkins_export.pdf
```

### 2. Advanced Analytics

```bash
curl -X GET "${API_URL}/premium/analytics/detailed?start_date=2025-11-01&end_date=2025-11-30" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## Webhooks

### 1. Create Webhook

```bash
curl -X POST "${API_URL}/webhooks" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://your-app.com/webhook",
    "events": ["checkin.created", "checkin.updated", "checkin.deleted"],
    "secret": "your-webhook-secret",
    "active": true
  }'
```

### 2. List Webhooks

```bash
curl -X GET "${API_URL}/webhooks" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 3. Test Webhook

```bash
curl -X POST "${API_URL}/webhooks/${WEBHOOK_ID}/test" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

### 4. Webhook Event Verification

When your webhook endpoint receives events, verify the signature:

```bash
# Your webhook should verify the signature
# X-Webhook-Signature: sha256=hash
# Compare with: HMAC-SHA256(payload, webhook_secret)
```

## Error Handling

### Common Error Responses

#### 400 Bad Request

```json
{
  "error": "validation_error",
  "message": "Invalid input data",
  "details": {
    "field": "email",
    "reason": "Invalid email format"
  }
}
```

#### 401 Unauthorized

```json
{
  "error": "unauthorized",
  "message": "Invalid or expired authentication token"
}
```

#### 403 Forbidden

```json
{
  "error": "forbidden",
  "message": "Premium feature requires subscription"
}
```

#### 404 Not Found

```json
{
  "error": "not_found",
  "message": "Checkin not found"
}
```

#### 409 Conflict

```json
{
  "error": "conflict",
  "message": "Concurrent edit detected. Please refresh and try again.",
  "details": {
    "current_version": 2,
    "provided_version": 1
  }
}
```

#### 422 Unprocessable Entity

```json
{
  "error": "validation_error",
  "message": "Password does not meet security requirements",
  "details": {
    "requirements": [
      "At least 12 characters",
      "At least 3 character types (uppercase, lowercase, digits, special)"
    ]
  }
}
```

#### 429 Too Many Requests

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests. Please try again later.",
  "retry_after": 60
}
```

### Handling Rate Limits

```bash
# Check rate limit headers in response
curl -i -X GET "${API_URL}/checkins" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Response headers:
# X-RateLimit-Limit: 1000
# X-RateLimit-Remaining: 999
# X-RateLimit-Reset: 1700000000
```

## Complete Workflow Examples

### Example 1: First-Time User Onboarding

```bash
#!/bin/bash
set -e

API_URL="http://localhost:8080/api/v1"

# 1. Register
echo "Registering user..."
AUTH_RESPONSE=$(curl -s -X POST "${API_URL}/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "newuser@example.com",
    "password": "SecurePass123!",
    "display_name": "New User"
  }')

ACCESS_TOKEN=$(echo $AUTH_RESPONSE | jq -r '.access_token')
echo "Registered successfully. Token: ${ACCESS_TOKEN:0:20}..."

# 2. Create first category
echo "Creating category..."
CATEGORY_RESPONSE=$(curl -s -X POST "${API_URL}/categories" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Work",
    "color": "#FF5733",
    "icon": "💼"
  }')

CATEGORY_ID=$(echo $CATEGORY_RESPONSE | jq -r '.id')
echo "Category created: ${CATEGORY_ID}"

# 3. Create first checkin
echo "Creating first checkin..."
CHECKIN_RESPONSE=$(curl -s -X POST "${API_URL}/checkins" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Set up development environment",
    "description": "Configured IDE and dependencies",
    "category_id": "'${CATEGORY_ID}'",
    "tags": ["development", "setup"],
    "duration_minutes": 60
  }')

echo "First checkin created!"

# 4. View timeline
echo "Viewing today timeline..."
curl -s -X GET "${API_URL}/timeline/daily/enhanced?date=$(date +%Y-%m-%d)" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" | jq '.summary'
```

### Example 2: Daily Activity Logging

```bash
#!/bin/bash

API_URL="http://localhost:8080/api/v1"
ACCESS_TOKEN="your-token-here"

# Log multiple activities
activities=(
  "Completed morning standup:Work:15"
  "Code review session:Development:45"
  "Lunch break:Personal:30"
  "Feature implementation:Development:120"
  "Team meeting:Work:30"
)

for activity in "${activities[@]}"; do
  IFS=':' read -r title category duration <<< "$activity"

  curl -s -X POST "${API_URL}/checkins" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "title": "'${title}'",
      "category_name": "'${category}'",
      "duration_minutes": '${duration}'
    }'

  echo "Logged: ${title}"
done

echo "Daily activities logged successfully!"
```

## Tips & Best Practices

### 1. Token Management

```bash
# Store tokens securely
TOKEN_FILE="${HOME}/.donelist_token"

# Save token after login
echo "${ACCESS_TOKEN}" > "${TOKEN_FILE}"
chmod 600 "${TOKEN_FILE}"

# Load token for requests
ACCESS_TOKEN=$(cat "${TOKEN_FILE}")
```

### 2. Error Recovery

```bash
# Retry with exponential backoff
retry_request() {
  local max_attempts=3
  local attempt=1
  local delay=1

  while [ $attempt -le $max_attempts ]; do
    if curl -s -X GET "${API_URL}/checkins" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}"; then
      return 0
    fi

    echo "Attempt $attempt failed. Retrying in ${delay}s..."
    sleep $delay
    delay=$((delay * 2))
    attempt=$((attempt + 1))
  done

  return 1
}
```

### 3. Pagination Helper

```bash
# Fetch all pages
fetch_all_checkins() {
  local offset=0
  local limit=100
  local has_more=true

  while [ "$has_more" = true ]; do
    response=$(curl -s -X GET \
      "${API_URL}/checkins?limit=${limit}&offset=${offset}" \
      -H "Authorization: Bearer ${ACCESS_TOKEN}")

    echo "$response" | jq '.data[]'

    count=$(echo "$response" | jq '.data | length')
    if [ "$count" -lt "$limit" ]; then
      has_more=false
    fi

    offset=$((offset + limit))
  done
}
```

## Resources

- [API Reference](./README.md)
- [OpenAPI Specification](./openapi.yaml)
- [Postman Collection](./DoneList_API.postman_collection.json)
- [Testing Guide](../testing/API_TESTING_GUIDE.md)

---

**Last Updated**: 2025-11-24
**Version**: 1.0.0
**Maintained By**: DoneList Development Team

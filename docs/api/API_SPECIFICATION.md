# Donelist - API Specification

**Version**: 1.0.0
**Date**: 2025-11-10
**Protocol**: REST + WebSocket
**Base URL**: `https://api.donelist.com/v1`

---

## 📋 Table of Contents

1. [Authentication](#authentication)
2. [Users API](#users-api)
3. [Check-ins API](#check-ins-api)
4. [Timeline API](#timeline-api)
5. [Categories API](#categories-api)
6. [Tags API](#tags-api)
7. [Subscriptions API](#subscriptions-api)
8. [WebSocket API](#websocket-api)
9. [Error Handling](#error-handling)
10. [Rate Limiting](#rate-limiting)

---

## 🔐 Authentication

### JWT-based Authentication

**Authentication Header**:
```http
Authorization: Bearer <access_token>
```

### POST /auth/register
사용자 회원가입

**Request**:
```json
{
  "email": "user@example.com",
  "password": "securePassword123!",
  "username": "johndoe",
  "full_name": "John Doe"
}
```

**Response** (201 Created):
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "username": "johndoe",
    "full_name": "John Doe",
    "tier": "free",
    "created_at": "2025-11-10T09:00:00Z"
  },
  "tokens": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 900
  }
}
```

**Validation Rules**:
- Email: Valid email format, unique
- Password: Min 8 chars, at least 1 uppercase, 1 lowercase, 1 number
- Username: 3-50 chars, alphanumeric + underscore
- Full name: 1-100 chars

**Errors**:
- `400`: Validation failed
- `409`: Email already exists

---

### POST /auth/login
사용자 로그인

**Request**:
```json
{
  "email": "user@example.com",
  "password": "securePassword123!"
}
```

**Response** (200 OK):
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "username": "johndoe",
    "tier": "premium"
  },
  "tokens": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 900
  }
}
```

**Errors**:
- `400`: Missing fields
- `401`: Invalid credentials
- `429`: Rate limit exceeded (5 attempts per 15 minutes)

---

### POST /auth/refresh
Access token 갱신

**Request**:
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Response** (200 OK):
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 900
}
```

**Errors**:
- `401`: Invalid or expired refresh token
- `404`: Token not found

---

### POST /auth/logout
로그아웃 (refresh token 무효화)

**Request Headers**:
```http
Authorization: Bearer <access_token>
```

**Request Body**:
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Response** (204 No Content)

---

## 👤 Users API

### GET /users/me
현재 사용자 정보 조회

**Request Headers**:
```http
Authorization: Bearer <access_token>
```

**Response** (200 OK):
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "username": "johndoe",
  "full_name": "John Doe",
  "avatar_url": "https://cdn.donelist.com/avatars/johndoe.jpg",
  "tier": "premium",
  "email_verified": true,
  "created_at": "2025-01-01T00:00:00Z",
  "last_login_at": "2025-11-10T09:00:00Z"
}
```

---

### PATCH /users/me
현재 사용자 정보 수정

**Request**:
```json
{
  "username": "newusername",
  "full_name": "New Full Name",
  "avatar_url": "https://cdn.donelist.com/avatars/new.jpg"
}
```

**Response** (200 OK):
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "username": "newusername",
  "full_name": "New Full Name",
  "avatar_url": "https://cdn.donelist.com/avatars/new.jpg",
  "tier": "premium",
  "updated_at": "2025-11-10T09:15:00Z"
}
```

**Errors**:
- `400`: Validation failed
- `409`: Username already taken

---

### DELETE /users/me
계정 삭제 (Soft delete)

**Request Headers**:
```http
Authorization: Bearer <access_token>
```

**Request Body**:
```json
{
  "password": "securePassword123!",
  "confirmation": "DELETE"
}
```

**Response** (204 No Content)

**Note**:
- 30일 유예 기간 후 영구 삭제
- 유예 기간 내 복구 가능

---

## ✅ Check-ins API

### POST /checkins
체크인 생성

**Request**:
```json
{
  "content": "React 컴포넌트 리팩토링 작업",
  "checkin_time": "2025-11-10T09:15:00Z",
  "interval_minutes": 15,
  "category_id": "c1234567-e29b-41d4-a716-446655440001",
  "tags": ["coding", "react", "frontend"],
  "metadata": {
    "mood": "productive",
    "energy_level": 8
  }
}
```

**Response** (201 Created):
```json
{
  "id": "abcdef12-3456-7890-abcd-ef1234567890",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "content": "React 컴포넌트 리팩토링 작업",
  "checkin_time": "2025-11-10T09:15:00Z",
  "interval_minutes": 15,
  "category": {
    "id": "c1234567-e29b-41d4-a716-446655440001",
    "name": "Work",
    "color_hex": "#3B82F6"
  },
  "tags": [
    {
      "id": "t1234567-e29b-41d4-a716-446655440001",
      "name": "coding",
      "color_hex": "#64748B"
    },
    {
      "id": "t1234567-e29b-41d4-a716-446655440002",
      "name": "react",
      "color_hex": "#64748B"
    }
  ],
  "metadata": {
    "mood": "productive",
    "energy_level": 8
  },
  "is_edited": false,
  "created_at": "2025-11-10T09:15:30Z"
}
```

**Validation Rules**:
- content: 1-2000 chars
- checkin_time: Cannot be in future
- interval_minutes: 15, 30, 45, 60, or 120
- tags: Max 10 tags per checkin

**Errors**:
- `400`: Validation failed
- `401`: Unauthorized
- `422`: Checkin time conflicts with existing checkin

---

### GET /checkins
체크인 목록 조회 (Timeline)

**Query Parameters**:
```
?start_date=2025-11-01T00:00:00Z
&end_date=2025-11-30T23:59:59Z
&category_id=c1234567-e29b-41d4-a716-446655440001
&tags=coding,react
&limit=50
&offset=0
&sort_by=checkin_time
&sort_order=desc
```

**Response** (200 OK):
```json
{
  "checkins": [
    {
      "id": "abcdef12-3456-7890-abcd-ef1234567890",
      "content": "React 컴포넌트 리팩토링 작업",
      "checkin_time": "2025-11-10T09:15:00Z",
      "interval_minutes": 15,
      "category": {
        "id": "c1234567-e29b-41d4-a716-446655440001",
        "name": "Work",
        "color_hex": "#3B82F6"
      },
      "tags": ["coding", "react"],
      "is_edited": false,
      "created_at": "2025-11-10T09:15:30Z"
    }
  ],
  "pagination": {
    "total": 150,
    "limit": 50,
    "offset": 0,
    "has_more": true
  }
}
```

**Filtering**:
- start_date, end_date: ISO 8601 format
- category_id: UUID
- tags: Comma-separated tag names
- limit: 1-100 (default: 50)
- offset: 0+ (default: 0)
- sort_by: checkin_time, created_at
- sort_order: asc, desc

---

### GET /checkins/:id
특정 체크인 조회

**Response** (200 OK):
```json
{
  "id": "abcdef12-3456-7890-abcd-ef1234567890",
  "content": "React 컴포넌트 리팩토링 작업",
  "checkin_time": "2025-11-10T09:15:00Z",
  "interval_minutes": 15,
  "category": {
    "id": "c1234567-e29b-41d4-a716-446655440001",
    "name": "Work",
    "color_hex": "#3B82F6"
  },
  "tags": ["coding", "react"],
  "metadata": {
    "mood": "productive"
  },
  "is_edited": false,
  "edit_count": 0,
  "created_at": "2025-11-10T09:15:30Z",
  "updated_at": "2025-11-10T09:15:30Z"
}
```

**Errors**:
- `404`: Checkin not found
- `403`: Not your checkin

---

### PATCH /checkins/:id
체크인 수정 (Premium feature for 2h+ old checkins)

**Request**:
```json
{
  "content": "Updated content",
  "category_id": "c1234567-e29b-41d4-a716-446655440002",
  "tags": ["updated", "tags"],
  "edit_reason": "Forgot to mention important detail"
}
```

**Response** (200 OK):
```json
{
  "id": "abcdef12-3456-7890-abcd-ef1234567890",
  "content": "Updated content",
  "checkin_time": "2025-11-10T09:15:00Z",
  "interval_minutes": 15,
  "category": {
    "id": "c1234567-e29b-41d4-a716-446655440002",
    "name": "Study",
    "color_hex": "#8B5CF6"
  },
  "tags": ["updated", "tags"],
  "is_edited": true,
  "edited_at": "2025-11-10T11:30:00Z",
  "edit_count": 1,
  "updated_at": "2025-11-10T11:30:00Z"
}
```

**Business Rules**:
- **< 2 hours old**: Free users can edit
- **>= 2 hours old**: Premium users only
- Edit history is preserved (see GET /checkins/:id/history)

**Errors**:
- `403`: Premium required (if > 2 hours old and user is free tier)
- `404`: Checkin not found
- `422`: Cannot edit checkin time to future

---

### DELETE /checkins/:id
체크인 삭제 (Soft delete)

**Response** (204 No Content)

**Errors**:
- `404`: Checkin not found
- `403`: Not your checkin

---

### GET /checkins/:id/history
체크인 수정 이력 조회 (Premium feature)

**Response** (200 OK):
```json
{
  "checkin_id": "abcdef12-3456-7890-abcd-ef1234567890",
  "history": [
    {
      "id": "hist_001",
      "previous_content": "Original content",
      "new_content": "Updated content",
      "previous_category": "Work",
      "new_category": "Study",
      "edit_reason": "Forgot to mention important detail",
      "edited_at": "2025-11-10T11:30:00Z"
    }
  ]
}
```

**Errors**:
- `403`: Premium required
- `404`: Checkin not found

---

## 📅 Timeline API

### GET /timeline/daily
일간 타임라인 조회

**Query Parameters**:
```
?date=2025-11-10
&timezone=Asia/Seoul
```

**Response** (200 OK):
```json
{
  "date": "2025-11-10",
  "checkins": [
    {
      "id": "abc",
      "content": "Morning standup",
      "checkin_time": "2025-11-10T09:00:00+09:00",
      "interval_minutes": 15,
      "category": { "name": "Work", "color_hex": "#3B82F6" },
      "tags": ["meeting"]
    }
  ],
  "summary": {
    "total_checkins": 12,
    "total_minutes": 480,
    "total_hours": 8,
    "completion_rate": 75.5,
    "categories_breakdown": [
      { "name": "Work", "count": 8, "minutes": 360 },
      { "name": "Rest", "count": 4, "minutes": 120 }
    ]
  }
}
```

**Notes**:
- timezone: IANA timezone (e.g., Asia/Seoul, America/New_York)
- completion_rate: Percentage of day covered by checkins

---

### GET /timeline/weekly
주간 타임라인 요약

**Query Parameters**:
```
?week_start=2025-11-04
&timezone=Asia/Seoul
```

**Response** (200 OK):
```json
{
  "week_start": "2025-11-04",
  "week_end": "2025-11-10",
  "daily_summaries": [
    {
      "date": "2025-11-04",
      "checkin_count": 12,
      "total_minutes": 480,
      "completion_rate": 75.5
    }
  ],
  "weekly_summary": {
    "total_checkins": 84,
    "total_minutes": 3360,
    "total_hours": 56,
    "active_days": 7,
    "avg_checkins_per_day": 12,
    "top_categories": [
      { "name": "Work", "count": 50, "minutes": 2400 },
      { "name": "Exercise", "count": 14, "minutes": 420 }
    ],
    "most_productive_hour": 14
  }
}
```

---

### GET /timeline/monthly
월간 타임라인 요약

**Query Parameters**:
```
?year=2025
&month=11
&timezone=Asia/Seoul
```

**Response** (200 OK):
```json
{
  "year": 2025,
  "month": 11,
  "daily_completion_rates": [
    { "date": "2025-11-01", "completion_rate": 80.5 },
    { "date": "2025-11-02", "completion_rate": 65.0 }
  ],
  "monthly_summary": {
    "total_checkins": 360,
    "total_minutes": 14400,
    "total_hours": 240,
    "active_days": 30,
    "avg_checkins_per_day": 12,
    "avg_completion_rate": 72.5,
    "top_categories": [
      { "name": "Work", "count": 200, "minutes": 9600 }
    ]
  }
}
```

---

## 🏷️ Categories API

### GET /categories
카테고리 목록 조회 (System + User custom)

**Response** (200 OK):
```json
{
  "system_categories": [
    {
      "id": "c1234567-e29b-41d4-a716-446655440001",
      "name": "Work",
      "description": "업무 관련 활동",
      "color_hex": "#3B82F6",
      "icon": "💼",
      "is_system": true
    }
  ],
  "custom_categories": [
    {
      "id": "custom123-e29b-41d4-a716-446655440001",
      "name": "Side Project",
      "description": "사이드 프로젝트 작업",
      "color_hex": "#F59E0B",
      "icon": "🚀",
      "is_system": false,
      "display_order": 10
    }
  ]
}
```

---

### POST /categories
커스텀 카테고리 생성 (Premium feature)

**Request**:
```json
{
  "name": "Side Project",
  "description": "사이드 프로젝트 작업",
  "color_hex": "#F59E0B",
  "icon": "🚀"
}
```

**Response** (201 Created):
```json
{
  "id": "custom123-e29b-41d4-a716-446655440001",
  "name": "Side Project",
  "description": "사이드 프로젝트 작업",
  "color_hex": "#F59E0B",
  "icon": "🚀",
  "is_system": false,
  "created_at": "2025-11-10T09:00:00Z"
}
```

**Errors**:
- `403`: Premium required
- `400`: Validation failed (name too long, invalid color)

---

### PATCH /categories/:id
카테고리 수정 (Custom only)

**Request**:
```json
{
  "name": "Updated Name",
  "color_hex": "#EF4444"
}
```

**Response** (200 OK)

**Errors**:
- `403`: Cannot modify system categories
- `404`: Category not found

---

### DELETE /categories/:id
카테고리 삭제 (Custom only, soft delete)

**Response** (204 No Content)

**Notes**:
- Existing checkins with this category will have category_id set to NULL
- System categories cannot be deleted

---

## 🏷️ Tags API

### GET /tags
사용자의 태그 목록 조회

**Query Parameters**:
```
?sort_by=usage_count
&sort_order=desc
&limit=100
```

**Response** (200 OK):
```json
{
  "tags": [
    {
      "id": "t1234567-e29b-41d4-a716-446655440001",
      "name": "coding",
      "color_hex": "#64748B",
      "usage_count": 45,
      "created_at": "2025-10-01T00:00:00Z"
    }
  ],
  "pagination": {
    "total": 25,
    "limit": 100,
    "offset": 0
  }
}
```

---

### POST /tags
태그 생성 (자동 생성됨, 명시적 생성도 가능)

**Request**:
```json
{
  "name": "urgent",
  "color_hex": "#EF4444"
}
```

**Response** (201 Created):
```json
{
  "id": "t1234567-e29b-41d4-a716-446655440999",
  "name": "urgent",
  "color_hex": "#EF4444",
  "usage_count": 0,
  "created_at": "2025-11-10T09:00:00Z"
}
```

**Note**: 태그는 체크인 생성 시 자동으로 생성되므로, 이 엔드포인트는 선택적

---

## 💳 Subscriptions API

### GET /subscriptions/me
현재 사용자의 구독 정보 조회

**Response** (200 OK):
```json
{
  "id": "sub_123",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "tier": "premium",
  "status": "active",
  "current_period_start": "2025-11-01T00:00:00Z",
  "current_period_end": "2025-12-01T00:00:00Z",
  "cancel_at": null,
  "trial_end": null
}
```

**Tiers**:
- `free`: 무료 사용자
- `premium`: 유료 구독자

**Statuses**:
- `active`: 정상 구독 중
- `trialing`: 무료 체험 중
- `canceled`: 취소됨 (기간 만료까지 사용 가능)
- `past_due`: 결제 실패

---

### POST /subscriptions/checkout
구독 결제 세션 생성 (Stripe Checkout)

**Request**:
```json
{
  "tier": "premium",
  "billing_cycle": "monthly",
  "success_url": "https://app.donelist.com/success",
  "cancel_url": "https://app.donelist.com/cancel"
}
```

**Response** (200 OK):
```json
{
  "checkout_session_id": "cs_test_abc123",
  "checkout_url": "https://checkout.stripe.com/pay/cs_test_abc123"
}
```

**Billing Cycles**:
- `monthly`: $4.99/month
- `yearly`: $49.99/year (Save 17%)

---

### POST /subscriptions/cancel
구독 취소 (기간 만료 시까지 사용 가능)

**Response** (200 OK):
```json
{
  "id": "sub_123",
  "status": "canceled",
  "cancel_at": "2025-12-01T00:00:00Z",
  "canceled_at": "2025-11-10T09:00:00Z"
}
```

---

### POST /subscriptions/reactivate
취소된 구독 재활성화

**Response** (200 OK):
```json
{
  "id": "sub_123",
  "status": "active",
  "cancel_at": null
}
```

---

## 🔌 WebSocket API

### Connection

**WebSocket URL**: `wss://api.donelist.com/v1/ws`

**Authentication**:
```
wss://api.donelist.com/v1/ws?token=<access_token>
```

### Message Types

#### 1. Client → Server: Subscribe to Updates

```json
{
  "type": "subscribe",
  "payload": {
    "events": ["checkin.created", "checkin.updated", "checkin.deleted"]
  }
}
```

#### 2. Server → Client: Checkin Created

```json
{
  "type": "checkin.created",
  "payload": {
    "id": "abc123",
    "content": "New checkin",
    "checkin_time": "2025-11-10T09:00:00Z",
    "category": { "name": "Work" }
  },
  "timestamp": "2025-11-10T09:00:05Z"
}
```

#### 3. Server → Client: Checkin Updated

```json
{
  "type": "checkin.updated",
  "payload": {
    "id": "abc123",
    "content": "Updated checkin",
    "is_edited": true
  },
  "timestamp": "2025-11-10T09:05:00Z"
}
```

#### 4. Server → Client: Checkin Deleted

```json
{
  "type": "checkin.deleted",
  "payload": {
    "id": "abc123"
  },
  "timestamp": "2025-11-10T09:10:00Z"
}
```

#### 5. Client → Server: Ping (Heartbeat)

```json
{
  "type": "ping"
}
```

#### 6. Server → Client: Pong

```json
{
  "type": "pong",
  "timestamp": "2025-11-10T09:00:00Z"
}
```

### Connection Lifecycle

```
1. Client connects with access token
2. Server validates token
3. Server sends "connected" message
4. Client subscribes to events
5. Server sends real-time updates
6. Client sends ping every 30 seconds
7. Server responds with pong
8. Connection closes on token expiration or client disconnect
```

---

## ❌ Error Handling

### Standard Error Response

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed",
    "details": [
      {
        "field": "email",
        "message": "Invalid email format"
      }
    ]
  },
  "request_id": "req_abc123",
  "timestamp": "2025-11-10T09:00:00Z"
}
```

### Error Codes

| HTTP Status | Error Code | Description |
|------------|------------|-------------|
| 400 | `VALIDATION_ERROR` | Request validation failed |
| 401 | `UNAUTHORIZED` | Missing or invalid authentication |
| 403 | `FORBIDDEN` | Insufficient permissions (e.g., Premium required) |
| 404 | `NOT_FOUND` | Resource not found |
| 409 | `CONFLICT` | Resource conflict (e.g., email already exists) |
| 422 | `UNPROCESSABLE_ENTITY` | Business logic validation failed |
| 429 | `RATE_LIMIT_EXCEEDED` | Too many requests |
| 500 | `INTERNAL_SERVER_ERROR` | Server error |
| 503 | `SERVICE_UNAVAILABLE` | Service temporarily unavailable |

---

## ⏱️ Rate Limiting

### Rate Limit Headers

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1699564800
```

### Limits by Tier

| Tier | Requests per Minute | Requests per Hour |
|------|---------------------|-------------------|
| Free | 60 | 1000 |
| Premium | 120 | 5000 |

### Rate Limit Response (429)

```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Too many requests. Please try again later.",
    "retry_after": 30
  }
}
```

---

## 📊 Pagination

### Query Parameters

```
?limit=50
&offset=0
```

### Response Format

```json
{
  "data": [...],
  "pagination": {
    "total": 150,
    "limit": 50,
    "offset": 0,
    "has_more": true,
    "next_offset": 50
  }
}
```

---

## 🔍 Filtering & Sorting

### Common Query Parameters

```
?sort_by=created_at
&sort_order=desc
&filter[category_id]=abc123
&filter[tags]=coding,react
&search=query
```

---

## 📝 OpenAPI Specification

Full OpenAPI 3.0 spec available at:
```
GET /openapi.json
GET /openapi.yaml
```

Interactive API docs (Swagger UI):
```
GET /api-docs
```

---

## 🧪 Testing & Sandbox

### Sandbox Environment

**Base URL**: `https://api-sandbox.donelist.com/v1`

### Test Credentials

```
Email: test@donelist.com
Password: TestPassword123!
```

### Test Stripe Cards

```
Success: 4242 4242 4242 4242
Decline: 4000 0000 0000 0002
```

---

## 📚 SDK & Client Libraries

### Official SDKs

- **JavaScript/TypeScript**: `npm install @donelist/sdk`
- **Swift (iOS/macOS)**: `Swift Package Manager`
- **Kotlin (Android)**: `implementation 'com.donelist:sdk:1.0.0'`

### Example Usage (TypeScript)

```typescript
import { DonelistClient } from '@donelist/sdk';

const client = new DonelistClient({
  baseURL: 'https://api.donelist.com/v1',
  accessToken: 'your_access_token'
});

// Create checkin
const checkin = await client.checkins.create({
  content: 'Working on API docs',
  checkin_time: new Date(),
  interval_minutes: 15,
  category_id: 'work_category_id',
  tags: ['documentation', 'api']
});

// Get timeline
const timeline = await client.timeline.getDaily({
  date: '2025-11-10',
  timezone: 'Asia/Seoul'
});
```

---

## 🔗 Webhooks (Future)

**Coming soon**: Webhook support for external integrations

**Events**:
- `checkin.created`
- `checkin.updated`
- `checkin.deleted`
- `subscription.updated`
- `subscription.canceled`

---

**Document Status**: Draft v1.0
**Next Review Date**: 2025-11-15
**Owner**: API Team

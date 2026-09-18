# Donelist API Documentation

## Overview

The Donelist API is a comprehensive RESTful API for managing daily check-ins, timelines, categories, tags, and user data. It features JWT-based authentication, real-time WebSocket updates, and premium subscription features.

## Base URL

```
http://localhost:8080/api/v1
```

Production:
```
https://api.donelist.io/api/v1
```

## Authentication

All authenticated endpoints require a JWT Bearer token in the Authorization header:

```
Authorization: Bearer <your_access_token>
```

### Getting Started

1. **Register**: `POST /auth/register`
2. **Login**: `POST /auth/login` - Returns `access_token` and `refresh_token`
3. **Use Token**: Include in Authorization header for all subsequent requests
4. **Refresh**: `POST /auth/refresh` - Get new access token when expired

## Quick Start

### 1. Register a New User

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePass123!",
    "display_name": "John Doe"
  }'
```

Response:
```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "email": "user@example.com",
    "display_name": "John Doe",
    "created_at": "2024-11-24T10:00:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2024-11-24T11:00:00Z"
}
```

### 2. Create a Check-in

```bash
curl -X POST http://localhost:8080/api/v1/checkins \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Completed morning workout",
    "category_id": "cat-uuid",
    "tags": ["fitness", "morning"],
    "visibility": "private"
  }'
```

### 3. List Check-ins

```bash
curl -X GET "http://localhost:8080/api/v1/checkins?limit=20&offset=0" \
  -H "Authorization: Bearer <access_token>"
```

## API Endpoints

### Authentication (`/auth`)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/auth/register` | Register new user | No |
| POST | `/auth/login` | Login user | No |
| POST | `/auth/refresh` | Refresh access token | No |
| POST | `/auth/logout` | Logout from current device | Yes |
| POST | `/auth/logout-all` | Logout from all devices | Yes |

### Users (`/users`)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/users/me` | Get current user profile | Yes |
| PATCH | `/users/me` | Update current user profile | Yes |
| DELETE | `/users/me` | Delete current user account | Yes |

### Check-ins (`/checkins`)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/checkins` | Create new check-in | Yes |
| GET | `/checkins` | List check-ins with filtering | Yes |
| GET | `/checkins/{id}` | Get check-in by ID | Yes |
| PATCH | `/checkins/{id}` | Update check-in | Yes |
| DELETE | `/checkins/{id}` | Delete check-in | Yes |
| GET | `/checkins/{id}/history` | Get edit history (Premium) | Yes |

### Categories (`/categories`)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/categories` | Create category | Yes |
| GET | `/categories` | List categories | Yes |
| GET | `/categories/{id}` | Get category by ID | Yes |
| PATCH | `/categories/{id}` | Update category | Yes |
| DELETE | `/categories/{id}` | Delete category | Yes |
| POST | `/categories/{id}/merge` | Merge categories | Yes |

### Tags (`/tags`)

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/tags` | Create tag | Yes |
| GET | `/tags` | List tags | Yes |
| GET | `/tags/{id}` | Get tag by ID | Yes |
| GET | `/tags/autocomplete` | Autocomplete tags | Yes |
| GET | `/tags/popular` | Get popular tags | Yes |

### Timeline (`/timeline`)

| Method | Endpoint | Description | Auth Required | Cache Support |
|--------|----------|-------------|---------------|---------------|
| GET | `/timeline/daily` | Simple daily timeline | Yes | No |
| GET | `/timeline/daily/enhanced` | Enhanced timeline with analytics | Yes | Yes (ETag) |
| GET | `/timeline/weekly` | Weekly timeline grouped by day | Yes | No |
| GET | `/timeline/monthly` | Monthly timeline grouped by day | Yes | No |

**See**: [TIMELINE_API.md](./TIMELINE_API.md) for complete documentation

### Health & Monitoring

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/health` | Basic health check | No |
| GET | `/health/ready` | Readiness probe | No |
| GET | `/health/live` | Liveness probe | No |
| GET | `/metrics` | Prometheus metrics | No |

## Data Models

### User

```json
{
  "id": "uuid",
  "email": "user@example.com",
  "display_name": "John Doe",
  "premium_tier": "free",
  "created_at": "2024-11-24T10:00:00Z",
  "updated_at": "2024-11-24T10:00:00Z"
}
```

### Check-in

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "message": "Completed morning workout",
  "category_id": "uuid",
  "tags": ["fitness", "morning"],
  "visibility": "private",
  "created_at": "2024-11-24T10:00:00Z",
  "updated_at": "2024-11-24T10:00:00Z",
  "version": 1
}
```

### Category

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "Fitness",
  "color": "#FF5733",
  "icon": "💪",
  "display_order": 1,
  "created_at": "2024-11-24T10:00:00Z"
}
```

### Tag

```json
{
  "id": "uuid",
  "name": "fitness",
  "usage_count": 42,
  "created_at": "2024-11-24T10:00:00Z"
}
```

## Query Parameters

### Pagination

All list endpoints support pagination:

```
GET /checkins?limit=20&offset=0
```

- `limit`: Number of items to return (default: 20, max: 100)
- `offset`: Number of items to skip (default: 0)

### Filtering

Check-ins support advanced filtering:

```
GET /checkins?category_id=uuid&tags=fitness,morning&start_date=2024-11-01&end_date=2024-11-30
```

- `category_id`: Filter by category UUID
- `tags`: Comma-separated tag names
- `start_date`: ISO 8601 date (inclusive)
- `end_date`: ISO 8601 date (inclusive)
- `visibility`: Filter by visibility (private, team, public)

### Sorting

```
GET /checkins?sort_by=created_at&sort_order=desc
```

- `sort_by`: Field to sort by (created_at, updated_at)
- `sort_order`: Sort order (asc, desc)

## Error Handling

The API uses standard HTTP status codes:

- `200 OK`: Request succeeded
- `201 Created`: Resource created successfully
- `204 No Content`: Request succeeded with no response body
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Missing or invalid authentication
- `403 Forbidden`: Not authorized to access resource
- `404 Not Found`: Resource not found
- `409 Conflict`: Resource conflict (e.g., duplicate email)
- `422 Unprocessable Entity`: Validation error
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Server error
- `503 Service Unavailable`: Service temporarily unavailable

### Error Response Format

```json
{
  "error": "Error message description",
  "code": "ERROR_CODE",
  "details": {
    "field": "Additional error details"
  }
}
```

## Rate Limiting

API requests are rate limited:

- **Anonymous**: 100 requests per hour
- **Authenticated**: 1000 requests per hour
- **Premium**: 5000 requests per hour

Rate limit headers:
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1700000000
```

## WebSocket

Real-time updates via WebSocket:

```javascript
const ws = new WebSocket('ws://localhost:8080/ws?token=<access_token>');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

### WebSocket Events

- `checkin.created`: New check-in created
- `checkin.updated`: Check-in updated
- `checkin.deleted`: Check-in deleted
- `category.created`: Category created
- `category.updated`: Category updated
- `sync.required`: Client should sync data

## Premium Features

Premium users have access to:

- **Edit History**: View and restore previous versions of check-ins
- **Advanced Analytics**: Detailed statistics and insights
- **Export**: Export data in CSV, JSON, PDF formats
- **Webhooks**: Configure webhooks for events
- **Priority Support**: Faster response times

## SDK Support

Official SDKs available:

- **JavaScript/TypeScript**: `npm install @donelist/api-client`
- **Python**: `pip install donelist-api`
- **Go**: `go get github.com/dev-jelly/donelist/sdk/go`

## Testing

### Interactive Documentation

Access Swagger UI for interactive testing:

```
http://localhost:8080/swagger/index.html
```

### cURL Examples

See [examples/curl-examples.sh](../examples/curl-examples.sh) for comprehensive cURL examples.

### Postman Collection

Import the Postman collection:

```
docs/api/postman_collection.json
```

## Support

- **Documentation**: https://docs.donelist.io
- **GitHub Issues**: https://github.com/dev-jelly/donelist/issues
- **Email**: support@donelist.io

## Version History

### v1.0 (Current)
- Initial API release
- JWT authentication
- Check-ins, categories, tags CRUD
- WebSocket real-time updates
- Premium features
- Comprehensive monitoring

## License

MIT License - See [LICENSE](../../LICENSE) for details

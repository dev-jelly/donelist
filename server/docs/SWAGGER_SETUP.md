# Swagger/OpenAPI Setup Guide

## Overview

This document explains how to use and maintain the Swagger/OpenAPI documentation for the Donelist API.

## Installation

The required dependencies are already installed:

```bash
go get -u github.com/swaggo/gin-swagger
go get -u github.com/swaggo/files
```

## Generating Documentation

To generate the Swagger documentation from code annotations:

```bash
# Install swag CLI (only needed once)
go install github.com/swaggo/swag/cmd/swag@latest

# Generate documentation
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

This will create/update:
- `docs/docs.go` - Generated Go code
- `docs/swagger.json` - OpenAPI 2.0 JSON spec
- `docs/swagger.yaml` - OpenAPI 2.0 YAML spec

## Using the Makefile

A convenience Makefile is provided at `Makefile.swagger`:

```bash
# Install swag CLI
make -f Makefile.swagger swagger-init

# Generate docs
make -f Makefile.swagger swagger-generate

# Start server and view docs
make -f Makefile.swagger swagger-serve

# Validate OpenAPI spec
make -f Makefile.swagger swagger-validate

# Generate SDKs for all languages
make -f Makefile.swagger sdk-generate-all
```

## Accessing the Documentation

Once the server is running:

1. **Swagger UI**: http://localhost:8080/swagger/index.html
2. **JSON spec**: http://localhost:8080/swagger/doc.json
3. **YAML spec**: http://localhost:8080/swagger/doc.yaml

## Annotation Structure

### General API Info (main.go)

```go
// @title Donelist API
// @version 1.0
// @description API description here
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

### Handler Annotations

```go
// @Summary Short summary
// @Description Detailed description
// @Tags TagName
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param paramName paramType dataType required "description"
// @Success 200 {object} ResponseType "description"
// @Failure 400 {object} map[string]string "error description"
// @Router /path [method]
func HandlerFunction(c *gin.Context) {
    // Implementation
}
```

## Parameter Types

- **body**: Request body parameter
- **path**: URL path parameter (e.g., `/users/{id}`)
- **query**: Query string parameter (e.g., `?limit=10`)
- **header**: HTTP header parameter

## Response Types

For complex responses, define models:

```go
// swagger:model User
type User struct {
    ID    string `json:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
    Email string `json:"email" example:"user@example.com"`
}
```

For simple responses:
- `map[string]string` - Simple key-value pairs
- `map[string]interface{}` - Mixed types

## Current API Coverage

All handlers have been annotated:

### Authentication (`/auth`)
- ✅ POST `/auth/register` - Register new user
- ✅ POST `/auth/login` - Login user
- ✅ POST `/auth/refresh` - Refresh access token
- ✅ POST `/auth/logout` - Logout from current device
- ✅ POST `/auth/logout-all` - Logout from all devices

### Users (`/users`)
- ✅ GET `/users/me` - Get current user profile
- ✅ PATCH `/users/me` - Update current user profile
- ✅ DELETE `/users/me` - Delete current user account

### Check-ins (`/checkins`)
- ✅ POST `/checkins` - Create new check-in
- ✅ GET `/checkins` - List check-ins with filtering
- ✅ GET `/checkins/{id}` - Get check-in by ID
- ✅ PATCH `/checkins/{id}` - Update check-in
- ✅ DELETE `/checkins/{id}` - Delete check-in
- ✅ GET `/checkins/{id}/history` - Get edit history (Premium)

### Categories (`/categories`)
- ✅ POST `/categories` - Create category
- ✅ GET `/categories` - List categories
- ✅ GET `/categories/{id}` - Get category by ID
- ✅ PATCH `/categories/{id}` - Update category
- ✅ DELETE `/categories/{id}` - Delete category

### Tags (`/tags`)
- ✅ POST `/tags` - Create tag
- ✅ GET `/tags` - List tags
- ✅ GET `/tags/{id}` - Get tag by ID
- ✅ GET `/tags/autocomplete` - Autocomplete tags
- ✅ GET `/tags/popular` - Get popular tags

## Adding New Endpoints

When adding new endpoints:

1. Add handler function with Swagger annotations
2. Register route in `internal/api/routes/routes.go`
3. Regenerate docs: `swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal`
4. Test in Swagger UI

## SDK Generation

### TypeScript/Axios

```bash
make -f Makefile.swagger sdk-typescript
```

Generates client in `sdk/typescript/` with:
- Type-safe API client
- Promise-based requests
- Automatic serialization

Usage:
```typescript
import { DefaultApi } from './sdk/typescript';

const api = new DefaultApi({ basePath: 'http://localhost:8080' });
const response = await api.authLoginPost({ email, password });
```

### Python

```bash
make -f Makefile.swagger sdk-python
```

Generates client in `sdk/python/` with:
- Pythonic API
- Type hints
- Documentation

Usage:
```python
from donelist_api_client import ApiClient, DefaultApi

api = DefaultApi(ApiClient())
response = api.auth_login_post({"email": email, "password": password})
```

### Go

```bash
make -f Makefile.swagger sdk-go
```

Generates client in `sdk/go/` with:
- Idiomatic Go code
- Context support
- Error handling

Usage:
```go
import "github.com/yourusername/donelist/sdk/go"

client := swagger.NewAPIClient(swagger.NewConfiguration())
resp, err := client.DefaultApi.AuthLoginPost(ctx, loginRequest)
```

## Testing

### Manual Testing

1. Start server: `go run cmd/api/main.go`
2. Open Swagger UI: http://localhost:8080/swagger/index.html
3. Click "Authorize" and enter Bearer token
4. Test endpoints interactively

### Automated Testing

```bash
# Validate spec
swagger validate docs/swagger.json

# Run API tests with Postman
newman run postman_collection.json
```

## Troubleshooting

### "Cannot GET /swagger/index.html"

Make sure:
1. Swagger route is registered in `routes.go`
2. Docs are generated: `swag init ...`
3. Server is running

### "Spec not found"

Regenerate docs:
```bash
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

### "Type not found"

Add `--parseDependency --parseInternal` flags to parse all types:
```bash
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

## Best Practices

1. **Keep annotations close to code** - Update docs when changing handlers
2. **Use examples** - Add `example:"value"` tags to struct fields
3. **Document errors** - Include all possible failure responses
4. **Version your API** - Use `/api/v1`, `/api/v2` prefixes
5. **Test regularly** - Verify docs match implementation
6. **Generate before committing** - Keep generated docs in sync

## CI/CD Integration

Add to your CI pipeline:

```yaml
# .github/workflows/docs.yml
name: API Documentation

on: [push, pull_request]

jobs:
  validate-docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Set up Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.21

      - name: Install swag
        run: go install github.com/swaggo/swag/cmd/swag@latest

      - name: Generate docs
        run: swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal

      - name: Validate spec
        run: swagger validate docs/swagger.json
```

## Resources

- [Swag Documentation](https://github.com/swaggo/swag)
- [OpenAPI Specification](https://swagger.io/specification/)
- [Gin-Swagger](https://github.com/swaggo/gin-swagger)
- [OpenAPI Generator](https://openapi-generator.tech/)

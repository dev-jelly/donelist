# Developer Onboarding Guide

Welcome to the Donelist API development team! This guide will help you get set up and productive quickly.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Environment Setup](#environment-setup)
3. [Project Structure](#project-structure)
4. [Development Workflow](#development-workflow)
5. [Testing](#testing)
6. [Code Standards](#code-standards)
7. [Common Tasks](#common-tasks)
8. [Troubleshooting](#troubleshooting)
9. [Resources](#resources)

## Prerequisites

### Required Software

- **Go**: 1.24 or later ([Download](https://golang.org/dl/))
- **PostgreSQL**: 15 or later ([Download](https://www.postgresql.org/download/))
- **Redis**: 7 or later ([Download](https://redis.io/download))
- **Docker**: Latest version ([Download](https://www.docker.com/get-started))
- **Make**: Usually pre-installed on macOS/Linux
- **Git**: Latest version

### Recommended Tools

- **VS Code** with Go extension
- **Postman** or **Insomnia** for API testing
- **TablePlus** or **DBeaver** for database management
- **Redis Insight** for Redis management

### Knowledge Prerequisites

- Proficiency in Go programming
- Understanding of REST API design
- Familiarity with SQL (PostgreSQL)
- Experience with Git workflows
- Basic understanding of Docker

## Environment Setup

### 1. Clone the Repository

```bash
git clone https://github.com/dev-jelly/donelist.git
cd donelist/server
```

### 2. Install Dependencies

```bash
# Install Go dependencies
go mod download

# Install development tools
make install-tools
```

This installs:
- `swag` - Swagger documentation generator
- `golangci-lint` - Comprehensive linter
- `gosec` - Security scanner
- `migrate` - Database migration tool

### 3. Configure Environment

Create a `.env` file:

```bash
cp .env.example .env
```

Edit `.env` with your local settings:

```env
# Application
GO_ENV=development
APP_PORT=8080
LOG_LEVEL=debug

# Database
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/donelist_dev?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# Redis
REDIS_URL=redis://localhost:6379/0

# JWT
JWT_SECRET=your-super-secret-key-min-32-chars-long-for-dev
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=7d

# CORS
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173

# Rate Limiting
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS_PER_MINUTE=60
```

### 4. Start Dependencies

#### Option A: Docker Compose (Recommended)

```bash
# Start PostgreSQL and Redis
docker-compose up -d postgres redis

# Check services are running
docker-compose ps
```

#### Option B: Local Installation

```bash
# PostgreSQL
brew install postgresql@15
brew services start postgresql@15

# Redis
brew install redis
brew services start redis
```

### 5. Setup Database

```bash
# Create database
createdb donelist_dev

# Run migrations
make migrate-up

# Verify migrations
make migrate-status
```

### 6. Verify Setup

```bash
# Build the application
make build

# Run tests
make test

# Start the server
make run
```

Visit http://localhost:8080/health - you should see:

```json
{
  "status": "healthy",
  "timestamp": "2024-11-24T10:00:00Z"
}
```

### 7. Access Swagger UI

Visit http://localhost:8080/swagger/index.html to see the interactive API documentation.

## Project Structure

```
server/
├── cmd/
│   └── api/
│       └── main.go              # Application entry point
├── internal/                    # Private application code
│   ├── api/
│   │   ├── handlers/           # HTTP request handlers
│   │   ├── middleware/         # HTTP middleware
│   │   └── routes/             # Route definitions
│   ├── auth/                   # Authentication logic
│   ├── checkin/                # Check-in domain logic
│   ├── category/               # Category domain logic
│   ├── user/                   # User domain logic
│   ├── config/                 # Configuration management
│   └── testutil/               # Testing utilities
├── pkg/                        # Public reusable packages
│   ├── cache/                  # Redis cache wrapper
│   ├── database/               # Database utilities
│   └── logger/                 # Logging utilities
├── migrations/                 # Database migrations
├── docs/                       # Documentation
│   ├── api/                    # API documentation
│   └── testing/                # Testing guides
├── tests/                      # Test suites
│   ├── integration/            # Integration tests
│   ├── e2e/                    # End-to-end tests
│   └── load/                   # Performance tests
├── scripts/                    # Utility scripts
├── Makefile                    # Build automation
├── go.mod                      # Go module definition
└── README.md                   # Project overview
```

## Development Workflow

### Daily Workflow

```bash
# 1. Pull latest changes
git pull origin main

# 2. Create feature branch
git checkout -b feature/your-feature-name

# 3. Make changes and test frequently
make test

# 4. Run linter
make lint

# 5. Commit changes
git add .
git commit -m "feat: add new feature"

# 6. Push and create PR
git push origin feature/your-feature-name
gh pr create
```

### Branching Strategy

- `main` - Production-ready code
- `develop` - Integration branch
- `feature/*` - New features
- `bugfix/*` - Bug fixes
- `hotfix/*` - Urgent production fixes

### Commit Message Convention

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add user profile endpoint
fix: resolve JWT expiration bug
docs: update API documentation
test: add integration tests for auth
refactor: simplify database query logic
perf: optimize timeline query
```

## Testing

### Running Tests

```bash
# All tests
make test

# With coverage
make test-coverage

# Specific package
go test -v ./internal/auth/...

# Integration tests only
make test-integration

# E2E tests only
make test-e2e

# Watch mode (requires gotestsum)
gotestsum --watch
```

### Writing Tests

#### Unit Test Template

```go
package mypackage_test

import (
    "testing"

    "github.com/dev-jelly/donelist/internal/mypackage"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test input"

    // Act
    result, err := mypackage.MyFunction(input)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, "expected", result)
}
```

### Test Coverage Goals

- Overall: 80% minimum
- Critical paths: 95% minimum
- New code: Must maintain or improve coverage

## Code Standards

### Linting

```bash
# Run all linters
make lint

# Auto-fix issues
make lint-fix
```

### Code Formatting

```bash
# Format code
make fmt

# Format and organize imports
goimports -w .
```

### Best Practices

#### DO

- ✅ Use meaningful variable names
- ✅ Write tests for all new code
- ✅ Handle errors properly (don't ignore them)
- ✅ Use context for cancellation
- ✅ Document exported functions
- ✅ Keep functions small and focused
- ✅ Use dependency injection

#### DON'T

- ❌ Use `panic()` except in init functions
- ❌ Ignore errors (`_ = someFunc()`)
- ❌ Use global variables
- ❌ Return nil slices (return empty slice)
- ❌ Mix business logic with HTTP handlers
- ❌ Commit commented-out code
- ❌ Use bare `interface{}`

### Code Review Checklist

Before submitting a PR:

- [ ] All tests pass
- [ ] Coverage maintained/improved
- [ ] Linter passes with no warnings
- [ ] Code is properly formatted
- [ ] Documentation updated
- [ ] No unnecessary dependencies added
- [ ] Migration scripts included (if needed)
- [ ] Breaking changes documented

## Common Tasks

### Adding a New Endpoint

1. **Define handler** in `internal/api/handlers/`

```go
// @Summary Create category
// @Description Create a new category for the authenticated user
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateCategoryRequest true "Category details"
// @Success 201 {object} Category
// @Failure 400 {object} map[string]string
// @Router /categories [post]
func (h *CategoryHandler) Create(c *gin.Context) {
    // Implementation
}
```

2. **Register route** in `internal/api/routes/routes.go`

```go
categories := v1.Group("/categories")
categories.Use(authMiddleware.RequireAuth())
{
    categories.POST("", categoryHandler.Create)
}
```

3. **Write tests** in `internal/api/handlers/category_handler_test.go`

4. **Regenerate Swagger docs**

```bash
make swagger-generate
```

5. **Test endpoint**

```bash
# Start server
make run

# Test with curl
curl -X POST http://localhost:8080/api/v1/categories \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Work", "color": "#FF5733"}'
```

### Creating a Database Migration

```bash
# Create migration files
make migrate-create name=add_user_preferences

# This creates:
# migrations/000XXX_add_user_preferences.up.sql
# migrations/000XXX_add_user_preferences.down.sql
```

Edit the migration files:

```sql
-- migrations/000XXX_add_user_preferences.up.sql
CREATE TABLE user_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme VARCHAR(20) DEFAULT 'light',
    language VARCHAR(10) DEFAULT 'en',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_preferences_user_id ON user_preferences(user_id);
```

```sql
-- migrations/000XXX_add_user_preferences.down.sql
DROP TABLE IF EXISTS user_preferences;
```

Apply migration:

```bash
make migrate-up
```

### Adding a New Service

1. Create package directory

```bash
mkdir -p internal/notification
```

2. Define domain types

```go
// internal/notification/types.go
package notification

type Notification struct {
    ID        string
    UserID    string
    Message   string
    Type      string
    Read      bool
    CreatedAt time.Time
}
```

3. Define repository interface

```go
// internal/notification/repository.go
package notification

type Repository interface {
    Create(ctx context.Context, notification *Notification) error
    List(ctx context.Context, userID string) ([]*Notification, error)
    MarkAsRead(ctx context.Context, id string) error
}
```

4. Implement service

```go
// internal/notification/service.go
package notification

type Service struct {
    repo   Repository
    logger *zap.Logger
}

func NewService(repo Repository, logger *zap.Logger) *Service {
    return &Service{
        repo:   repo,
        logger: logger,
    }
}
```

5. Write tests

```go
// internal/notification/service_test.go
package notification_test
```

## Troubleshooting

### Database Connection Issues

```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Check connection
psql -h localhost -U postgres -d donelist_dev -c "SELECT 1;"

# Reset database
make db-reset
```

### Redis Connection Issues

```bash
# Check Redis is running
redis-cli ping
# Should return: PONG

# Restart Redis
docker-compose restart redis
```

### Build Errors

```bash
# Clean build cache
go clean -cache -modcache -i -r

# Reinstall dependencies
go mod download
go mod tidy
```

### Test Failures

```bash
# Clean test cache
go clean -testcache

# Run specific test with verbose output
go test -v -run TestSpecificTest ./internal/auth/...

# Check for race conditions
go test -race ./...
```

### Migration Issues

```bash
# Check migration status
make migrate-status

# Force version (use carefully!)
make migrate-force version=20240101120000

# Rollback one migration
make migrate-down
```

## Resources

### Documentation

- [Go Documentation](https://golang.org/doc/)
- [Gin Web Framework](https://gin-gonic.com/docs/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Redis Documentation](https://redis.io/documentation)

### Internal Resources

- [API Documentation](./docs/api/README.md)
- [Testing Guide](./docs/testing/TESTING_GUIDE.md)
- [Architecture Overview](./docs/ARCHITECTURE.md)
- [Database Schema](./docs/DATABASE_SCHEMA.md)

### Tools

- [Swagger UI](http://localhost:8080/swagger/index.html) - API documentation
- [Prometheus](http://localhost:9090) - Metrics
- [Grafana](http://localhost:3000) - Dashboards

### Communication

- **Slack**: #donelist-dev
- **GitHub Issues**: Bug reports and feature requests
- **Email**: dev@donelist.io
- **Wiki**: https://github.com/dev-jelly/donelist/wiki

## Getting Help

1. **Check Documentation**: Start with this guide and other docs
2. **Search Issues**: Someone may have had the same problem
3. **Ask in Slack**: #donelist-dev channel
4. **Create Issue**: If it's a bug or feature request
5. **Email Team**: dev@donelist.io

## Next Steps

Now that you're set up:

1. **Explore the codebase**: Start with `cmd/api/main.go`
2. **Run the test suite**: `make test`
3. **Try the API**: Use Swagger UI or Postman
4. **Pick a ticket**: Look for "good first issue" labels
5. **Ask questions**: We're here to help!

Welcome to the team! 🚀

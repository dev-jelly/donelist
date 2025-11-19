# Donelist Server (Go Backend API)

**Version**: 1.0.0
**Language**: Go 1.23+
**Framework**: Gin

---

## 🚀 Quick Start

### Prerequisites
- Go 1.23 or higher
- PostgreSQL 16
- Redis 7
- Make (optional, for convenience)

### Installation

1. **Clone and navigate**:
```bash
cd server
```

2. **Install dependencies**:
```bash
make deps
# or
go mod download
```

3. **Setup environment**:
```bash
make setup
# or
cp .env.example .env
# Edit .env with your configuration
```

4. **Run the server**:
```bash
make run
# or
go run cmd/api/main.go
```

5. **Build binary**:
```bash
make build
# Binary will be in ./bin/api
```

---

## 📁 Project Structure

```
server/
├── cmd/
│   ├── api/              # API server entry point
│   │   └── main.go
│   └── migrate/          # Database migration tool
│       └── main.go
├── internal/
│   ├── api/              # HTTP layer
│   │   ├── handlers/     # Request handlers
│   │   ├── middleware/   # HTTP middleware
│   │   └── routes/       # Route definitions
│   ├── auth/             # Authentication (JWT, password)
│   ├── checkin/          # Check-in domain
│   ├── timeline/         # Timeline domain
│   ├── user/             # User domain
│   ├── category/         # Category domain
│   ├── tag/              # Tag domain
│   ├── realtime/         # WebSocket real-time
│   └── config/           # Configuration management
├── pkg/
│   ├── database/         # Database utilities
│   ├── logger/           # Logging utilities
│   ├── validator/        # Custom validators
│   └── errors/           # Error handling
├── migrations/           # SQL migrations
├── tests/
│   ├── integration/      # Integration tests
│   └── load/             # Load tests
├── deployments/
│   ├── docker/           # Docker & docker-compose
│   └── k8s/              # Kubernetes manifests
├── scripts/              # Helper scripts
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 🔧 Development

### Available Make Commands

```bash
make help              # Show all available commands

# Build
make build             # Build API binary
make build-all         # Build all binaries

# Run
make run               # Run the application
make run-dev           # Run with hot reload (requires air)

# Test
make test              # Run tests
make test-coverage     # Run tests with coverage report
make test-integration  # Run integration tests

# Database
make migrate-up        # Run migrations
make migrate-down      # Rollback migrations
make migrate-create name=create_table  # Create new migration
make db-reset          # Reset database
make db-seed           # Seed test data

# Docker
make docker-build      # Build Docker image
make docker-run        # Run Docker container
make docker-compose-up # Start all services

# Quality
make lint              # Run linter
make fmt               # Format code
make vet               # Run go vet

# Other
make clean             # Clean build artifacts
make deps              # Download dependencies
make setup             # Setup dev environment
```

### Running Tests

```bash
# Unit tests
go test -v ./...

# With coverage
make test-coverage

# Integration tests
make test-integration

# Load tests
make load-test
```

---

## 🌐 API Endpoints

### Health Check
- `GET /health` - Health check
- `GET /ready` - Readiness check

### Authentication
- `POST /auth/register` - User registration
- `POST /auth/login` - User login
- `POST /auth/refresh` - Refresh access token
- `POST /auth/logout` - User logout

### Users
- `GET /users/me` - Get current user
- `PATCH /users/me` - Update current user
- `DELETE /users/me` - Delete current user

### Check-ins
- `POST /checkins` - Create check-in
- `GET /checkins` - List check-ins
- `GET /checkins/:id` - Get check-in by ID
- `PATCH /checkins/:id` - Update check-in
- `DELETE /checkins/:id` - Delete check-in
- `GET /checkins/:id/history` - Get edit history (Premium)

### Timeline
- `GET /timeline/daily` - Get daily timeline
- `GET /timeline/weekly` - Get weekly timeline
- `GET /timeline/monthly` - Get monthly timeline

### Categories
- `GET /categories` - List categories
- `POST /categories` - Create category (Premium)
- `PATCH /categories/:id` - Update category
- `DELETE /categories/:id` - Delete category

### Tags
- `GET /tags` - List tags
- `POST /tags` - Create tag

### WebSocket
- `GET /ws?token=<jwt>` - WebSocket connection

Full API documentation: See `/docs/api/API_SPECIFICATION.md`

---

## 🗄️ Database

### Migrations

```bash
# Create new migration
make migrate-create name=add_column_to_users

# Run migrations
make migrate-up

# Rollback last migration
make migrate-down

# Reset database
make db-reset
```

### Schema
- Full schema: See `/docs/database/DATABASE_SCHEMA.md`
- Tables: users, checkins, categories, tags, subscriptions, payments, etc.

---

## 🐳 Docker

### Build Image

```bash
make docker-build
```

### Run with Docker Compose

```bash
# Start all services (API, PostgreSQL, Redis)
make docker-compose-up

# Stop all services
make docker-compose-down
```

---

## ☸️ Kubernetes Deployment

See `/docs/deployment/K3S_DEPLOYMENT_GUIDE.md` for detailed deployment instructions.

```bash
# Build and push image
docker build -t donelist/api:latest -f deployments/docker/Dockerfile .
docker push donelist/api:latest

# Deploy to K3s
kubectl apply -f deployments/k8s/
```

---

## 🧪 Testing

### Unit Tests

```bash
go test -v ./internal/...
```

### Integration Tests

```bash
go test -v -tags=integration ./tests/integration/...
```

### Load Testing

```bash
# Install k6
brew install k6

# Run load test
k6 run tests/load/checkin_load_test.js
```

---

## 📊 Monitoring

### Health Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Readiness check
curl http://localhost:8080/ready
```

### Metrics

Prometheus metrics available at `/metrics` (to be implemented)

---

## 🔒 Security

- JWT-based authentication
- HTTPS only in production
- Rate limiting per user/endpoint
- Input validation
- SQL injection protection (parameterized queries)
- CORS policy

---

## 📝 Environment Variables

See `.env.example` for all available environment variables.

Key variables:
- `APP_ENV`: Environment (development/production)
- `APP_PORT`: API server port (default: 8080)
- `POSTGRES_*`: Database configuration
- `REDIS_*`: Redis configuration
- `JWT_SECRET`: JWT signing secret

---

## 🤝 Contributing

1. Create feature branch
2. Write code with tests
3. Run linter: `make lint`
4. Run tests: `make test`
5. Commit changes
6. Create pull request

---

## 📚 Documentation

- [PRD](../docs/planning/PRD.md)
- [System Architecture](../docs/architecture/SYSTEM_ARCHITECTURE.md)
- [Database Schema](../docs/database/DATABASE_SCHEMA.md)
- [API Specification](../docs/api/API_SPECIFICATION.md)
- [K3s Deployment](../docs/deployment/K3S_DEPLOYMENT_GUIDE.md)

---

## 🐛 Troubleshooting

### Port already in use
```bash
# Find process using port 8080
lsof -i :8080

# Kill process
kill -9 <PID>
```

### Database connection error
```bash
# Check if PostgreSQL is running
psql -h localhost -U donelist -d donelist

# Reset database
make db-reset
```

### Build errors
```bash
# Clean and rebuild
make clean
make deps
make build
```

---

**Status**: ✅ Phase 1 Complete (Foundation)
**Next**: Phase 2 - Database & Configuration
**Owner**: Backend Team

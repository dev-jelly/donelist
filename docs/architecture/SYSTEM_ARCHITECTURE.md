# Donelist - System Architecture

**Version**: 1.0
**Date**: 2025-11-10
**Tech Stack**: Go + PostgreSQL + K3s

---

## 📐 Architecture Overview

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        CLIENTS                               │
├──────────────┬──────────────┬──────────────┬────────────────┤
│   iOS/macOS  │   Android    │     Web      │   API Clients  │
│   (SwiftUI)  │  (Compose)   │   (React)    │   (REST/WS)    │
└──────┬───────┴──────┬───────┴──────┬───────┴────────┬───────┘
       │              │              │                │
       └──────────────┴──────────────┴────────────────┘
                          │
                          ▼
       ┌──────────────────────────────────────────────┐
       │            Load Balancer (Traefik)           │
       │          (Ingress Controller in K3s)         │
       └────────────────────┬─────────────────────────┘
                            │
       ┌────────────────────┴─────────────────────────┐
       │                                              │
       ▼                                              ▼
┌─────────────────┐                        ┌─────────────────┐
│   API Gateway   │◄──────────────────────►│  WebSocket      │
│   (Go + Gin)    │                        │  Server (Go)    │
│   REST API      │                        │  Real-time      │
└────────┬────────┘                        └────────┬────────┘
         │                                          │
         └──────────────────┬───────────────────────┘
                            │
       ┌────────────────────┴─────────────────────────┐
       │                                              │
       ▼                                              ▼
┌─────────────────┐                        ┌─────────────────┐
│  PostgreSQL     │                        │     Redis       │
│  (Primary DB)   │                        │    (Cache)      │
│  • Users        │                        │  • Sessions     │
│  • Check-ins    │                        │  • Rate Limit   │
│  • Timelines    │                        │  • Real-time    │
└─────────────────┘                        └─────────────────┘
```

---

## 🏗️ Technology Stack Details

### Backend: Go

**Why Go?**
- ✅ **고성능**: 네이티브 컴파일, 낮은 메모리 사용
- ✅ **동시성**: Goroutines으로 실시간 처리 최적화
- ✅ **타입 안정성**: 컴파일 타임 에러 검출
- ✅ **작은 컨테이너 이미지**: 배포 최적화
- ✅ **풍부한 에코시스템**: 성숙한 라이브러리

**Core Libraries**:
```go
// Web Framework
"github.com/gin-gonic/gin"           // HTTP router & middleware

// Database
"github.com/jackc/pgx/v5"            // PostgreSQL driver
"github.com/jmoiron/sqlx"            // SQL extensions

// Migration
"github.com/golang-migrate/migrate"  // DB migrations

// WebSocket
"github.com/gorilla/websocket"       // WebSocket support

// Authentication
"github.com/golang-jwt/jwt/v5"       // JWT tokens

// Configuration
"github.com/spf13/viper"             // Config management

// Validation
"github.com/go-playground/validator" // Input validation

// Redis
"github.com/redis/go-redis/v9"       // Redis client

// Logging
"go.uber.org/zap"                    // Structured logging

// Testing
"github.com/stretchr/testify"        // Testing toolkit
```

### Database: PostgreSQL 16

**Why PostgreSQL?**
- ✅ **ACID 보장**: 데이터 일관성 critical
- ✅ **JSON 지원**: Flexible 스키마 (tags, metadata)
- ✅ **Time-series 최적화**: TimescaleDB extension 가능
- ✅ **Full-text search**: 검색 기능 내장
- ✅ **성숙한 에코시스템**: 풍부한 도구 및 모니터링

**Extensions Used**:
```sql
-- UUID 지원
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Full-text search
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Time-series (optional, for analytics)
-- CREATE EXTENSION IF NOT EXISTS "timescaledb";
```

### Orchestration: K3s

**Why K3s?**
- ✅ **경량**: 40MB 바이너리, 낮은 리소스
- ✅ **완전한 K8s**: 프로덕션 급 기능
- ✅ **Single binary**: 설치 및 관리 간단
- ✅ **자동 TLS**: Traefik ingress 내장
- ✅ **Edge 최적화**: IoT/Edge 배포 가능

---

## 🔧 Backend Architecture (Go)

### Project Structure

```
donelist-server/
├── cmd/
│   ├── api/                 # API 서버 entry point
│   │   └── main.go
│   ├── worker/              # Background worker
│   │   └── main.go
│   └── migrate/             # Migration tool
│       └── main.go
├── internal/
│   ├── api/                 # API handlers
│   │   ├── handlers/        # HTTP handlers
│   │   ├── middleware/      # Middleware
│   │   └── routes/          # Route definitions
│   ├── auth/                # Authentication
│   │   ├── jwt.go
│   │   └── service.go
│   ├── checkin/             # Check-in domain
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── validator.go
│   ├── timeline/            # Timeline domain
│   │   ├── model.go
│   │   ├── repository.go
│   │   └── service.go
│   ├── user/                # User domain
│   │   ├── model.go
│   │   ├── repository.go
│   │   └── service.go
│   ├── notification/        # Notification service
│   │   ├── push.go
│   │   └── scheduler.go
│   ├── realtime/            # WebSocket service
│   │   ├── hub.go
│   │   └── client.go
│   └── config/              # Configuration
│       └── config.go
├── pkg/
│   ├── database/            # Database utilities
│   │   ├── postgres.go
│   │   └── redis.go
│   ├── logger/              # Logging utilities
│   │   └── logger.go
│   ├── validator/           # Custom validators
│   │   └── validator.go
│   └── errors/              # Error handling
│       └── errors.go
├── migrations/              # SQL migrations
│   ├── 000001_init.up.sql
│   ├── 000001_init.down.sql
│   └── ...
├── deployments/             # K3s manifests
│   ├── k8s/
│   │   ├── namespace.yaml
│   │   ├── configmap.yaml
│   │   ├── secret.yaml
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── ingress.yaml
│   │   └── hpa.yaml
│   └── docker/
│       ├── Dockerfile
│       └── docker-compose.yml
├── scripts/                 # Helper scripts
│   ├── setup.sh
│   ├── migrate.sh
│   └── deploy.sh
├── tests/
│   ├── integration/
│   └── e2e/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Layered Architecture

```
┌──────────────────────────────────────────────────┐
│                   API Layer                       │
│  (HTTP Handlers, WebSocket, Middleware)          │
└────────────────────┬─────────────────────────────┘
                     │
┌────────────────────┴─────────────────────────────┐
│                Service Layer                      │
│  (Business Logic, Validation, Orchestration)     │
└────────────────────┬─────────────────────────────┘
                     │
┌────────────────────┴─────────────────────────────┐
│              Repository Layer                     │
│  (Database Access, Query Building)               │
└────────────────────┬─────────────────────────────┘
                     │
┌────────────────────┴─────────────────────────────┐
│              Database Layer                       │
│  (PostgreSQL, Redis, Connection Pool)            │
└──────────────────────────────────────────────────┘
```

---

## 🗄️ Database Design (Detailed in DATABASE_SCHEMA.md)

### Core Tables

```sql
-- users: 사용자 정보
-- checkins: 체크인 기록
-- timelines: 타임라인 집계
-- subscriptions: 구독 정보
-- categories: 카테고리
-- tags: 태그
```

---

## 🔐 Authentication & Authorization

### JWT-based Authentication

**Flow**:
```
1. User Login (email/password)
   ↓
2. Server validates credentials
   ↓
3. Generate JWT (access + refresh token)
   ↓
4. Client stores tokens (secure storage)
   ↓
5. Client sends access token in Authorization header
   ↓
6. Server validates JWT on each request
```

**JWT Payload**:
```json
{
  "user_id": "uuid",
  "email": "user@example.com",
  "tier": "free|premium",
  "iat": 1699564800,
  "exp": 1699651200
}
```

**Security Measures**:
- Access token: 15분 만료
- Refresh token: 7일 만료
- Token rotation on refresh
- Blacklist for revoked tokens (Redis)

---

## 🚀 Real-time Communication

### WebSocket Architecture

**Hub Pattern**:
```go
// Hub manages all active WebSocket connections
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
}

// Client represents a WebSocket connection
type Client struct {
    hub  *Hub
    conn *websocket.Conn
    send chan []byte
    userID string
}
```

**Use Cases**:
1. **실시간 동기화**: 다른 기기에서 체크인 시 즉시 반영
2. **Notification delivery**: 체크인 알림 실시간 전송
3. **Presence**: 사용자 온라인 상태

---

## ☁️ K3s Deployment Architecture

### Cluster Setup

```yaml
# K3s Cluster Configuration
apiVersion: v1
kind: Cluster
metadata:
  name: donelist-prod
spec:
  nodes:
    - master: 1
    - worker: 2

  # Embedded Traefik Ingress
  ingress:
    enabled: true
    provider: traefik

  # Storage
  storage:
    defaultClass: local-path
```

### Kubernetes Resources

#### 1. Namespace
```yaml
# deployments/k8s/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: donelist
```

#### 2. ConfigMap
```yaml
# deployments/k8s/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: donelist-config
  namespace: donelist
data:
  APP_ENV: "production"
  LOG_LEVEL: "info"
  POSTGRES_HOST: "postgres-service"
  POSTGRES_PORT: "5432"
  POSTGRES_DB: "donelist"
  REDIS_HOST: "redis-service"
  REDIS_PORT: "6379"
```

#### 3. Secret
```yaml
# deployments/k8s/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: donelist-secret
  namespace: donelist
type: Opaque
data:
  # Base64 encoded values
  POSTGRES_USER: <base64>
  POSTGRES_PASSWORD: <base64>
  JWT_SECRET: <base64>
  REDIS_PASSWORD: <base64>
```

#### 4. Deployment
```yaml
# deployments/k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: donelist-api
  namespace: donelist
spec:
  replicas: 3
  selector:
    matchLabels:
      app: donelist-api
  template:
    metadata:
      labels:
        app: donelist-api
    spec:
      containers:
      - name: api
        image: donelist/api:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 8081
          name: websocket
        env:
        - name: APP_ENV
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: APP_ENV
        # ... (more env vars from ConfigMap/Secret)
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

#### 5. Service
```yaml
# deployments/k8s/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: donelist-api-service
  namespace: donelist
spec:
  selector:
    app: donelist-api
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: websocket
    port: 8081
    targetPort: 8081
  type: ClusterIP
```

#### 6. Ingress (Traefik)
```yaml
# deployments/k8s/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: donelist-ingress
  namespace: donelist
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - api.donelist.com
    secretName: donelist-tls
  rules:
  - host: api.donelist.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: donelist-api-service
            port:
              number: 80
      - path: /ws
        pathType: Prefix
        backend:
          service:
            name: donelist-api-service
            port:
              number: 8081
```

#### 7. Horizontal Pod Autoscaler (HPA)
```yaml
# deployments/k8s/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: donelist-api-hpa
  namespace: donelist
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: donelist-api
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

---

## 🐳 Docker Configuration

### Multi-stage Dockerfile

```dockerfile
# deployments/docker/Dockerfile

# Stage 1: Build
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /app/bin/api ./cmd/api

# Stage 2: Runtime
FROM alpine:3.18

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/bin/api /app/api

# Copy migrations (if needed)
COPY --from=builder /app/migrations /app/migrations

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8080 8081

ENTRYPOINT ["/app/api"]
```

### Docker Compose (Local Development)

```yaml
# deployments/docker/docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: donelist
      POSTGRES_USER: donelist
      POSTGRES_PASSWORD: donelist_dev
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U donelist"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  api:
    build:
      context: ../../
      dockerfile: deployments/docker/Dockerfile
    ports:
      - "8080:8080"
      - "8081:8081"
    environment:
      APP_ENV: development
      POSTGRES_HOST: postgres
      POSTGRES_PORT: 5432
      POSTGRES_DB: donelist
      POSTGRES_USER: donelist
      POSTGRES_PASSWORD: donelist_dev
      REDIS_HOST: redis
      REDIS_PORT: 6379
      JWT_SECRET: dev_jwt_secret_change_in_prod
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    volumes:
      - ../../:/app
    command: /app/api

volumes:
  postgres_data:
  redis_data:
```

---

## 🔄 CI/CD Pipeline

### GitHub Actions Workflow

```yaml
# .github/workflows/deploy.yml
name: Build and Deploy

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run tests
        run: |
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out

      - name: Lint
        uses: golangci/golangci-lint-action@v3
        with:
          version: latest

  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2

      - name: Login to Docker Hub
        uses: docker/login-action@v2
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Build and push
        uses: docker/build-push-action@v4
        with:
          context: .
          file: deployments/docker/Dockerfile
          push: true
          tags: donelist/api:${{ github.sha }},donelist/api:latest
          cache-from: type=registry,ref=donelist/api:latest
          cache-to: type=inline

  deploy:
    needs: build
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v3

      - name: Set up kubectl
        uses: azure/setup-kubectl@v3

      - name: Configure kubectl
        run: |
          echo "${{ secrets.KUBECONFIG }}" | base64 -d > kubeconfig.yaml
          export KUBECONFIG=kubeconfig.yaml

      - name: Deploy to K3s
        run: |
          kubectl set image deployment/donelist-api \
            api=donelist/api:${{ github.sha }} \
            -n donelist
          kubectl rollout status deployment/donelist-api -n donelist
```

---

## 📊 Monitoring & Observability

### Prometheus + Grafana Stack

**Metrics to Track**:
- API request rate & latency
- WebSocket connection count
- Database query performance
- Cache hit rate
- Error rate & types
- Resource usage (CPU, Memory)

**Go Metrics Exposure**:
```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Custom metrics
var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )

    checkinDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "checkin_duration_seconds",
            Help: "Check-in processing duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"status"},
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal)
    prometheus.MustRegister(checkinDuration)
}

// Expose metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

---

## 🔒 Security Considerations

### Security Checklist

- [x] **TLS everywhere**: HTTPS + WSS only
- [x] **JWT secret rotation**: 매 90일 rotation
- [x] **Rate limiting**: Per-user, per-endpoint
- [x] **Input validation**: All user inputs sanitized
- [x] **SQL injection prevention**: Parameterized queries only
- [x] **CORS policy**: Strict origin whitelist
- [x] **Secrets management**: Kubernetes Secrets + Vault
- [x] **Container scanning**: Trivy for image scanning
- [x] **Network policies**: Pod-to-pod communication rules
- [x] **Audit logging**: All sensitive operations logged

---

## 🚦 Performance Targets

### API Performance
- **P50 latency**: < 50ms
- **P95 latency**: < 200ms
- **P99 latency**: < 500ms
- **Throughput**: > 1000 req/s per instance

### Database Performance
- **Query latency**: < 10ms for simple queries
- **Connection pool**: 20-50 connections per instance
- **Cache hit rate**: > 80% for read operations

### Availability
- **Uptime SLA**: 99.9% (8.7h downtime/year)
- **RTO (Recovery Time Objective)**: < 15 minutes
- **RPO (Recovery Point Objective)**: < 5 minutes

---

## 📝 Next Steps

1. **Database Schema Design**: 완전한 PostgreSQL 스키마 설계
2. **API Specification**: OpenAPI 3.0 명세서 작성
3. **K3s Deployment Guide**: Step-by-step 배포 가이드
4. **Development Setup**: 로컬 개발 환경 구성 가이드
5. **Testing Strategy**: Unit, Integration, E2E 테스트 계획

---

**Document Status**: Draft v1.0
**Next Review Date**: 2025-11-15
**Owner**: Engineering Team

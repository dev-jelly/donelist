# Plan: Backend API 구현

**Date**: 2025-11-11
**Status**: Planning
**Tech Stack**: Go 1.21 + Gin + PostgreSQL + Redis

---

## 📋 Hypothesis (가설)

### What (무엇을)
Donelist Backend API를 Go + Gin framework로 구현하여, RESTful API와 WebSocket 실시간 통신을 제공하는 프로덕션 급 서버를 구축한다.

### Why (왜 이 접근법인가)
1. **Go 선택 이유**:
   - 고성능 & 낮은 메모리 사용 (컴파일 언어)
   - Goroutines로 동시성 처리 최적화 (실시간 WebSocket에 유리)
   - 작은 Docker 이미지 (배포 최적화)
   - 강력한 타입 시스템 (컴파일 타임 에러 검출)

2. **Gin Framework 선택 이유**:
   - Go에서 가장 빠른 HTTP router (40x faster than others)
   - 미들웨어 시스템 (인증, 로깅, CORS 등)
   - JSON validation built-in
   - 활발한 커뮤니티 & 성숙한 에코시스템

3. **Layered Architecture**:
   - 유지보수성: 각 레이어의 책임 명확히 분리
   - 테스트 용이성: Repository layer를 mock 가능
   - 확장성: 새로운 기능 추가 시 기존 코드 변경 최소화

### How (어떻게)
단계별로 bottom-up 방식으로 구현:
1. Database & Migration (foundation)
2. Repository Layer (data access)
3. Service Layer (business logic)
4. API Layer (HTTP handlers)
5. WebSocket (real-time)
6. Testing (quality assurance)
7. Deployment (K3s ready)

---

## 🎯 Expected Outcomes (예상 결과)

### 정량적 목표
| Metric | Target | Measurement |
|--------|--------|-------------|
| **Test Coverage** | > 80% | `go test -cover` |
| **API Latency (P95)** | < 200ms | Load testing |
| **Build Time** | < 2 min | CI/CD pipeline |
| **Docker Image Size** | < 50MB | `docker images` |
| **Development Time** | ~40 hours | Time tracking |

### 정성적 목표
- ✅ Clean architecture 준수
- ✅ RESTful API design principles
- ✅ SOLID principles 적용
- ✅ Error handling 일관성
- ✅ Logging & monitoring 준비
- ✅ Security best practices (OWASP)

### 기능적 목표
- ✅ **Authentication**: JWT 기반 로그인/회원가입
- ✅ **Check-ins CRUD**: 체크인 생성, 조회, 수정, 삭제
- ✅ **Timeline**: 일간/주간/월간 타임라인 조회
- ✅ **Categories & Tags**: 카테고리 및 태그 관리
- ✅ **WebSocket**: 실시간 체크인 동기화
- ✅ **Health Check**: `/health`, `/ready` endpoints

---

## ⚠️ Risks & Mitigation (위험 요소 및 대응책)

### Risk 1: Database Schema 변경 필요
**Probability**: Medium
**Impact**: Medium
**Mitigation**:
- Migration tool (golang-migrate) 사용으로 rollback 가능
- Down migration 항상 작성
- Staging 환경에서 먼저 테스트

### Risk 2: JWT Token 보안 이슈
**Probability**: Low
**Impact**: High
**Mitigation**:
- JWT secret을 환경변수로 관리 (never hardcode)
- Short-lived access token (15분)
- Refresh token rotation
- Token blacklist (Redis)

### Risk 3: WebSocket 연결 관리 복잡도
**Probability**: Medium
**Impact**: Medium
**Mitigation**:
- Hub pattern 사용 (proven pattern)
- Gorilla WebSocket 라이브러리 (안정적)
- Connection pool 관리
- Heartbeat (ping/pong) 구현

### Risk 4: N+1 Query 문제
**Probability**: Medium
**Impact**: High (Performance)
**Mitigation**:
- GORM/sqlx preloading 사용
- Query 분석 도구 활용
- Database indexing 최적화
- Load testing으로 병목 지점 발견

### Risk 5: Time Zone 처리 실수
**Probability**: High
**Impact**: Medium
**Mitigation**:
- 모든 timestamp는 UTC로 저장
- Client에서 timezone 변환
- `time.Time` with `time.Location` 명시적 사용

---

## 🏗️ Implementation Strategy (구현 전략)

### Phase 1: Foundation (기반 구축) - ~8 hours

#### 1.1 Project Initialization (1h)
```bash
# Directory structure
server/
├── cmd/
│   ├── api/main.go          # API server entry
│   └── migrate/main.go      # Migration tool
├── internal/
│   ├── api/                 # HTTP handlers
│   ├── auth/                # Authentication
│   ├── checkin/             # Check-in domain
│   ├── user/                # User domain
│   ├── config/              # Configuration
│   └── middleware/          # Middleware
├── pkg/
│   ├── database/            # DB utilities
│   ├── logger/              # Logging
│   └── validator/           # Custom validators
├── migrations/              # SQL migrations
├── tests/                   # Integration tests
├── go.mod
├── go.sum
├── Makefile
└── .env.example
```

**Tasks**:
- [x] `go mod init github.com/yourusername/donelist`
- [ ] Create directory structure
- [ ] Setup `.gitignore` for Go
- [ ] Create `.env.example`
- [ ] Setup Makefile for common tasks

#### 1.2 Configuration Management (1h)
```go
// internal/config/config.go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Redis    RedisConfig
    JWT      JWTConfig
}

// Use viper for env management
```

**Library**: `github.com/spf13/viper`

#### 1.3 Logging Setup (1h)
```go
// pkg/logger/logger.go
// Use Zap for structured logging
logger, _ := zap.NewProduction()
defer logger.Sync()
```

**Library**: `go.uber.org/zap`

#### 1.4 Database Connection (2h)
```go
// pkg/database/postgres.go
import (
    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
)

func NewPostgresDB(cfg DatabaseConfig) (*sqlx.DB, error) {
    dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
        cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name, cfg.SSLMode)

    db, err := sqlx.Connect("postgres", dsn)
    if err != nil {
        return nil, err
    }

    // Connection pooling
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    return db, nil
}
```

**Libraries**:
- `github.com/jmoiron/sqlx` (SQL extensions)
- `github.com/lib/pq` (PostgreSQL driver)

#### 1.5 Migration Setup (2h)
```go
// cmd/migrate/main.go
// Use golang-migrate for migrations
import "github.com/golang-migrate/migrate/v4"

// migrations/000001_init.up.sql
// migrations/000001_init.down.sql
```

**Tasks**:
- [ ] Create migration files from `docs/database/DATABASE_SCHEMA.md`
- [ ] Test `migrate up` and `migrate down`
- [ ] Add seed data script

#### 1.6 Redis Connection (1h)
```go
// pkg/database/redis.go
import "github.com/redis/go-redis/v9"

func NewRedisClient(cfg RedisConfig) *redis.Client {
    return redis.NewClient(&redis.Options{
        Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
        Password: cfg.Password,
        DB:       cfg.DB,
    })
}
```

---

### Phase 2: Authentication (인증 시스템) - ~6 hours

#### 2.1 JWT Utilities (2h)
```go
// internal/auth/jwt.go
type JWTService interface {
    GenerateAccessToken(userID uuid.UUID, email string) (string, error)
    GenerateRefreshToken(userID uuid.UUID) (string, error)
    ValidateToken(tokenString string) (*Claims, error)
}

type Claims struct {
    UserID uuid.UUID `json:"user_id"`
    Email  string    `json:"email"`
    Tier   string    `json:"tier"`
    jwt.StandardClaims
}
```

**Library**: `github.com/golang-jwt/jwt/v5`

#### 2.2 Password Hashing (1h)
```go
// internal/auth/password.go
import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

#### 2.3 Auth Middleware (2h)
```go
// internal/middleware/auth.go
func AuthMiddleware(jwtService *auth.JWTService) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(401, gin.H{"error": "Authorization header required"})
            c.Abort()
            return
        }

        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        claims, err := jwtService.ValidateToken(tokenString)
        if err != nil {
            c.JSON(401, gin.H{"error": "Invalid token"})
            c.Abort()
            return
        }

        c.Set("user_id", claims.UserID)
        c.Set("email", claims.Email)
        c.Next()
    }
}
```

#### 2.4 Auth API Handlers (1h)
```go
// internal/api/handlers/auth.go
// POST /auth/register
// POST /auth/login
// POST /auth/refresh
// POST /auth/logout
```

---

### Phase 3: Core Domain Implementation - ~12 hours

#### 3.1 User Domain (3h)

**Repository** (`internal/user/repository.go`):
```go
type Repository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id uuid.UUID) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id uuid.UUID) error
}

type postgresRepository struct {
    db *sqlx.DB
}
```

**Service** (`internal/user/service.go`):
```go
type Service interface {
    Register(ctx context.Context, req RegisterRequest) (*User, error)
    GetProfile(ctx context.Context, userID uuid.UUID) (*User, error)
    UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateRequest) error
}
```

**Handler** (`internal/api/handlers/user.go`):
```go
// GET /users/me
// PATCH /users/me
// DELETE /users/me
```

#### 3.2 Check-in Domain (5h) - **핵심 기능**

**Model** (`internal/checkin/model.go`):
```go
type Checkin struct {
    ID              uuid.UUID      `db:"id" json:"id"`
    UserID          uuid.UUID      `db:"user_id" json:"user_id"`
    Content         string         `db:"content" json:"content"`
    CheckinTime     time.Time      `db:"checkin_time" json:"checkin_time"`
    IntervalMinutes int            `db:"interval_minutes" json:"interval_minutes"`
    CategoryID      *uuid.UUID     `db:"category_id" json:"category_id,omitempty"`
    Metadata        json.RawMessage `db:"metadata" json:"metadata,omitempty"`
    IsEdited        bool           `db:"is_edited" json:"is_edited"`
    CreatedAt       time.Time      `db:"created_at" json:"created_at"`
    UpdatedAt       time.Time      `db:"updated_at" json:"updated_at"`
}
```

**Repository**:
```go
type Repository interface {
    Create(ctx context.Context, checkin *Checkin) error
    GetByID(ctx context.Context, id uuid.UUID) (*Checkin, error)
    List(ctx context.Context, userID uuid.UUID, filter Filter) ([]*Checkin, error)
    Update(ctx context.Context, checkin *Checkin) error
    Delete(ctx context.Context, id uuid.UUID) error
}
```

**Service** (Business logic):
```go
type Service interface {
    CreateCheckin(ctx context.Context, req CreateRequest) (*Checkin, error)
    GetCheckin(ctx context.Context, id uuid.UUID) (*Checkin, error)
    ListCheckins(ctx context.Context, userID uuid.UUID, filter Filter) ([]*Checkin, int, error)
    UpdateCheckin(ctx context.Context, id uuid.UUID, req UpdateRequest) error
    DeleteCheckin(ctx context.Context, id uuid.UUID) error
}

// Business logic: Check premium tier for editing > 2h old checkins
func (s *service) UpdateCheckin(ctx context.Context, id uuid.UUID, req UpdateRequest) error {
    checkin, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return err
    }

    // Check if > 2 hours old
    if time.Since(checkin.CheckinTime) > 2*time.Hour {
        // Require premium tier
        user, _ := s.userRepo.GetByID(ctx, checkin.UserID)
        if user.Tier != "premium" {
            return ErrPremiumRequired
        }
    }

    // Update logic...
}
```

**Handler**:
```go
// POST /checkins
// GET /checkins
// GET /checkins/:id
// PATCH /checkins/:id
// DELETE /checkins/:id
// GET /checkins/:id/history (premium)
```

#### 3.3 Timeline Domain (2h)

**Service**:
```go
type Service interface {
    GetDailyTimeline(ctx context.Context, userID uuid.UUID, date time.Time) (*DailyTimeline, error)
    GetWeeklyTimeline(ctx context.Context, userID uuid.UUID, weekStart time.Time) (*WeeklyTimeline, error)
    GetMonthlyTimeline(ctx context.Context, userID uuid.UUID, year, month int) (*MonthlyTimeline, error)
}
```

**Handler**:
```go
// GET /timeline/daily?date=2025-11-10&timezone=Asia/Seoul
// GET /timeline/weekly?week_start=2025-11-04&timezone=Asia/Seoul
// GET /timeline/monthly?year=2025&month=11&timezone=Asia/Seoul
```

**Note**: Use database functions from `docs/database/DATABASE_SCHEMA.md`:
- `get_user_timeline()`
- `calculate_user_stats()`

#### 3.4 Categories & Tags (2h)

**Categories**:
```go
// GET /categories
// POST /categories (premium)
// PATCH /categories/:id (custom only)
// DELETE /categories/:id (custom only)
```

**Tags**:
```go
// GET /tags
// POST /tags
```

---

### Phase 4: WebSocket Real-time (실시간 통신) - ~4 hours

#### 4.1 Hub Pattern Implementation (2h)
```go
// internal/realtime/hub.go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan Message
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            h.clients[client] = true
            h.mu.Unlock()

        case client := <-h.unregister:
            h.mu.Lock()
            if _, ok := h.clients[client]; ok {
                delete(h.clients, client)
                close(client.send)
            }
            h.mu.Unlock()

        case message := <-h.broadcast:
            h.mu.RLock()
            for client := range h.clients {
                if client.userID == message.UserID {
                    select {
                    case client.send <- message.Data:
                    default:
                        close(client.send)
                        delete(h.clients, client)
                    }
                }
            }
            h.mu.RUnlock()
        }
    }
}
```

#### 4.2 Client Connection Management (1h)
```go
// internal/realtime/client.go
type Client struct {
    hub    *Hub
    conn   *websocket.Conn
    send   chan []byte
    userID uuid.UUID
}

func (c *Client) readPump() {
    // Handle incoming messages
}

func (c *Client) writePump() {
    // Handle outgoing messages
}
```

#### 4.3 WebSocket Handler (1h)
```go
// internal/api/handlers/websocket.go
// GET /ws?token=<jwt_token>

func HandleWebSocket(hub *Hub, jwtService *auth.JWTService) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.Query("token")
        claims, err := jwtService.ValidateToken(token)
        if err != nil {
            c.JSON(401, gin.H{"error": "Invalid token"})
            return
        }

        conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
        if err != nil {
            return
        }

        client := &Client{
            hub:    hub,
            conn:   conn,
            send:   make(chan []byte, 256),
            userID: claims.UserID,
        }

        hub.register <- client

        go client.writePump()
        go client.readPump()
    }
}
```

---

### Phase 5: Middleware & Error Handling - ~3 hours

#### 5.1 Common Middleware (1h)
```go
// internal/middleware/logger.go
func LoggerMiddleware(logger *zap.Logger) gin.HandlerFunc

// internal/middleware/cors.go
func CORSMiddleware() gin.HandlerFunc

// internal/middleware/rate_limit.go
func RateLimitMiddleware(redis *redis.Client) gin.HandlerFunc

// internal/middleware/recovery.go
func RecoveryMiddleware(logger *zap.Logger) gin.HandlerFunc
```

#### 5.2 Error Handling (1h)
```go
// pkg/errors/errors.go
type APIError struct {
    Code       string `json:"code"`
    Message    string `json:"message"`
    StatusCode int    `json:"-"`
}

var (
    ErrUnauthorized    = &APIError{"UNAUTHORIZED", "Unauthorized", 401}
    ErrForbidden       = &APIError{"FORBIDDEN", "Forbidden", 403}
    ErrNotFound        = &APIError{"NOT_FOUND", "Resource not found", 404}
    ErrValidation      = &APIError{"VALIDATION_ERROR", "Validation failed", 400}
    ErrPremiumRequired = &APIError{"PREMIUM_REQUIRED", "Premium subscription required", 403}
)
```

#### 5.3 Request Validation (1h)
```go
// Use go-playground/validator
import "github.com/go-playground/validator/v10"

type CreateCheckinRequest struct {
    Content         string    `json:"content" binding:"required,min=1,max=2000"`
    CheckinTime     time.Time `json:"checkin_time" binding:"required"`
    IntervalMinutes int       `json:"interval_minutes" binding:"required,oneof=15 30 45 60 120"`
    CategoryID      *uuid.UUID `json:"category_id"`
    Tags            []string  `json:"tags" binding:"max=10"`
}
```

---

### Phase 6: Testing - ~5 hours

#### 6.1 Unit Tests (2h)
```go
// internal/user/service_test.go
func TestUserService_Register(t *testing.T) {
    // Mock repository
    mockRepo := &mockUserRepository{}
    service := NewService(mockRepo)

    // Test cases
    tests := []struct {
        name    string
        req     RegisterRequest
        wantErr bool
    }{
        {"valid registration", validReq, false},
        {"duplicate email", dupReq, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := service.Register(context.Background(), tt.req)
            if (err != nil) != tt.wantErr {
                t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

**Library**: `github.com/stretchr/testify`

#### 6.2 Integration Tests (2h)
```go
// tests/integration/checkin_test.go
func TestCheckinAPI_CreateAndGet(t *testing.T) {
    // Setup test database
    db := setupTestDB(t)
    defer db.Close()

    // Create test server
    router := setupTestRouter(db)

    // Test create checkin
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("POST", "/checkins", bytes.NewBuffer(body))
    router.ServeHTTP(w, req)

    assert.Equal(t, 201, w.Code)
}
```

#### 6.3 Load Testing (1h)
```go
// Use vegeta or k6 for load testing
// tests/load/checkin_load_test.js (k6)
import http from 'k6/http';
import { check } from 'k6';

export let options = {
    vus: 100,
    duration: '30s',
};

export default function() {
    let res = http.post('https://api.donelist.com/checkins', payload);
    check(res, {
        'status is 201': (r) => r.status === 201,
        'response time < 200ms': (r) => r.timings.duration < 200,
    });
}
```

---

### Phase 7: Deployment Setup - ~2 hours

#### 7.1 Dockerfile (1h)
```dockerfile
# Already defined in docs/architecture/SYSTEM_ARCHITECTURE.md
# Copy and test
```

#### 7.2 K3s Manifests (1h)
```bash
# Already defined in docs/deployment/K3S_DEPLOYMENT_GUIDE.md
# Ensure all YAML files are ready
```

---

## 📊 Technical Decisions (기술적 결정사항)

### Decision 1: sqlx vs GORM
**Choice**: sqlx
**Rationale**:
- Raw SQL control (performance critical)
- No ORM magic (predictable behavior)
- Lighter weight (smaller binary)
- Better for complex queries (timeline aggregations)

**Trade-off**: More boilerplate code, but acceptable for our use case

### Decision 2: Gin vs Echo vs Chi
**Choice**: Gin
**Rationale**:
- Fastest HTTP router (benchmarks)
- Built-in validation
- Largest community
- Battle-tested (used by many production apps)

### Decision 3: JWT Storage
**Choice**: Access token (15min) + Refresh token (7 days)
**Rationale**:
- Security: Short-lived access token
- UX: Refresh token for seamless experience
- Revocation: Store refresh tokens in DB for blacklist

### Decision 4: WebSocket Library
**Choice**: Gorilla WebSocket
**Rationale**:
- Most mature Go WebSocket library
- RFC 6455 compliant
- Active maintenance
- Used by major projects

### Decision 5: Timezone Handling
**Choice**: Store UTC, convert on client
**Rationale**:
- Database simplicity
- No timezone conversion bugs
- Client knows user's timezone

---

## 📅 Timeline Estimate

| Phase | Tasks | Estimated Time |
|-------|-------|----------------|
| Phase 1 | Foundation | 8 hours |
| Phase 2 | Authentication | 6 hours |
| Phase 3 | Core Domains | 12 hours |
| Phase 4 | WebSocket | 4 hours |
| Phase 5 | Middleware | 3 hours |
| Phase 6 | Testing | 5 hours |
| Phase 7 | Deployment | 2 hours |
| **Total** | | **40 hours** |

**Realistic**: 40-50 hours (accounting for debugging, documentation)

---

## ✅ Definition of Done

### Phase Completion Criteria

Each phase is considered complete when:
1. ✅ **Code Written**: All planned code implemented
2. ✅ **Tests Pass**: Unit tests > 80% coverage
3. ✅ **Manual Test**: API tested with Postman/curl
4. ✅ **Code Review**: Self-review for quality
5. ✅ **Documentation**: API endpoints documented
6. ✅ **Git Commit**: Changes committed with clear message

### Final Completion Criteria

Project is production-ready when:
- ✅ All phases complete
- ✅ Integration tests pass
- ✅ Load testing meets targets (P95 < 200ms)
- ✅ Docker image builds successfully
- ✅ K3s deployment tested
- ✅ Health check endpoints working
- ✅ Logging & monitoring configured
- ✅ Security audit passed

---

## 🚀 Next Steps

1. **Approval**: Review and approve this plan
2. **Setup**: Initialize Go project and dependencies
3. **Execute**: Follow phase-by-phase implementation
4. **Track**: Update `docs/pdca/backend-api/do.md` during implementation
5. **Review**: Complete `docs/pdca/backend-api/check.md` after completion

---

**Plan Status**: ✅ Ready for Approval
**Estimated Start**: 2025-11-11
**Estimated Completion**: 2025-11-15 (if full-time)
**Owner**: Backend Team

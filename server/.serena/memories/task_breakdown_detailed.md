# Donelist 서버 - 상세 작업 분해

## 작업 우선순위 및 예상 시간
- **Phase 1**: WebSocket (2-3시간) - 즉시 시작
- **Phase 2**: 테스트 (4-6시간) - Phase 1 후 진행 권장
- **Phase 3**: 배포 설정 (1-2시간) - 모든 기능 완성 후

**총 예상 시간: 7-11시간**

---

## Phase 1: WebSocket 실시간 통신 (2-3시간)

### Task 1.1: WebSocket Hub 구현 (30분)
**파일**: `internal/websocket/hub.go`
- [ ] Hub 구조체 정의
  - clients map[*Client]bool
  - register chan *Client
  - unregister chan *Client
  - broadcast chan []byte
- [ ] NewHub() 생성자
- [ ] Run() 메서드 (무한 루프)
  - register 채널 처리
  - unregister 채널 처리
  - broadcast 채널 처리
- [ ] 사용자별 필터링 로직

### Task 1.2: WebSocket Client 구현 (40분)
**파일**: `internal/websocket/client.go`
- [ ] Client 구조체 정의
  - hub *Hub
  - conn *websocket.Conn
  - send chan []byte
  - userID uuid.UUID
- [ ] readPump() 구현 (클라이언트 → 서버)
  - Pong 핸들러 설정
  - 메시지 읽기 루프
  - 에러 처리 및 연결 해제
- [ ] writePump() 구현 (서버 → 클라이언트)
  - Ping 주기 설정
  - send 채널 처리
  - 메시지 전송 및 에러 처리
- [ ] serveWs() 핸들러 함수
  - HTTP → WebSocket 업그레이드
  - JWT 인증 확인
  - Client 등록

### Task 1.3: 메시지 타입 정의 (20분)
**파일**: `internal/websocket/message.go`
- [ ] MessageType enum
  - CheckinCreated
  - CheckinUpdated
  - CheckinDeleted
  - TimelineUpdate
- [ ] Message 구조체
  - Type MessageType
  - UserID uuid.UUID
  - Data interface{}
  - Timestamp time.Time
- [ ] JSON 직렬화 함수

### Task 1.4: Check-in Service 통합 (40분)
**파일**: `internal/checkin/service.go` 수정
- [ ] Service 구조체에 Hub 추가
- [ ] Create() 메서드에 이벤트 발행
- [ ] Update() 메서드에 이벤트 발행
- [ ] Delete() 메서드에 이벤트 발행
- [ ] 브로드캐스트 헬퍼 함수 작성

### Task 1.5: 핸들러 및 라우트 추가 (30분)
**파일**: 
- `internal/api/handlers/websocket_handler.go` (생성)
- `internal/api/routes/routes.go` (수정)
- `cmd/api/main.go` (수정)

작업:
- [ ] WebSocketHandler 구현
- [ ] /ws 라우트 추가 (JWT 인증 적용)
- [ ] main.go에서 Hub 초기화 및 실행
- [ ] 빌드 및 테스트

---

## Phase 2: Unit & Integration 테스트 (4-6시간)

### Task 2.1: 테스트 인프라 설정 (1시간)
- [ ] go.mod에 testify 추가
- [ ] go.mod에 testcontainers-go 추가
- [ ] `internal/testutil/db.go` 생성
  - SetupTestDB() 함수
  - TeardownTestDB() 함수
  - testcontainers PostgreSQL 설정
- [ ] `internal/testutil/fixtures.go` 생성
  - 테스트 데이터 생성 헬퍼
  - Mock User, Checkin 생성 함수

### Task 2.2: Repository 레이어 테스트 (1.5시간)
- [ ] `internal/user/repository_test.go`
  - TestCreate
  - TestGetByEmail
  - TestUpdate
  - TestDelete
- [ ] `internal/checkin/repository_test.go`
  - TestCreate
  - TestList (필터링, 페이지네이션)
  - TestUpdate
  - TestDelete
  - TestGetEditHistory
- [ ] `internal/category/repository_test.go`
  - TestCreate
  - TestNameExists
  - TestList
  - TestUpdate
- [ ] `internal/tag/repository_test.go`
  - TestGetOrCreate
  - TestGetByNames

### Task 2.3: Service 레이어 테스트 (1.5시간)
- [ ] `internal/auth/service_test.go`
  - TestRegister (유효성 검증)
  - TestLogin (성공/실패 케이스)
  - TestGenerateTokens
  - TestVerifyToken
- [ ] `internal/checkin/service_test.go`
  - TestCreate (유효성 검증)
  - TestUpdate (프리미엄 제한 검증)
  - TestDelete
  - TestGetEditHistory
- [ ] `internal/timeline/service_test.go`
  - TestGetDaily
  - TestGetWeekly (주 시작일 검증)
  - TestGetMonthly

### Task 2.4: Integration 테스트 (2시간)
**디렉토리**: `tests/integration/`
- [ ] `auth_test.go`
  - 전체 회원가입 → 로그인 → 토큰 갱신 플로우
  - 잘못된 자격증명 시나리오
- [ ] `checkin_test.go`
  - 인증 → 체크인 생성 → 조회 → 수정 → 삭제
  - 카테고리/태그 연결 검증
- [ ] `timeline_test.go`
  - 여러 체크인 생성
  - 일간/주간/월간 조회 검증
  - 날짜 그룹핑 검증

---

## Phase 3: 배포 설정 파일 작성 (1-2시간)

### Task 3.1: Multi-stage Dockerfile (30분)
**파일**: `Dockerfile`
- [ ] Stage 1: Builder
  - golang:1.24-alpine
  - 의존성 캐싱 최적화
  - CGO_ENABLED=0 설정
  - 바이너리 빌드
- [ ] Stage 2: Runtime
  - alpine:latest
  - CA 인증서 설치
  - 바이너리 복사
  - USER 설정 (non-root)
  - EXPOSE 8080
  - ENTRYPOINT 설정
- [ ] 빌드 검증: `docker build -t donelist-api .`

### Task 3.2: Docker Compose 설정 (30분)
**파일**: `docker-compose.yml`, `.env.example`
- [ ] API 서비스 정의
  - build context
  - ports 매핑
  - environment 변수
  - depends_on: postgres, redis
- [ ] PostgreSQL 서비스
  - image: postgres:15-alpine
  - volumes 설정
  - environment 변수
- [ ] Redis 서비스
  - image: redis:7-alpine
  - volumes 설정
- [ ] Networks 정의
- [ ] .env.example 템플릿 작성

### Task 3.3: K3s 배포 매니페스트 (40분)
**디렉토리**: `deploy/k8s/`
- [ ] `deployment.yaml`
  - replicas: 2
  - resources (requests/limits)
  - livenessProbe: /health
  - readinessProbe: /ready
  - env from ConfigMap/Secret
- [ ] `service.yaml`
  - type: ClusterIP
  - port: 8080
  - selector labels
- [ ] `configmap.yaml`
  - 환경 변수 템플릿
  - 주석으로 설명 추가
- [ ] `secret.yaml`
  - 민감 정보 템플릿 (base64)
  - JWT_SECRET, DB_PASSWORD 등
- [ ] `ingress.yaml`
  - host 설정
  - TLS 설정 (선택)
  - path routing

### Task 3.4: 배포 가이드 문서 (30분)
**파일**: `README_DEPLOY.md`
- [ ] Docker 빌드 섹션
  - 빌드 명령어
  - 이미지 태깅 전략
- [ ] 로컬 실행 섹션
  - docker-compose up 명령어
  - 환경 변수 설정 가이드
  - 마이그레이션 실행 방법
- [ ] K3s 배포 섹션
  - kubectl apply 순서
  - ConfigMap/Secret 생성 방법
  - 배포 확인 명령어
- [ ] 트러블슈팅 섹션
  - 로그 확인 방법
  - 일반적인 문제 해결
  - Health check 디버깅

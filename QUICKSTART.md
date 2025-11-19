# DoneList 빠른 시작 가이드

> 5분 안에 DoneList를 로컬에서 실행하세요!

## 🚀 빠른 시작 (Docker Compose 사용)

가장 빠르고 쉬운 방법입니다.

```bash
# 1. 프로젝트 클론
git clone https://github.com/dev-jelly/donelist.git
cd donelist/server

# 2. 환경 변수 설정
cp .env.example .env

# 3. 필수 환경 변수 설정 (최소한의 설정)
cat > .env << 'EOF'
DB_PASSWORD=dev_password_123
REDIS_PASSWORD=redis_password_123
JWT_SECRET=your-super-secret-jwt-key-minimum-32-characters-change-me
EOF

# 4. 모든 서비스 시작
docker-compose up -d

# 5. 헬스 체크
curl http://localhost:8080/health
```

✅ **완료!** API 서버가 http://localhost:8080 에서 실행 중입니다.

---

## 🛠️ 로컬 개발 환경 (Go 직접 실행)

### 필수 요구사항

- Go 1.24+
- PostgreSQL 15+
- Redis 7+
- Make

### 설정 단계

```bash
# 1. 데이터베이스 설정
psql postgres -c "CREATE USER donelist WITH PASSWORD 'dev123';"
psql postgres -c "CREATE DATABASE donelist OWNER donelist;"

# 2. 환경 변수 설정
cd server
cp .env.example .env
# .env 파일에서 DB_PASSWORD와 REDIS_PASSWORD 설정

# 3. 의존성 다운로드 및 마이그레이션
make deps
make migrate-up

# 4. 서버 실행
make run
```

---

## 🧪 테스트 실행

```bash
# 단위 테스트
make test

# 커버리지 포함
make test-coverage

# 통합 테스트
make test-integration
```

---

## 📦 프로덕션 빌드

```bash
# 바이너리 빌드
make build

# Docker 이미지 빌드
docker build -t donelist-api:latest .

# 실행
./bin/api
```

---

## 📊 주요 엔드포인트

| 엔드포인트 | 설명 |
|-----------|------|
| `GET /health` | 헬스 체크 |
| `GET /metrics` | Prometheus 메트릭 |
| `POST /api/v1/auth/register` | 사용자 등록 |
| `POST /api/v1/auth/login` | 로그인 |
| `GET /api/v1/checkins` | 체크인 목록 조회 |
| `POST /api/v1/checkins` | 체크인 생성 |

---

## 🔧 자주 사용하는 명령어

```bash
# 서비스 시작/중지
docker-compose up -d          # 시작
docker-compose down           # 중지
docker-compose restart api    # API 서버만 재시작
docker-compose logs -f api    # 로그 확인

# 데이터베이스 관리
make migrate-up               # 마이그레이션 실행
make migrate-down             # 마이그레이션 롤백
make db-reset                 # 데이터베이스 리셋

# 개발
make run                      # 서버 실행
make test                     # 테스트 실행
make lint                     # 린트 실행
make fmt                      # 코드 포맷팅
```

---

## 🐛 트러블슈팅

### Docker 컨테이너가 시작되지 않음

```bash
# 로그 확인
docker-compose logs -f

# 컨테이너 재시작
docker-compose restart

# 완전히 재시작
docker-compose down -v
docker-compose up -d
```

### 데이터베이스 연결 오류

```bash
# 데이터베이스 컨테이너 상태 확인
docker-compose ps postgres

# 데이터베이스 접속 테스트
docker-compose exec postgres psql -U donelist -d donelist
```

### 포트 충돌

`.env` 파일에서 포트 변경:

```bash
API_PORT=8081    # 기본값: 8080
DB_PORT=5433     # 기본값: 5432
REDIS_PORT=6380  # 기본값: 6379
```

---

## 📚 더 자세한 정보

- **전체 배포 가이드**: [DEPLOYMENT.md](./DEPLOYMENT.md)
- **백업 가이드**: [server/docs/PITR_QUICK_GUIDE.md](./server/docs/PITR_QUICK_GUIDE.md)
- **파티셔닝 가이드**: [server/docs/PARTITIONING_GUIDE.md](./server/docs/PARTITIONING_GUIDE.md)
- **API 문서**: http://localhost:8080/swagger/index.html (서버 실행 후)

---

**Happy Coding! 🎉**

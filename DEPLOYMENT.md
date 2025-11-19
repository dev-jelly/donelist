# DoneList 배포 및 테스트 가이드

> **DoneList** - 체크인 추적 및 관리 시스템의 종합 배포 및 테스트 가이드

## 📑 목차

1. [로컬 개발 환경 설정](#1-로컬-개발-환경-설정)
2. [테스트 실행](#2-테스트-실행)
3. [빌드 및 실행](#3-빌드-및-실행)
4. [Docker 배포](#4-docker-배포)
5. [K3s/Kubernetes 배포](#5-k3skubernetes-배포)
6. [프로덕션 배포](#6-프로덕션-배포)
7. [모니터링 및 유지보수](#7-모니터링-및-유지보수)
8. [트러블슈팅](#8-트러블슈팅)

---

## 1. 로컬 개발 환경 설정

### 1.1 필수 요구사항

- **Go**: 1.24 이상
- **PostgreSQL**: 15.x
- **Redis**: 7.x
- **Make**: 빌드 자동화
- **Docker & Docker Compose**: 컨테이너 실행 (선택사항)
- **Git**: 버전 관리

### 1.2 프로젝트 클론 및 초기 설정

```bash
# 프로젝트 클론
git clone https://github.com/dev-jelly/donelist.git
cd donelist/server

# 의존성 다운로드
make deps

# 환경 변수 설정
cp .env.example .env
```

### 1.3 환경 변수 설정

`.env` 파일을 열고 다음 필수 항목을 설정하세요:

```bash
# 데이터베이스 설정
DB_HOST=localhost
DB_PORT=5432
DB_NAME=donelist
DB_USER=donelist
DB_PASSWORD=your_secure_password_here

# Redis 설정
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password_here

# JWT 비밀키 (최소 32자)
JWT_SECRET=your-super-secret-jwt-key-minimum-32-characters

# Stripe 설정 (결제 기능 사용 시)
STRIPE_SECRET_KEY=sk_test_your_test_key_here
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret_here
```

### 1.4 데이터베이스 설정

#### PostgreSQL 설치 (Mac)

```bash
brew install postgresql@15
brew services start postgresql@15

# 데이터베이스 및 사용자 생성
psql postgres -c "CREATE USER donelist WITH PASSWORD 'your_password';"
psql postgres -c "CREATE DATABASE donelist OWNER donelist;"
psql postgres -c "GRANT ALL PRIVILEGES ON DATABASE donelist TO donelist;"
```

#### PostgreSQL 설치 (Ubuntu/Debian)

```bash
sudo apt update
sudo apt install postgresql-15 postgresql-contrib

# 데이터베이스 및 사용자 생성
sudo -u postgres psql -c "CREATE USER donelist WITH PASSWORD 'your_password';"
sudo -u postgres psql -c "CREATE DATABASE donelist OWNER donelist;"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE donelist TO donelist;"
```

#### 마이그레이션 실행

```bash
# 마이그레이션 실행
make migrate-up

# 마이그레이션 롤백 (필요 시)
make migrate-down

# 데이터베이스 리셋 (개발 환경)
make db-reset
```

### 1.5 Redis 설치

#### Mac

```bash
brew install redis
brew services start redis

# 비밀번호 설정
echo "requirepass your_redis_password_here" >> /opt/homebrew/etc/redis.conf
brew services restart redis
```

#### Ubuntu/Debian

```bash
sudo apt update
sudo apt install redis-server

# 비밀번호 설정
sudo nano /etc/redis/redis.conf
# 다음 줄 추가: requirepass your_redis_password_here

sudo systemctl restart redis
```

---

## 2. 테스트 실행

### 2.1 단위 테스트

```bash
# 모든 테스트 실행
make test

# 커버리지 리포트와 함께 실행
make test-coverage

# 특정 패키지만 테스트
cd server
go test -v ./internal/auth/...

# Race detector와 함께 실행
go test -v -race ./...
```

### 2.2 통합 테스트

```bash
# 통합 테스트 실행
make test-integration

# 또는 직접 실행
cd server
go test -v -tags=integration ./tests/integration/...
```

### 2.3 부하 테스트

```bash
# k6 설치
brew install k6  # Mac
# 또는
sudo apt install k6  # Ubuntu

# 부하 테스트 실행
make load-test

# 또는 직접 실행
k6 run tests/load/checkin_load_test.js
```

### 2.4 CI/CD 환경에서 테스트

GitHub Actions 워크플로우가 자동으로 다음을 실행합니다:

- ✅ 단위 테스트 (race detector 포함)
- ✅ 통합 테스트
- ✅ 커버리지 분석 (70% 이상 목표)
- ✅ Linting (golangci-lint)
- ✅ 보안 스캔 (gosec)
- ✅ Static analysis (staticcheck)

---

## 3. 빌드 및 실행

### 3.1 로컬에서 개발 서버 실행

```bash
# 기본 실행
make run

# 또는 직접 실행
cd server
go run cmd/api/main.go

# Hot reload를 사용한 개발 (air 설치 필요)
make install-air
make run-dev
```

### 3.2 프로덕션 빌드

```bash
# API 서버 빌드
make build

# 모든 바이너리 빌드 (API, Migrate, Backup)
make build-all

# 빌드된 바이너리 실행
./bin/api
```

### 3.3 헬스 체크

서버가 실행 중인지 확인:

```bash
# Make 명령 사용
make health

# 또는 curl 직접 사용
curl http://localhost:8080/health

# 예상 응답:
# {"status":"ok","timestamp":"2025-01-19T12:00:00Z"}
```

---

## 4. Docker 배포

### 4.1 Docker Compose로 전체 스택 실행

가장 간단한 배포 방법입니다. PostgreSQL, Redis, API 서버를 한 번에 실행합니다.

```bash
# .env 파일 설정 확인
cp .env.example .env
# .env 파일 편집 후

# 모든 서비스 시작
docker-compose up -d

# 로그 확인
docker-compose logs -f api

# 서비스 상태 확인
docker-compose ps

# 서비스 중지
docker-compose down

# 볼륨까지 모두 삭제
docker-compose down -v
```

### 4.2 개별 Docker 이미지 빌드

```bash
# Docker 이미지 빌드
docker build -t donelist-api:latest -f Dockerfile .

# 이미지 실행 (데이터베이스가 이미 실행 중이어야 함)
docker run -p 8080:8080 --env-file .env donelist-api:latest

# 또는 환경 변수를 직접 전달
docker run -p 8080:8080 \
  -e DB_HOST=host.docker.internal \
  -e DB_PASSWORD=your_password \
  -e REDIS_HOST=host.docker.internal \
  donelist-api:latest
```

### 4.3 마이그레이션만 실행

```bash
# Docker Compose 프로파일을 사용한 마이그레이션
docker-compose --profile migrate up migrate

# 또는 별도로 실행
docker run --rm \
  -e DB_HOST=postgres \
  -e DB_PASSWORD=your_password \
  --network donelist-network \
  donelist-api:latest /app/donelist-api --migrate-only
```

---

## 5. K3s/Kubernetes 배포

### 5.1 K3s 설치 (경량 Kubernetes)

```bash
# K3s 설치 (단일 노드)
curl -sfL https://get.k3s.io | sh -

# kubectl 설정
sudo chmod 644 /etc/rancher/k3s/k3s.yaml
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# 설치 확인
kubectl get nodes
```

### 5.2 Kubernetes 매니페스트 생성

`k8s/` 디렉토리를 생성하고 다음 파일들을 추가합니다:

#### 5.2.1 네임스페이스 생성

**`k8s/namespace.yaml`**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: donelist
```

#### 5.2.2 ConfigMap 및 Secret

**`k8s/configmap.yaml`**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: donelist-config
  namespace: donelist
data:
  SERVER_ENV: "production"
  SERVER_PORT: "8080"
  DB_HOST: "postgres-service"
  DB_PORT: "5432"
  DB_NAME: "donelist"
  REDIS_HOST: "redis-service"
  REDIS_PORT: "6379"
  LOG_LEVEL: "info"
  LOG_FORMAT: "json"
```

**`k8s/secret.yaml`**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: donelist-secret
  namespace: donelist
type: Opaque
stringData:
  DB_PASSWORD: "your_db_password_here"
  REDIS_PASSWORD: "your_redis_password_here"
  JWT_SECRET: "your-super-secret-jwt-key-minimum-32-characters"
  STRIPE_SECRET_KEY: "sk_live_your_production_key_here"
  STRIPE_WEBHOOK_SECRET: "whsec_your_webhook_secret_here"
```

#### 5.2.3 PostgreSQL 배포

**`k8s/postgres.yaml`**

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: donelist
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: donelist
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15-alpine
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: donelist
        - name: POSTGRES_USER
          value: donelist
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: donelist-secret
              key: DB_PASSWORD
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        livenessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - donelist
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - donelist
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: donelist
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
  type: ClusterIP
```

#### 5.2.4 Redis 배포

**`k8s/redis.yaml`**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: donelist
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        command:
        - redis-server
        - --requirepass
        - $(REDIS_PASSWORD)
        env:
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: donelist-secret
              key: REDIS_PASSWORD
        livenessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: donelist
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
  type: ClusterIP
```

#### 5.2.5 API 서버 배포

**`k8s/api.yaml`**

```yaml
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
        image: donelist-api:latest  # 실제 이미지 레지스트리 경로로 변경
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
        envFrom:
        - configMapRef:
            name: donelist-config
        - secretRef:
            name: donelist-secret
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: donelist-api-service
  namespace: donelist
spec:
  selector:
    app: donelist-api
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer  # 또는 NodePort, ClusterIP
```

#### 5.2.6 Ingress 설정 (선택사항)

**`k8s/ingress.yaml`**

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: donelist-ingress
  namespace: donelist
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
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
```

### 5.3 K3s에 배포

```bash
# 네임스페이스 생성
kubectl apply -f k8s/namespace.yaml

# Secret 및 ConfigMap 적용
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/configmap.yaml

# 데이터베이스 및 캐시 배포
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml

# API 서버 배포
kubectl apply -f k8s/api.yaml

# Ingress 설정 (선택사항)
kubectl apply -f k8s/ingress.yaml

# 배포 상태 확인
kubectl get all -n donelist

# Pod 로그 확인
kubectl logs -f deployment/donelist-api -n donelist

# 서비스 접속 테스트
kubectl port-forward svc/donelist-api-service 8080:80 -n donelist
curl http://localhost:8080/health
```

### 5.4 마이그레이션 Job

**`k8s/migration-job.yaml`**

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: donelist-migration
  namespace: donelist
spec:
  template:
    spec:
      containers:
      - name: migrate
        image: donelist-api:latest
        command: ["/app/donelist-api", "--migrate-only"]
        envFrom:
        - configMapRef:
            name: donelist-config
        - secretRef:
            name: donelist-secret
      restartPolicy: OnFailure
  backoffLimit: 3
```

```bash
# 마이그레이션 실행
kubectl apply -f k8s/migration-job.yaml

# Job 상태 확인
kubectl get jobs -n donelist
kubectl logs job/donelist-migration -n donelist
```

---

## 6. 프로덕션 배포

### 6.1 프로덕션 체크리스트

배포 전 반드시 확인해야 할 사항:

#### 보안

- [ ] `.env` 파일에서 모든 기본 비밀번호 변경
- [ ] JWT_SECRET을 강력한 랜덤 문자열로 설정 (최소 32자)
- [ ] 데이터베이스 비밀번호를 강력하게 설정
- [ ] Redis 비밀번호 설정
- [ ] CORS 설정을 프로덕션 도메인으로 제한
- [ ] Rate limiting 활성화 확인
- [ ] SSL/TLS 인증서 설정
- [ ] Stripe 프로덕션 키 사용 (sk_live_...)

#### 성능

- [ ] 데이터베이스 연결 풀 크기 조정
- [ ] Redis 메모리 제한 설정
- [ ] 로그 레벨을 `warn` 또는 `error`로 설정
- [ ] 프로파일링 비활성화

#### 백업

- [ ] 데이터베이스 자동 백업 설정
- [ ] WAL 아카이빙 활성화 (PITR용)
- [ ] S3 백업 스토리지 설정
- [ ] 백업 복원 테스트 실행

#### 모니터링

- [ ] Prometheus 메트릭 엔드포인트 활성화
- [ ] Grafana 대시보드 설정
- [ ] 알림 규칙 설정
- [ ] 헬스체크 엔드포인트 확인

### 6.2 환경 변수 (프로덕션)

**프로덕션 `.env` 예시:**

```bash
# Server Configuration
SERVER_ENV=production
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

# Database Configuration
DB_HOST=prod-postgres.example.com
DB_PORT=5432
DB_NAME=donelist_prod
DB_USER=donelist_prod
DB_PASSWORD=STRONG_RANDOM_PASSWORD_HERE
DB_SSLMODE=require
DB_MAX_CONNECTIONS=100
DB_MAX_IDLE_CONNECTIONS=10
DB_CONNECTION_MAX_LIFETIME=1h

# Redis Configuration
REDIS_HOST=prod-redis.example.com
REDIS_PORT=6379
REDIS_PASSWORD=STRONG_REDIS_PASSWORD_HERE
REDIS_DB=0

# JWT Configuration
JWT_SECRET=STRONG_RANDOM_SECRET_MINIMUM_32_CHARACTERS_HERE
JWT_ACCESS_TOKEN_EXPIRY=15m
JWT_REFRESH_TOKEN_EXPIRY=7d

# Security
CORS_ALLOWED_ORIGINS=https://donelist.com,https://app.donelist.com
RATE_LIMIT_ENABLED=true
RATE_LIMIT_MAX_REQUESTS=100
RATE_LIMIT_WINDOW=1m

# Logging
LOG_LEVEL=warn
LOG_FORMAT=json
LOG_OUTPUT=stdout

# Stripe (Production)
STRIPE_SECRET_KEY=sk_live_YOUR_LIVE_KEY_HERE
STRIPE_WEBHOOK_SECRET=whsec_YOUR_WEBHOOK_SECRET_HERE

# Backup Configuration
BACKUP_ENABLED=true
BACKUP_SCHEDULE=0 2 * * *
WAL_ARCHIVING_ENABLED=true
S3_BUCKET=donelist-backups-prod
S3_REGION=us-east-1
```

### 6.3 도메인 및 SSL 설정

#### Nginx Reverse Proxy 설정

```nginx
# /etc/nginx/sites-available/donelist
server {
    listen 80;
    server_name api.donelist.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.donelist.com;

    # SSL 인증서 (Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/api.donelist.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.donelist.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Proxy settings
    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # WebSocket support
    location /ws {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }
}
```

#### Let's Encrypt SSL 인증서 발급

```bash
# Certbot 설치
sudo apt install certbot python3-certbot-nginx

# SSL 인증서 발급
sudo certbot --nginx -d api.donelist.com

# 자동 갱신 테스트
sudo certbot renew --dry-run
```

### 6.4 Systemd 서비스 설정 (베어메탈 배포)

**`/etc/systemd/system/donelist.service`**

```ini
[Unit]
Description=DoneList API Server
After=network.target postgresql.service redis.service
Wants=postgresql.service redis.service

[Service]
Type=simple
User=donelist
Group=donelist
WorkingDirectory=/opt/donelist
EnvironmentFile=/opt/donelist/.env
ExecStart=/opt/donelist/bin/api
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=donelist

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/donelist/logs

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

```bash
# 서비스 등록 및 시작
sudo systemctl daemon-reload
sudo systemctl enable donelist
sudo systemctl start donelist

# 상태 확인
sudo systemctl status donelist

# 로그 확인
sudo journalctl -u donelist -f
```

---

## 7. 모니터링 및 유지보수

### 7.1 Prometheus 메트릭 수집

API 서버는 `/metrics` 엔드포인트를 통해 Prometheus 메트릭을 제공합니다.

**Prometheus 설정 (`prometheus.yml`)**

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'donelist-api'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

### 7.2 Grafana 대시보드

Prometheus 데이터 소스 추가 후, 다음 메트릭을 모니터링하세요:

- HTTP 요청 수 및 응답 시간
- 데이터베이스 연결 풀 상태
- Redis 캐시 히트율
- 에러율 및 5xx 응답
- 메모리 및 CPU 사용률

### 7.3 로그 관리

#### 구조화된 로그 (JSON)

프로덕션 환경에서는 JSON 형식의 로그를 사용하여 분석을 용이하게 합니다.

```bash
# 로그 출력 예시
{"level":"info","time":"2025-01-19T12:00:00Z","msg":"Server started","port":8080}
{"level":"error","time":"2025-01-19T12:01:00Z","msg":"Database connection failed","error":"timeout"}
```

#### ELK 스택 연동

로그를 Elasticsearch로 전송하여 Kibana에서 시각화할 수 있습니다.

### 7.4 백업 및 복구

#### 자동 백업 설정

```bash
# 백업 스크립트 실행 권한 부여
chmod +x scripts/backup/*.sh

# 백업 디렉토리 생성
sudo mkdir -p /var/backups/donelist/wal
sudo chown -R donelist:donelist /var/backups/donelist

# Cron job 설정 (매일 새벽 2시)
crontab -e
# 다음 줄 추가:
0 2 * * * /opt/donelist/scripts/backup/backup.sh
```

#### PITR 설정

```bash
# PITR 설정 스크립트 실행
sudo ./scripts/backup/setup-pitr.sh

# 복구 테스트
./scripts/backup/recovery-rehearsal.sh
```

#### 백업 복원

```bash
# 특정 백업에서 복원
make backup-restore BACKUP_FILE=/var/backups/donelist/donelist_20250119_020000.sql.gz

# PITR을 사용한 특정 시점 복구
./scripts/backup/pitr-restore.sh \
  --timestamp "2025-01-19 12:00:00" \
  --target-dir /var/lib/postgresql/15/main
```

---

## 8. 트러블슈팅

### 8.1 일반적인 문제

#### 데이터베이스 연결 실패

```bash
# PostgreSQL 서비스 상태 확인
sudo systemctl status postgresql

# 연결 테스트
psql -h localhost -U donelist -d donelist -c "SELECT 1;"

# 포트 확인
sudo netstat -tuln | grep 5432

# 방화벽 확인
sudo ufw status
```

#### Redis 연결 실패

```bash
# Redis 서비스 상태 확인
sudo systemctl status redis

# 연결 테스트
redis-cli -h localhost -p 6379 -a your_password ping

# 메모리 사용량 확인
redis-cli INFO memory
```

#### 마이그레이션 실패

```bash
# 마이그레이션 상태 확인
psql -U donelist -d donelist -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 5;"

# 마이그레이션 로그 확인
make migrate-up 2>&1 | tee migration.log

# 마이그레이션 강제 버전 설정 (주의!)
# 먼저 백업 후 실행
```

### 8.2 성능 문제

#### 슬로우 쿼리 디버깅

```sql
-- PostgreSQL 슬로우 쿼리 로그 활성화
ALTER SYSTEM SET log_min_duration_statement = 1000; -- 1초 이상 쿼리 로깅
SELECT pg_reload_conf();

-- 슬로우 쿼리 확인
SELECT query, calls, total_time, mean_time
FROM pg_stat_statements
ORDER BY total_time DESC
LIMIT 10;
```

#### 메모리 누수 디버깅

```bash
# pprof를 사용한 메모리 프로파일링
go tool pprof http://localhost:8080/debug/pprof/heap

# CPU 프로파일링
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
```

### 8.3 로그 및 디버깅

```bash
# Docker 로그 확인
docker-compose logs -f api

# Kubernetes 로그 확인
kubectl logs -f deployment/donelist-api -n donelist

# Systemd 로그 확인
sudo journalctl -u donelist -f

# 특정 시간대 로그
sudo journalctl -u donelist --since "2025-01-19 12:00" --until "2025-01-19 13:00"
```

---

## 📞 지원 및 문의

문제가 발생하거나 질문이 있는 경우:

- **이슈 트래커**: https://github.com/dev-jelly/donelist/issues
- **문서**: `docs/` 디렉토리 참조
- **백업 가이드**: `server/docs/PITR_QUICK_GUIDE.md`
- **파티셔닝 가이드**: `server/docs/PARTITIONING_GUIDE.md`

---

## 📝 라이선스

이 프로젝트는 MIT 라이선스를 따릅니다.

---

**마지막 업데이트**: 2025-01-19

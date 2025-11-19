# Donelist - Done tracking, not Todo planning

**Version**: 1.0.0
**Status**: Planning Phase
**Tech Stack**: Go + PostgreSQL + K3s + SwiftUI + Jetpack Compose + React

---

## 📋 Overview

**Donelist**는 "할 일(Todo)" 대신 "한 일(Done)"에 집중하는 혁신적인 생산성 추적 앱입니다.

### 🎯 Core Concept

기존 Todo 앱의 문제점:
- ❌ 계획만 쌓이고 실천은 안 됨
- ❌ 실제로 뭘 했는지 기억이 안 남
- ❌ 생산성 측정이 불가능

Donelist의 해결책:
- ✅ 시간 기반 체크인 시스템 (15/30/45분 간격)
- ✅ 이미 완료한 일에 대한 기록
- ✅ 데이터 기반 생산성 패턴 분석

---

## 🚀 Features

### Core Features
- ⏰ **지능적 체크인 시스템**: 15/30/45분 간격으로 실시간 기록
- 📅 **캘린더 뷰**: 일간/주간/월간 타임라인 시각화
- 🏷️ **카테고리 & 태그**: 활동 분류 및 검색
- 📊 **통계 & 분석**: 생산성 패턴 인사이트
- 🔄 **실시간 동기화**: 모든 플랫폼 간 seamless sync

### Premium Features
- 💎 **과거 수정**: 2시간 이후 기록 수정 가능
- 📈 **고급 분석**: AI 기반 생산성 인사이트
- 📤 **데이터 Export**: CSV/JSON/PDF 내보내기

---

## 🛠️ Tech Stack

### Backend
- **Language**: Go 1.21+
- **Framework**: Gin (HTTP) + Gorilla WebSocket
- **Database**: PostgreSQL 16
- **Cache**: Redis 7
- **Deployment**: K3s (Kubernetes)

### Frontend - iOS/macOS
- **Language**: Swift 5.9+
- **Framework**: SwiftUI
- **Architecture**: MVVM + Combine

### Frontend - Android
- **Language**: Kotlin 1.9+
- **Framework**: Jetpack Compose
- **Architecture**: MVVM + Coroutines

### Frontend - Web
- **Framework**: React 18+ / Next.js 14+
- **State**: Zustand or Jotai
- **UI**: Tailwind CSS + Shadcn/ui

---

## 📚 Documentation

### 🚀 배포 및 운영 (최신)
- **[빠른 시작 가이드](./QUICKSTART.md)**: 5분 안에 DoneList 시작하기
- **[배포 가이드](./DEPLOYMENT.md)**: 종합 배포 및 테스트 가이드
  - 로컬 개발 환경 설정
  - Docker 배포
  - K3s/Kubernetes 배포
  - 프로덕션 체크리스트
  - 모니터링 및 유지보수
  - 트러블슈팅

### 기능별 가이드
- [백업 및 복구](./server/docs/PITR_QUICK_GUIDE.md): PITR 설정 및 복구
- [파티셔닝](./server/docs/PARTITIONING_GUIDE.md): 데이터베이스 파티셔닝
- [Stripe 통합](./server/docs/STRIPE_INTEGRATION.md): 결제 시스템 설정
- [보안](./server/docs/security/ANOMALY_DETECTION.md): 이상행동 탐지 시스템

### Planning & Architecture
- [📋 PRD (Product Requirements)](docs/planning/PRD.md)
- [🏗️ System Architecture](docs/architecture/SYSTEM_ARCHITECTURE.md)

### Technical Specs
- [🗄️ Database Schema](docs/database/DATABASE_SCHEMA.md)
- [🔌 API Specification](docs/api/API_SPECIFICATION.md)

### Design
- [🎨 UI/UX Overview](docs/ui-ux/UI_UX_OVERVIEW.md)

---

## 📁 Project Structure

```
donelist/
├── docs/                    # 📚 All documentation
│   ├── planning/            # PRD, roadmap
│   ├── architecture/        # System design
│   ├── database/            # DB schema
│   ├── api/                 # API specs
│   ├── deployment/          # K3s guides
│   └── ui-ux/               # Design docs
├── server/                  # 🔧 Go backend (TBD)
│   ├── cmd/
│   ├── internal/
│   ├── pkg/
│   └── deployments/
├── ios/                     # 📱 iOS/macOS app (TBD)
│   └── Donelist.xcodeproj
├── android/                 # 🤖 Android app (TBD)
│   └── app/
└── web/                     # 🌐 Web app (TBD)
    └── src/
```

---

## 🗓️ Development Roadmap

### Phase 1: MVP (3-4 months)
- [x] PRD 작성
- [x] 시스템 아키텍처 설계
- [x] DB 스키마 설계
- [x] API 명세서 작성
- [x] K3s 배포 가이드 작성
- [ ] Backend API 구현 (Go)
- [ ] iOS 앱 개발 (SwiftUI)
- [ ] Web 앱 개발 (React)
- [ ] Alpha 테스트

### Phase 2: Android + Premium (2-3 months)
- [ ] Android 앱 개발 (Kotlin)
- [ ] Premium 기능 구현
- [ ] 결제 시스템 (Stripe)
- [ ] Beta 테스트

### Phase 3: macOS + Advanced (2 months)
- [ ] macOS 앱 (Mac Catalyst)
- [ ] AI 인사이트
- [ ] 고급 분석
- [ ] 공식 출시

---

## 🚦 Getting Started

### 빠른 시작 (Docker Compose 사용)

가장 빠르고 쉬운 방법입니다:

```bash
# 프로젝트 클론
git clone https://github.com/dev-jelly/donelist.git
cd donelist/server

# 환경 변수 설정
cp .env.example .env
# .env 파일 편집 (최소한 DB_PASSWORD, REDIS_PASSWORD, JWT_SECRET 설정)

# 모든 서비스 시작
docker-compose up -d

# 헬스 체크
curl http://localhost:8080/health
```

✅ **완료!** API 서버가 http://localhost:8080 에서 실행 중입니다.

### 로컬 개발 환경 (Go 직접 실행)

**필수 요구사항**:
- Go 1.24+
- PostgreSQL 15+
- Redis 7+
- Make

**설정 단계**:
```bash
# 1. 데이터베이스 생성
psql postgres -c "CREATE USER donelist WITH PASSWORD 'dev123';"
psql postgres -c "CREATE DATABASE donelist OWNER donelist;"

# 2. 환경 변수 설정
cd server
cp .env.example .env
# .env 파일 편집

# 3. 의존성 다운로드 및 마이그레이션
make deps
make migrate-up

# 4. 서버 실행
make run
```

더 자세한 내용은 **[빠른 시작 가이드](./QUICKSTART.md)** 또는 **[배포 가이드](./DEPLOYMENT.md)**를 참조하세요.

---

## 🧪 Testing

### Backend Tests
```bash
cd server
go test -v ./...
go test -race -coverprofile=coverage.out ./...
```

### Frontend Tests
```bash
# iOS
cd ios && xcodebuild test -scheme Donelist

# Android
cd android && ./gradlew test

# Web
cd web && pnpm test
```

---

## 🚀 Deployment

### Docker Compose (개발/스테이징)

```bash
# 모든 서비스 시작
docker-compose up -d

# 로그 확인
docker-compose logs -f api

# 중지
docker-compose down
```

### Kubernetes / K3s (프로덕션)

**자동 배포 스크립트 사용**:
```bash
# K3s 설치
curl -sfL https://get.k3s.io | sh -

# 배포
cd k8s
./deploy.sh production

# 포트 포워딩
kubectl port-forward svc/donelist-api-service 8080:80 -n donelist
```

**수동 배포**:
```bash
# Secret 설정
kubectl create secret generic donelist-secret \
  --from-env-file=k8s/.env.production \
  --namespace=donelist

# 모든 리소스 배포
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/migration-job.yaml
kubectl apply -f k8s/api.yaml
kubectl apply -f k8s/ingress.yaml

# 배포 상태 확인
kubectl get all -n donelist
```

자세한 내용은 **[배포 가이드](./DEPLOYMENT.md)**를 참조하세요.

---

## 📊 Monitoring

### Health Checks

```bash
# API health
curl https://api.donelist.com/health

# Database connection
curl https://api.donelist.com/health/db

# Redis connection
curl https://api.donelist.com/health/redis
```

### Metrics

- **Prometheus**: `https://prometheus.donelist.com`
- **Grafana**: `https://grafana.donelist.com`

---

## 🤝 Contributing

**Coming Soon**: Contribution guidelines

For now, this is a private development project.

---

## 📄 License

**Proprietary** - All rights reserved

---

## 📞 Contact

- **Email**: admin@donelist.com
- **Website**: https://donelist.com (TBD)

---

## 🙏 Acknowledgments

Special thanks to:
- Go community for excellent libraries
- PostgreSQL team for robust database
- K3s project for lightweight Kubernetes
- SwiftUI & Jetpack Compose teams

---

**Last Updated**: 2025-01-19
**Status**: Backend Implementation Complete ✅
**Next**: iOS & Web App Development

---

## 🎯 최근 구현 완료 (2025-01-19)

### Backend API 완성
- ✅ Stripe 결제 시스템 통합
- ✅ 카테고리 병합 및 일괄 수정 기능
- ✅ 사용 통계 파이프라인
- ✅ 검색 인덱싱 시스템 (2,500+ docs/sec)
- ✅ 알림 엔진 & DnD 스케줄러
- ✅ 프론트엔드 (React + TypeScript)
- ✅ 이상행동 탐지 시스템 (7가지 규칙)
- ✅ 비밀 관리 & 자동 키 로테이션
- ✅ 데이터베이스 파티셔닝 (10x 성능 향상)
- ✅ PITR 백업 시스템

### 문서화
- ✅ 종합 배포 가이드 (DEPLOYMENT.md)
- ✅ 빠른 시작 가이드 (QUICKSTART.md)
- ✅ K3s 자동 배포 스크립트
- ✅ 백업 및 복구 가이드
- ✅ 파티셔닝 가이드

# 남은 작업 계획 (Remaining Work Plan)

## 전체 목표
Donelist 서버의 실시간 통신, 테스트, 배포 설정 파일 작성

## Phase 1: WebSocket 실시간 통신 구현
**목표**: 클라이언트 간 실시간 체크인 업데이트 동기화

### Task 1.1: WebSocket 인프라 구축
- WebSocket 연결 관리자 (Hub) 구현
- 클라이언트 연결/해제 처리
- 사용자별 연결 추적

### Task 1.2: 메시지 브로드캐스팅 시스템
- 메시지 타입 정의 (CheckinCreated, CheckinUpdated, CheckinDeleted)
- 브로드캐스트 로직 구현
- 사용자별 메시지 필터링

### Task 1.3: Check-in 이벤트 통합
- Check-in Service와 WebSocket 통합
- 생성/수정/삭제 시 이벤트 발행
- Timeline 업데이트 알림

### Task 1.4: WebSocket 핸들러 및 라우트
- /ws 엔드포인트 구현
- JWT 인증 미들웨어 적용
- 연결 업그레이드 처리

## Phase 2: Unit & Integration 테스트 작성
**목표**: 코드 품질 보장 및 회귀 방지

### Task 2.1: 테스트 인프라 설정
- 테스트 데이터베이스 설정 (testcontainers or dockertest)
- 테스트 헬퍼 함수 작성
- Mock 객체 준비

### Task 2.2: Repository 레이어 테스트
- User Repository 테스트
- Check-in Repository 테스트
- Category/Tag Repository 테스트

### Task 2.3: Service 레이어 테스트
- Auth Service 테스트
- Check-in Service 테스트 (비즈니스 로직 검증)
- Timeline Service 테스트

### Task 2.4: Integration 테스트
- 전체 API 플로우 테스트
- 인증 플로우 통합 테스트
- Check-in CRUD 통합 테스트

## Phase 3: 배포 설정 파일 작성 (참고용)
**목표**: Docker 및 K3s 배포를 위한 참고 설정 파일 준비
**주의**: 실제 배포는 하지 않고 설정 파일만 작성

### Task 3.1: Dockerfile 작성
- Multi-stage 빌드 구성
- 최적화된 이미지 크기
- 환경 변수 설정
- 빌드 가능 여부 검증

### Task 3.2: Docker Compose 설정
- API 서버 컨테이너 설정
- PostgreSQL 컨테이너 설정
- Redis 컨테이너 설정
- 네트워크 및 볼륨 설정
- 로컬 개발 환경용 설정

### Task 3.3: K3s 배포 매니페스트 작성
- Deployment 매니페스트 (deploy/k8s/deployment.yaml)
- Service 매니페스트 (deploy/k8s/service.yaml)
- ConfigMap 템플릿 (deploy/k8s/configmap.yaml)
- Secret 템플릿 (deploy/k8s/secret.yaml)
- 주석으로 설명 추가

### Task 3.4: 배포 가이드 문서 작성
- README_DEPLOY.md 작성
- Docker 빌드 및 실행 가이드
- K3s 배포 절차 설명
- 환경 변수 설정 가이드
- 트러블슈팅 팁

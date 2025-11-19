#!/bin/bash
set -e

# DoneList K3s 자동 배포 스크립트
# Usage: ./deploy.sh [환경]
# 환경: development, staging, production (기본값: development)

ENVIRONMENT=${1:-development}
NAMESPACE="donelist"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🚀 DoneList K3s 배포 시작 - 환경: $ENVIRONMENT"

# 색상 정의
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 함수 정의
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_command() {
    if ! command -v $1 &> /dev/null; then
        log_error "$1이(가) 설치되어 있지 않습니다."
        exit 1
    fi
}

# 1. 사전 확인
log_info "1. 사전 요구사항 확인 중..."
check_command kubectl
check_command docker

# kubectl 연결 확인
if ! kubectl cluster-info &> /dev/null; then
    log_error "kubectl이 클러스터에 연결할 수 없습니다."
    log_error "K3s가 설치되어 있고 kubectl이 올바르게 설정되어 있는지 확인하세요."
    exit 1
fi

log_info "✓ 사전 요구사항 확인 완료"

# 2. 네임스페이스 생성
log_info "2. 네임스페이스 생성 중..."
kubectl apply -f "${SCRIPT_DIR}/namespace.yaml"

# 3. Secret 설정 확인
log_info "3. Secret 설정 확인 중..."
if [ ! -f "${SCRIPT_DIR}/.env.${ENVIRONMENT}" ]; then
    log_warn ".env.${ENVIRONMENT} 파일이 없습니다."
    log_warn "기본 템플릿을 생성합니다. 반드시 실제 값으로 수정하세요!"

    cat > "${SCRIPT_DIR}/.env.${ENVIRONMENT}" << 'EOF'
DB_PASSWORD=CHANGE_ME
REDIS_PASSWORD=CHANGE_ME
JWT_SECRET=CHANGE_ME_MINIMUM_32_CHARACTERS
STRIPE_SECRET_KEY=sk_test_CHANGE_ME
STRIPE_WEBHOOK_SECRET=whsec_CHANGE_ME
EOF

    log_error "Secret 파일을 수정한 후 다시 실행하세요: ${SCRIPT_DIR}/.env.${ENVIRONMENT}"
    exit 1
fi

# Secret 생성
log_info "Secret 생성 중..."
kubectl create secret generic donelist-secret \
    --from-env-file="${SCRIPT_DIR}/.env.${ENVIRONMENT}" \
    --namespace="${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -

log_info "✓ Secret 설정 완료"

# 4. ConfigMap 적용
log_info "4. ConfigMap 적용 중..."
kubectl apply -f "${SCRIPT_DIR}/configmap.yaml"

# 5. 스토리지 및 데이터베이스 배포
log_info "5. PostgreSQL 배포 중..."
kubectl apply -f "${SCRIPT_DIR}/postgres.yaml"

log_info "PostgreSQL이 준비될 때까지 대기 중..."
kubectl wait --for=condition=ready pod \
    --selector=app=postgres \
    --namespace="${NAMESPACE}" \
    --timeout=180s

# 6. Redis 배포
log_info "6. Redis 배포 중..."
kubectl apply -f "${SCRIPT_DIR}/redis.yaml"

log_info "Redis가 준비될 때까지 대기 중..."
kubectl wait --for=condition=ready pod \
    --selector=app=redis \
    --namespace="${NAMESPACE}" \
    --timeout=180s

# 7. 마이그레이션 실행
log_info "7. 데이터베이스 마이그레이션 실행 중..."
kubectl apply -f "${SCRIPT_DIR}/migration-job.yaml"

# Job 완료 대기
log_info "마이그레이션 완료 대기 중..."
kubectl wait --for=condition=complete job/donelist-migration \
    --namespace="${NAMESPACE}" \
    --timeout=300s || {
    log_error "마이그레이션 실패. 로그 확인:"
    kubectl logs job/donelist-migration -n "${NAMESPACE}"
    exit 1
}

log_info "✓ 마이그레이션 완료"

# 8. API 서버 배포
log_info "8. API 서버 배포 중..."
kubectl apply -f "${SCRIPT_DIR}/api.yaml"

log_info "API 서버가 준비될 때까지 대기 중..."
kubectl wait --for=condition=ready pod \
    --selector=app=donelist-api \
    --namespace="${NAMESPACE}" \
    --timeout=300s

# 9. Ingress 배포 (선택사항)
if [ -f "${SCRIPT_DIR}/ingress.yaml" ]; then
    log_info "9. Ingress 설정 중..."
    kubectl apply -f "${SCRIPT_DIR}/ingress.yaml"
fi

# 10. 배포 상태 확인
log_info "10. 배포 상태 확인 중..."
echo ""
log_info "=== 배포된 리소스 ==="
kubectl get all -n "${NAMESPACE}"

echo ""
log_info "=== Pod 상태 ==="
kubectl get pods -n "${NAMESPACE}"

echo ""
log_info "=== Service 엔드포인트 ==="
kubectl get svc -n "${NAMESPACE}"

# 11. 헬스 체크
log_info "11. 헬스 체크 수행 중..."
SERVICE_IP=$(kubectl get svc donelist-api-service -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")

if [ -z "$SERVICE_IP" ]; then
    # LoadBalancer IP가 없으면 포트 포워딩 사용
    log_info "LoadBalancer IP를 찾을 수 없습니다. 포트 포워딩을 사용하세요:"
    log_info "kubectl port-forward svc/donelist-api-service 8080:80 -n ${NAMESPACE}"
else
    log_info "API 서버: http://$SERVICE_IP"

    # 헬스 체크
    sleep 5
    if curl -f "http://$SERVICE_IP/health" &> /dev/null; then
        log_info "✓ 헬스 체크 성공!"
    else
        log_warn "헬스 체크 실패. Pod 로그를 확인하세요:"
        log_warn "kubectl logs -f deployment/donelist-api -n ${NAMESPACE}"
    fi
fi

# 완료
echo ""
echo "================================================================"
log_info "🎉 배포 완료!"
echo "================================================================"
echo ""
log_info "다음 명령어로 로그를 확인할 수 있습니다:"
echo "  kubectl logs -f deployment/donelist-api -n ${NAMESPACE}"
echo ""
log_info "포트 포워딩:"
echo "  kubectl port-forward svc/donelist-api-service 8080:80 -n ${NAMESPACE}"
echo ""
log_info "배포 제거:"
echo "  kubectl delete namespace ${NAMESPACE}"
echo ""

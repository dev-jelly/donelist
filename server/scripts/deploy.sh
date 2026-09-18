#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-donelist}"
ENVIRONMENT="${ENVIRONMENT:-staging}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
HELM_RELEASE="${HELM_RELEASE:-donelist-api}"
TIMEOUT="${TIMEOUT:-600s}"

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl."
        exit 1
    fi

    if ! command -v helm &> /dev/null; then
        log_error "helm not found. Please install helm."
        exit 1
    fi

    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster. Check your kubeconfig."
        exit 1
    fi

    log_info "Prerequisites check passed."
}

# Create namespace
create_namespace() {
    log_info "Creating namespace $NAMESPACE if not exists..."
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
}

# Deploy with Helm
deploy() {
    log_info "Deploying $HELM_RELEASE to $ENVIRONMENT environment..."

    local VALUES_FILE="./helm/donelist-api/values-${ENVIRONMENT}.yaml"

    if [ ! -f "$VALUES_FILE" ]; then
        log_warn "Values file $VALUES_FILE not found. Using default values."
        VALUES_FILE="./helm/donelist-api/values.yaml"
    fi

    helm upgrade --install "$HELM_RELEASE" ./helm/donelist-api \
        --namespace "$NAMESPACE" \
        --values "$VALUES_FILE" \
        --set image.tag="$IMAGE_TAG" \
        --wait \
        --timeout="$TIMEOUT" \
        --atomic \
        --cleanup-on-fail \
        --create-namespace

    log_info "Deployment completed successfully."
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."

    # Check rollout status
    kubectl rollout status deployment/"$HELM_RELEASE" -n "$NAMESPACE" --timeout=300s

    # Get pod status
    kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api"

    # Check service
    kubectl get svc -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api"

    log_info "Deployment verification completed."
}

# Run health checks
health_check() {
    log_info "Running health checks..."

    local MAX_RETRIES=30
    local RETRY_INTERVAL=2
    local RETRY_COUNT=0

    while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
        if kubectl exec -n "$NAMESPACE" \
            "$(kubectl get pod -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api" -o jsonpath='{.items[0].metadata.name}')" \
            -- wget --spider -q http://localhost:8080/health; then
            log_info "Health check passed."
            return 0
        fi

        RETRY_COUNT=$((RETRY_COUNT + 1))
        log_warn "Health check failed. Retrying in $RETRY_INTERVAL seconds... ($RETRY_COUNT/$MAX_RETRIES)"
        sleep $RETRY_INTERVAL
    done

    log_error "Health check failed after $MAX_RETRIES attempts."
    return 1
}

# Main deployment flow
main() {
    log_info "Starting deployment process..."
    log_info "Environment: $ENVIRONMENT"
    log_info "Namespace: $NAMESPACE"
    log_info "Image Tag: $IMAGE_TAG"

    check_prerequisites
    create_namespace
    deploy
    verify_deployment
    health_check

    log_info "Deployment process completed successfully!"
}

# Handle script arguments
case "${1:-deploy}" in
    deploy)
        main
        ;;
    verify)
        verify_deployment
        ;;
    health)
        health_check
        ;;
    *)
        echo "Usage: $0 {deploy|verify|health}"
        exit 1
        ;;
esac

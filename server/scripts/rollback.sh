#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-donelist}"
HELM_RELEASE="${HELM_RELEASE:-donelist-api}"
REVISION="${REVISION:-}"

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

# Show deployment history
show_history() {
    log_info "Deployment history for $HELM_RELEASE in $NAMESPACE:"
    helm history "$HELM_RELEASE" -n "$NAMESPACE" --max 10
}

# Perform rollback
rollback() {
    log_warn "Rolling back $HELM_RELEASE in $NAMESPACE..."

    if [ -n "$REVISION" ]; then
        log_info "Rolling back to revision $REVISION"
        helm rollback "$HELM_RELEASE" "$REVISION" -n "$NAMESPACE" --wait --timeout=300s
    else
        log_info "Rolling back to previous revision"
        helm rollback "$HELM_RELEASE" -n "$NAMESPACE" --wait --timeout=300s
    fi

    log_info "Rollback completed."
}

# Verify rollback
verify_rollback() {
    log_info "Verifying rollback..."

    # Check rollout status
    kubectl rollout status deployment/"$HELM_RELEASE" -n "$NAMESPACE" --timeout=300s

    # Get current revision
    CURRENT_REVISION=$(helm list -n "$NAMESPACE" -o json | jq -r ".[] | select(.name==\"$HELM_RELEASE\") | .revision")
    log_info "Current revision: $CURRENT_REVISION"

    # Check pod status
    kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api"

    log_info "Rollback verification completed."
}

# Run health checks
health_check() {
    log_info "Running health checks after rollback..."

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

# Main rollback flow
main() {
    log_warn "========================================="
    log_warn "ROLLBACK OPERATION"
    log_warn "========================================="
    log_info "Namespace: $NAMESPACE"
    log_info "Release: $HELM_RELEASE"

    if [ -n "$REVISION" ]; then
        log_info "Target Revision: $REVISION"
    else
        log_info "Target Revision: Previous"
    fi

    # Show current history
    show_history

    # Confirm rollback
    read -p "Are you sure you want to rollback? (yes/no): " CONFIRM
    if [ "$CONFIRM" != "yes" ]; then
        log_info "Rollback cancelled."
        exit 0
    fi

    rollback
    verify_rollback
    health_check

    log_info "Rollback process completed successfully!"
    show_history
}

# Handle script arguments
case "${1:-rollback}" in
    history)
        show_history
        ;;
    rollback)
        main
        ;;
    verify)
        verify_rollback
        ;;
    *)
        echo "Usage: $0 {rollback|history|verify}"
        exit 1
        ;;
esac

#!/bin/bash
# rollback.sh - Automated rollback script for Kubernetes deployment
# This script provides safe rollback procedures with validation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-donelist}"
DEPLOYMENT_NAME="donelist-api"
TIMEOUT=300

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

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

confirm() {
    read -p "$1 (yes/no): " choice
    case "$choice" in
        yes|YES|y|Y ) return 0;;
        * ) return 1;;
    esac
}

# Check prerequisites
if ! command -v kubectl &> /dev/null; then
    log_error "kubectl is not installed"
    exit 1
fi

# Parse command line arguments
REVISION=""
AUTO_CONFIRM=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --revision)
            REVISION="$2"
            shift 2
            ;;
        --auto-confirm)
            AUTO_CONFIRM=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --revision N       Rollback to specific revision (default: previous)"
            echo "  --auto-confirm     Skip confirmation prompts"
            echo "  --help            Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  NAMESPACE         Kubernetes namespace (default: donelist)"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Check if namespace exists
if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
    log_error "Namespace $NAMESPACE does not exist"
    exit 1
fi

# Check if deployment exists
if ! kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" &> /dev/null; then
    log_error "Deployment $DEPLOYMENT_NAME not found in namespace $NAMESPACE"
    exit 1
fi

log_info "=========================================="
log_info "Kubernetes Deployment Rollback"
log_info "=========================================="
log_info "Namespace: $NAMESPACE"
log_info "Deployment: $DEPLOYMENT_NAME"
log_info ""

# Step 1: Show current deployment status
log_step "1. Current deployment status"
echo ""
kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE"
echo ""

# Step 2: Show rollout history
log_step "2. Rollout history"
echo ""
kubectl rollout history deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE"
echo ""

# Determine target revision
CURRENT_REVISION=$(kubectl rollout history deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" | tail -1 | awk '{print $1}')

if [ -z "$REVISION" ]; then
    # Calculate previous revision
    if [ "$CURRENT_REVISION" -gt 1 ]; then
        REVISION=$((CURRENT_REVISION - 1))
        log_info "No revision specified, will rollback to previous revision: $REVISION"
    else
        log_error "Cannot determine previous revision"
        exit 1
    fi
else
    log_info "Will rollback to specified revision: $REVISION"
fi

# Validate revision exists
if ! kubectl rollout history deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --revision="$REVISION" &> /dev/null; then
    log_error "Revision $REVISION does not exist"
    echo ""
    log_info "Available revisions:"
    kubectl rollout history deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE"
    exit 1
fi

# Show details of target revision
log_step "3. Target revision details"
echo ""
kubectl rollout history deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --revision="$REVISION"
echo ""

# Step 3: Get confirmation
if [ "$AUTO_CONFIRM" = false ]; then
    log_warn "WARNING: This will rollback the deployment to revision $REVISION"
    echo ""
    if ! confirm "Do you want to proceed with the rollback?"; then
        log_info "Rollback cancelled"
        exit 0
    fi
fi

# Step 4: Create backup of current deployment (optional)
log_step "4. Creating backup of current deployment"
BACKUP_FILE="/tmp/donelist-deployment-backup-$(date +%Y%m%d-%H%M%S).yaml"
kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o yaml > "$BACKUP_FILE"
log_info "Current deployment backed up to: $BACKUP_FILE"
echo ""

# Step 5: Perform rollback
log_step "5. Performing rollback"
echo ""

if kubectl rollout undo deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --to-revision="$REVISION"; then
    log_info "Rollback command executed successfully"
else
    log_error "Rollback command failed"
    exit 1
fi
echo ""

# Step 6: Wait for rollback to complete
log_step "6. Waiting for rollback to complete"
echo ""

if kubectl rollout status deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout="${TIMEOUT}s"; then
    log_info "Rollback completed successfully"
else
    log_error "Rollback did not complete within timeout"
    echo ""
    log_info "Current pod status:"
    kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api
    echo ""
    log_info "Recent events:"
    kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' | tail -10
    exit 1
fi
echo ""

# Step 7: Validate rollback
log_step "7. Validating rollback"
echo ""

# Check if all pods are ready
READY_REPLICAS=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}')
DESIRED_REPLICAS=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}')

if [ "$READY_REPLICAS" = "$DESIRED_REPLICAS" ]; then
    log_info "All replicas are ready: $READY_REPLICAS/$DESIRED_REPLICAS ✓"
else
    log_warn "Not all replicas are ready: $READY_REPLICAS/$DESIRED_REPLICAS"
fi

# Check pod health
log_info "Checking pod health..."
PODS=$(kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api -o jsonpath='{.items[*].metadata.name}')
HEALTHY_PODS=0
TOTAL_PODS=0

for POD in $PODS; do
    TOTAL_PODS=$((TOTAL_PODS + 1))
    READY=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
    if [ "$READY" = "True" ]; then
        HEALTHY_PODS=$((HEALTHY_PODS + 1))
        log_info "  Pod $POD is healthy ✓"
    else
        log_warn "  Pod $POD is not healthy"
    fi
done

echo ""

# Step 8: Test health endpoint
log_step "8. Testing application health"
echo ""

log_info "Starting port-forward to test health endpoint..."
kubectl port-forward -n "$NAMESPACE" "svc/donelist-api-service" 8080:80 &> /dev/null &
PF_PID=$!
sleep 3

HEALTH_CHECK_PASSED=false
if curl -f -s http://localhost:8080/health > /dev/null; then
    log_info "Health check passed ✓"
    HEALTH_CHECK_PASSED=true
else
    log_warn "Health check failed"
fi

kill $PF_PID 2> /dev/null || true
echo ""

# Step 9: Check for errors in logs
log_step "9. Checking application logs for errors"
echo ""

if [ "$TOTAL_PODS" -gt 0 ]; then
    FIRST_POD=$(kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api -o jsonpath='{.items[0].metadata.name}')
    ERROR_COUNT=$(kubectl logs "$FIRST_POD" -n "$NAMESPACE" --tail=50 | grep -i error | wc -l || true)

    if [ "$ERROR_COUNT" -gt 5 ]; then
        log_warn "Found $ERROR_COUNT error messages in recent logs"
        log_info "Recent log entries:"
        kubectl logs "$FIRST_POD" -n "$NAMESPACE" --tail=20
    else
        log_info "No significant errors in recent logs ✓"
    fi
fi
echo ""

# Final summary
log_info "=========================================="
log_info "Rollback Summary"
log_info "=========================================="
log_info "Namespace: $NAMESPACE"
log_info "Deployment: $DEPLOYMENT_NAME"
log_info "Target revision: $REVISION"
log_info "Healthy pods: $HEALTHY_PODS/$TOTAL_PODS"
log_info "Health check: $([ "$HEALTH_CHECK_PASSED" = true ] && echo "PASSED" || echo "FAILED")"
log_info "Backup file: $BACKUP_FILE"
log_info ""

# Determine overall success
if [ "$HEALTHY_PODS" -eq "$TOTAL_PODS" ] && [ "$HEALTH_CHECK_PASSED" = true ]; then
    log_info "✓ Rollback completed successfully!"
    log_info ""
    log_info "Next steps:"
    log_info "  1. Monitor application: kubectl logs -f -n $NAMESPACE -l app=donelist,component=api"
    log_info "  2. Check metrics and alerts"
    log_info "  3. Verify application functionality"
    log_info "  4. Document the rollback reason"
    echo ""
    exit 0
else
    log_error "✗ Rollback completed but issues detected"
    log_info ""
    log_info "Troubleshooting steps:"
    log_info "  1. Check pod logs: kubectl logs -n $NAMESPACE <pod-name>"
    log_info "  2. Describe pods: kubectl describe pods -n $NAMESPACE -l app=donelist,component=api"
    log_info "  3. Check events: kubectl get events -n $NAMESPACE --sort-by='.lastTimestamp'"
    log_info "  4. Consider rolling back further: $0 --revision <earlier-revision>"
    log_info "  5. Restore from backup if needed: kubectl apply -f $BACKUP_FILE"
    echo ""
    exit 1
fi

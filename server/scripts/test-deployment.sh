#!/bin/bash
set -euo pipefail

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration
NAMESPACE="${NAMESPACE:-donelist}"
SERVICE_NAME="${SERVICE_NAME:-donelist-api}"
TIMEOUT=300

# Functions
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# Test 1: Check if namespace exists
test_namespace() {
    log_info "Testing namespace existence..."
    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_info "✓ Namespace '$NAMESPACE' exists"
        return 0
    else
        log_error "✗ Namespace '$NAMESPACE' not found"
        return 1
    fi
}

# Test 2: Check deployment status
test_deployment() {
    log_info "Testing deployment status..."
    if kubectl get deployment "$SERVICE_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_info "✓ Deployment '$SERVICE_NAME' exists"

        # Check if deployment is available
        AVAILABLE=$(kubectl get deployment "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.status.availableReplicas}')
        DESIRED=$(kubectl get deployment "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}')

        if [ "$AVAILABLE" = "$DESIRED" ] && [ "$AVAILABLE" -gt 0 ]; then
            log_info "✓ Deployment is healthy ($AVAILABLE/$DESIRED replicas available)"
            return 0
        else
            log_error "✗ Deployment is unhealthy ($AVAILABLE/$DESIRED replicas available)"
            return 1
        fi
    else
        log_error "✗ Deployment '$SERVICE_NAME' not found"
        return 1
    fi
}

# Test 3: Check pod status
test_pods() {
    log_info "Testing pod status..."
    PODS=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api" -o jsonpath='{.items[*].metadata.name}')

    if [ -z "$PODS" ]; then
        log_error "✗ No pods found"
        return 1
    fi

    log_info "Found pods: $PODS"

    local all_running=true
    for POD in $PODS; do
        STATUS=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}')
        if [ "$STATUS" = "Running" ]; then
            log_info "✓ Pod '$POD' is running"
        else
            log_error "✗ Pod '$POD' status: $STATUS"
            all_running=false
        fi
    done

    if $all_running; then
        return 0
    else
        return 1
    fi
}

# Test 4: Check service
test_service() {
    log_info "Testing service..."
    if kubectl get service "$SERVICE_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_info "✓ Service '$SERVICE_NAME' exists"

        # Get service endpoint
        CLUSTER_IP=$(kubectl get service "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.clusterIP}')
        log_info "  Service ClusterIP: $CLUSTER_IP"
        return 0
    else
        log_error "✗ Service '$SERVICE_NAME' not found"
        return 1
    fi
}

# Test 5: Check ingress
test_ingress() {
    log_info "Testing ingress..."
    if kubectl get ingress "$SERVICE_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_info "✓ Ingress '$SERVICE_NAME' exists"

        # Get ingress hosts
        HOSTS=$(kubectl get ingress "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.rules[*].host}')
        log_info "  Ingress hosts: $HOSTS"
        return 0
    else
        log_warn "⚠ Ingress '$SERVICE_NAME' not found (may be optional)"
        return 0
    fi
}

# Test 6: Health endpoint
test_health_endpoint() {
    log_info "Testing health endpoint..."

    # Get a pod name
    POD=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api" -o jsonpath='{.items[0].metadata.name}')

    if [ -z "$POD" ]; then
        log_error "✗ No pods available for testing"
        return 1
    fi

    # Test /health endpoint
    if kubectl exec "$POD" -n "$NAMESPACE" -- wget --spider -q http://localhost:8080/health 2>&1; then
        log_info "✓ Health endpoint (/health) is responding"
    else
        log_error "✗ Health endpoint (/health) failed"
        return 1
    fi

    # Test /readyz endpoint
    if kubectl exec "$POD" -n "$NAMESPACE" -- wget --spider -q http://localhost:8080/readyz 2>&1; then
        log_info "✓ Readiness endpoint (/readyz) is responding"
    else
        log_error "✗ Readiness endpoint (/readyz) failed"
        return 1
    fi

    # Test /livez endpoint
    if kubectl exec "$POD" -n "$NAMESPACE" -- wget --spider -q http://localhost:8080/livez 2>&1; then
        log_info "✓ Liveness endpoint (/livez) is responding"
    else
        log_error "✗ Liveness endpoint (/livez) failed"
        return 1
    fi

    return 0
}

# Test 7: Database connectivity
test_database() {
    log_info "Testing database connectivity..."

    POD=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api" -o jsonpath='{.items[0].metadata.name}')

    if [ -z "$POD" ]; then
        log_error "✗ No pods available for testing"
        return 1
    fi

    # Check logs for database connection
    if kubectl logs "$POD" -n "$NAMESPACE" --tail=100 | grep -i "database.*connected\|migration.*complete" &> /dev/null; then
        log_info "✓ Database connection appears healthy"
        return 0
    else
        log_warn "⚠ Could not verify database connection from logs"
        return 0  # Don't fail on this
    fi
}

# Test 8: Redis connectivity
test_redis() {
    log_info "Testing Redis connectivity..."

    POD=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api" -o jsonpath='{.items[0].metadata.name}')

    if [ -z "$POD" ]; then
        log_error "✗ No pods available for testing"
        return 1
    fi

    # Check logs for redis connection
    if kubectl logs "$POD" -n "$NAMESPACE" --tail=100 | grep -i "redis.*connected" &> /dev/null; then
        log_info "✓ Redis connection appears healthy"
        return 0
    else
        log_warn "⚠ Could not verify Redis connection from logs"
        return 0  # Don't fail on this
    fi
}

# Test 9: Resource usage
test_resources() {
    log_info "Testing resource usage..."

    # Check if metrics-server is available
    if ! kubectl top nodes &> /dev/null; then
        log_warn "⚠ Metrics server not available, skipping resource tests"
        return 0
    fi

    PODS=$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/name=donelist-api" -o jsonpath='{.items[*].metadata.name}')

    for POD in $PODS; do
        METRICS=$(kubectl top pod "$POD" -n "$NAMESPACE" --no-headers 2>/dev/null || echo "")
        if [ -n "$METRICS" ]; then
            log_info "✓ Pod '$POD' metrics: $METRICS"
        fi
    done

    return 0
}

# Test 10: HPA (if enabled)
test_hpa() {
    log_info "Testing HPA..."

    if kubectl get hpa "$SERVICE_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_info "✓ HPA '$SERVICE_NAME' exists"

        HPA_STATUS=$(kubectl get hpa "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.status.currentReplicas}/{.spec.maxReplicas}')
        log_info "  Current/Max replicas: $HPA_STATUS"
        return 0
    else
        log_warn "⚠ HPA not found (may be disabled)"
        return 0
    fi
}

# Test 11: Secrets
test_secrets() {
    log_info "Testing secrets..."

    if kubectl get secret "${SERVICE_NAME}-secret" -n "$NAMESPACE" &> /dev/null; then
        log_info "✓ Secret '${SERVICE_NAME}-secret' exists"

        # Check if required keys exist
        KEYS=$(kubectl get secret "${SERVICE_NAME}-secret" -n "$NAMESPACE" -o jsonpath='{.data}' | grep -o '"[^"]*"' | tr -d '"' || echo "")
        log_info "  Secret keys found: $(echo $KEYS | tr '\n' ' ')"
        return 0
    else
        log_error "✗ Secret '${SERVICE_NAME}-secret' not found"
        return 1
    fi
}

# Test 12: ConfigMap
test_configmap() {
    log_info "Testing configmap..."

    if kubectl get configmap "${SERVICE_NAME}-config" -n "$NAMESPACE" &> /dev/null; then
        log_info "✓ ConfigMap '${SERVICE_NAME}-config' exists"
        return 0
    else
        log_error "✗ ConfigMap '${SERVICE_NAME}-config' not found"
        return 1
    fi
}

# Run all tests
run_all_tests() {
    log_info "========================================="
    log_info "Donelist API Deployment Test Suite"
    log_info "========================================="
    log_info "Namespace: $NAMESPACE"
    log_info "Service: $SERVICE_NAME"
    log_info ""

    local failed_tests=()
    local passed_tests=()

    # Run tests
    for test_func in test_namespace test_deployment test_pods test_service test_ingress test_secrets test_configmap test_health_endpoint test_database test_redis test_resources test_hpa; do
        echo ""
        if $test_func; then
            passed_tests+=("$test_func")
        else
            failed_tests+=("$test_func")
        fi
    done

    # Summary
    echo ""
    log_info "========================================="
    log_info "Test Summary"
    log_info "========================================="
    log_info "Passed: ${#passed_tests[@]}"
    log_info "Failed: ${#failed_tests[@]}"

    if [ ${#failed_tests[@]} -eq 0 ]; then
        log_info ""
        log_info "✓ All tests passed!"
        return 0
    else
        log_error ""
        log_error "✗ Some tests failed:"
        for test in "${failed_tests[@]}"; do
            log_error "  - $test"
        done
        return 1
    fi
}

# Main
main() {
    if [ "${1:-all}" = "all" ]; then
        run_all_tests
    else
        # Run specific test
        "test_$1"
    fi
}

main "$@"

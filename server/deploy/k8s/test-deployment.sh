#!/bin/bash
# test-deployment.sh - Comprehensive deployment testing script
# This script validates the Kubernetes deployment is working correctly

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-donelist}"
DEPLOYMENT_NAME="donelist-api"
SERVICE_NAME="donelist-api-service"
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

check_command() {
    if ! command -v $1 &> /dev/null; then
        log_error "$1 is not installed. Please install it first."
        exit 1
    fi
}

# Check prerequisites
log_info "Checking prerequisites..."
check_command kubectl
check_command curl

# Check if namespace exists
log_info "Checking namespace..."
if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
    log_error "Namespace $NAMESPACE does not exist"
    exit 1
fi
log_info "Namespace $NAMESPACE exists ✓"

# Test 1: Check deployment exists and is ready
log_info "Test 1: Checking deployment status..."
if ! kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" &> /dev/null; then
    log_error "Deployment $DEPLOYMENT_NAME not found"
    exit 1
fi

READY_REPLICAS=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}')
DESIRED_REPLICAS=$(kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}')

if [ "$READY_REPLICAS" != "$DESIRED_REPLICAS" ]; then
    log_error "Deployment not ready. Ready: $READY_REPLICAS, Desired: $DESIRED_REPLICAS"
    exit 1
fi
log_info "Deployment is ready with $READY_REPLICAS/$DESIRED_REPLICAS replicas ✓"

# Test 2: Check pods are running
log_info "Test 2: Checking pod status..."
RUNNING_PODS=$(kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api -o json | jq -r '.items[] | select(.status.phase=="Running") | .metadata.name' | wc -l)

if [ "$RUNNING_PODS" -lt 1 ]; then
    log_error "No running pods found"
    kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api
    exit 1
fi
log_info "$RUNNING_PODS pod(s) are running ✓"

# Test 3: Check pod health
log_info "Test 3: Checking pod health..."
PODS=$(kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api -o jsonpath='{.items[*].metadata.name}')

for POD in $PODS; do
    READY=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
    if [ "$READY" != "True" ]; then
        log_error "Pod $POD is not ready"
        kubectl describe pod "$POD" -n "$NAMESPACE"
        exit 1
    fi
    log_info "Pod $POD is ready ✓"
done

# Test 4: Check service exists
log_info "Test 4: Checking service..."
if ! kubectl get service "$SERVICE_NAME" -n "$NAMESPACE" &> /dev/null; then
    log_error "Service $SERVICE_NAME not found"
    exit 1
fi
log_info "Service $SERVICE_NAME exists ✓"

# Test 5: Check endpoints
log_info "Test 5: Checking service endpoints..."
ENDPOINTS=$(kubectl get endpoints "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.subsets[*].addresses[*].ip}' | wc -w)

if [ "$ENDPOINTS" -lt 1 ]; then
    log_error "No endpoints found for service"
    kubectl describe service "$SERVICE_NAME" -n "$NAMESPACE"
    exit 1
fi
log_info "Service has $ENDPOINTS endpoint(s) ✓"

# Test 6: Check health endpoint via port-forward
log_info "Test 6: Testing health endpoint..."
kubectl port-forward -n "$NAMESPACE" "svc/$SERVICE_NAME" 8080:80 &> /dev/null &
PF_PID=$!
sleep 3

if ! curl -f -s http://localhost:8080/health > /dev/null; then
    log_error "Health check failed"
    kill $PF_PID 2> /dev/null || true
    exit 1
fi
log_info "Health check passed ✓"

# Test 7: Check readiness endpoint
log_info "Test 7: Testing readiness endpoint..."
if ! curl -f -s http://localhost:8080/ready > /dev/null; then
    log_error "Readiness check failed"
    kill $PF_PID 2> /dev/null || true
    exit 1
fi
log_info "Readiness check passed ✓"

# Test 8: Check metrics endpoint
log_info "Test 8: Testing metrics endpoint..."
if ! curl -f -s http://localhost:8080/metrics > /dev/null; then
    log_warn "Metrics endpoint not accessible (this may be expected)"
else
    log_info "Metrics endpoint accessible ✓"
fi

# Clean up port-forward
kill $PF_PID 2> /dev/null || true

# Test 9: Check ConfigMap
log_info "Test 9: Checking ConfigMap..."
if ! kubectl get configmap donelist-config -n "$NAMESPACE" &> /dev/null; then
    log_error "ConfigMap donelist-config not found"
    exit 1
fi
log_info "ConfigMap exists ✓"

# Test 10: Check Secret
log_info "Test 10: Checking Secret..."
if ! kubectl get secret donelist-secret -n "$NAMESPACE" &> /dev/null; then
    log_error "Secret donelist-secret not found"
    exit 1
fi
log_info "Secret exists ✓"

# Test 11: Check HPA
log_info "Test 11: Checking HorizontalPodAutoscaler..."
if kubectl get hpa donelist-api-hpa -n "$NAMESPACE" &> /dev/null; then
    CURRENT_REPLICAS=$(kubectl get hpa donelist-api-hpa -n "$NAMESPACE" -o jsonpath='{.status.currentReplicas}')
    DESIRED_REPLICAS=$(kubectl get hpa donelist-api-hpa -n "$NAMESPACE" -o jsonpath='{.status.desiredReplicas}')
    log_info "HPA is active. Current: $CURRENT_REPLICAS, Desired: $DESIRED_REPLICAS ✓"
else
    log_warn "HPA not found (this may be expected in some environments)"
fi

# Test 12: Check NetworkPolicies
log_info "Test 12: Checking NetworkPolicies..."
NETPOL_COUNT=$(kubectl get networkpolicy -n "$NAMESPACE" | grep -c donelist || true)
if [ "$NETPOL_COUNT" -gt 0 ]; then
    log_info "NetworkPolicies are configured ($NETPOL_COUNT found) ✓"
else
    log_warn "No NetworkPolicies found (consider adding for security)"
fi

# Test 13: Check PVCs
log_info "Test 13: Checking PersistentVolumeClaims..."
if kubectl get pvc -n "$NAMESPACE" &> /dev/null; then
    BOUND_PVCS=$(kubectl get pvc -n "$NAMESPACE" -o json | jq -r '.items[] | select(.status.phase=="Bound") | .metadata.name' | wc -l)
    TOTAL_PVCS=$(kubectl get pvc -n "$NAMESPACE" --no-headers | wc -l)
    log_info "PVCs: $BOUND_PVCS/$TOTAL_PVCS are bound ✓"
else
    log_warn "No PVCs found (stateful services may be external)"
fi

# Test 14: Check Ingress
log_info "Test 14: Checking Ingress..."
if kubectl get ingress -n "$NAMESPACE" &> /dev/null; then
    INGRESS_HOST=$(kubectl get ingress -n "$NAMESPACE" -o jsonpath='{.items[0].spec.rules[0].host}')
    log_info "Ingress configured for host: $INGRESS_HOST ✓"

    # Check if TLS is configured
    TLS_SECRET=$(kubectl get ingress -n "$NAMESPACE" -o jsonpath='{.items[0].spec.tls[0].secretName}')
    if [ -n "$TLS_SECRET" ]; then
        log_info "TLS configured with secret: $TLS_SECRET ✓"
    else
        log_warn "TLS not configured"
    fi
else
    log_warn "No Ingress found"
fi

# Test 15: Check for recent errors in logs
log_info "Test 15: Checking recent logs for errors..."
FIRST_POD=$(kubectl get pods -n "$NAMESPACE" -l app=donelist,component=api -o jsonpath='{.items[0].metadata.name}')
ERROR_COUNT=$(kubectl logs "$FIRST_POD" -n "$NAMESPACE" --tail=100 | grep -i error | wc -l || true)

if [ "$ERROR_COUNT" -gt 5 ]; then
    log_warn "Found $ERROR_COUNT error messages in recent logs"
    kubectl logs "$FIRST_POD" -n "$NAMESPACE" --tail=20
else
    log_info "No significant errors in recent logs ✓"
fi

# Test 16: Check resource usage
log_info "Test 16: Checking resource usage..."
if command -v kubectl &> /dev/null && kubectl top pod -n "$NAMESPACE" &> /dev/null; then
    log_info "Resource usage:"
    kubectl top pod -n "$NAMESPACE" -l app=donelist,component=api
else
    log_warn "Cannot check resource usage (metrics-server may not be installed)"
fi

# Test 17: Check monitoring setup
log_info "Test 17: Checking monitoring setup..."
if kubectl get servicemonitor -n "$NAMESPACE" &> /dev/null 2>&1; then
    SM_COUNT=$(kubectl get servicemonitor -n "$NAMESPACE" | grep -c donelist || true)
    log_info "ServiceMonitor configured ($SM_COUNT found) ✓"
else
    log_warn "ServiceMonitor not found (Prometheus operator may not be installed)"
fi

if kubectl get prometheusrule -n "$NAMESPACE" &> /dev/null 2>&1; then
    PR_COUNT=$(kubectl get prometheusrule -n "$NAMESPACE" | grep -c donelist || true)
    log_info "PrometheusRules configured ($PR_COUNT found) ✓"
else
    log_warn "PrometheusRules not found"
fi

# Test 18: Rolling update test (optional, commented out by default)
# log_info "Test 18: Testing rolling update..."
# kubectl set image deployment/"$DEPLOYMENT_NAME" donelist-api=your-registry/donelist-api:latest -n "$NAMESPACE"
# kubectl rollout status deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout="${TIMEOUT}s"
# log_info "Rolling update successful ✓"

# Final summary
log_info ""
log_info "=================================="
log_info "All tests passed successfully! ✓"
log_info "=================================="
log_info ""
log_info "Deployment summary:"
log_info "  Namespace: $NAMESPACE"
log_info "  Deployment: $DEPLOYMENT_NAME"
log_info "  Replicas: $READY_REPLICAS/$DESIRED_REPLICAS"
log_info "  Running pods: $RUNNING_PODS"
log_info "  Service endpoints: $ENDPOINTS"
log_info ""
log_info "Next steps:"
log_info "  1. Test the application through the ingress: https://$INGRESS_HOST"
log_info "  2. Monitor logs: kubectl logs -f -n $NAMESPACE -l app=donelist,component=api"
log_info "  3. Check metrics: kubectl top pods -n $NAMESPACE"
log_info "  4. View events: kubectl get events -n $NAMESPACE"
log_info ""

exit 0

#!/bin/bash
set -euo pipefail

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }

CHART_DIR="./helm/donelist-api"
ERRORS=0

log_info "========================================="
log_info "Helm Chart Validation"
log_info "========================================="
log_info ""

# Check if chart directory exists
if [ ! -d "$CHART_DIR" ]; then
    log_error "Chart directory not found: $CHART_DIR"
    exit 1
fi

# 1. Lint the chart
log_info "1. Linting Helm chart..."
if helm lint "$CHART_DIR"; then
    log_success "Chart linting passed"
else
    log_error "Chart linting failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 2. Lint with staging values
log_info "2. Linting with staging values..."
if helm lint "$CHART_DIR" -f "$CHART_DIR/values-staging.yaml"; then
    log_success "Staging values linting passed"
else
    log_error "Staging values linting failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 3. Lint with production values
log_info "3. Linting with production values..."
if helm lint "$CHART_DIR" -f "$CHART_DIR/values-production.yaml"; then
    log_success "Production values linting passed"
else
    log_error "Production values linting failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 4. Template rendering
log_info "4. Testing template rendering..."
if helm template test-release "$CHART_DIR" > /dev/null; then
    log_success "Template rendering successful"
else
    log_error "Template rendering failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 5. Validate generated manifests
log_info "5. Validating Kubernetes manifests..."
if helm template test-release "$CHART_DIR" | kubectl apply --dry-run=client -f - > /dev/null 2>&1; then
    log_success "Kubernetes manifest validation passed"
else
    log_error "Kubernetes manifest validation failed"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 6. Check required files
log_info "6. Checking required files..."
REQUIRED_FILES=(
    "Chart.yaml"
    "values.yaml"
    "values-staging.yaml"
    "values-production.yaml"
    "templates/_helpers.tpl"
    "templates/deployment.yaml"
    "templates/service.yaml"
    "templates/ingress.yaml"
    "templates/configmap.yaml"
    "templates/secret.yaml"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$CHART_DIR/$file" ]; then
        log_success "  ✓ $file exists"
    else
        log_error "  ✗ $file missing"
        ERRORS=$((ERRORS + 1))
    fi
done
echo ""

# 7. Check Chart.yaml
log_info "7. Validating Chart.yaml..."
if grep -q "apiVersion: v2" "$CHART_DIR/Chart.yaml"; then
    log_success "  ✓ API version is v2"
else
    log_error "  ✗ API version is not v2"
    ERRORS=$((ERRORS + 1))
fi

if grep -q "name: donelist-api" "$CHART_DIR/Chart.yaml"; then
    log_success "  ✓ Chart name is correct"
else
    log_error "  ✗ Chart name is incorrect"
    ERRORS=$((ERRORS + 1))
fi
echo ""

# 8. Generate and inspect manifests
log_info "8. Generating sample manifests..."
helm template test-release "$CHART_DIR" \
    --set image.tag=test \
    --output-dir /tmp/helm-test > /dev/null

MANIFEST_COUNT=$(find /tmp/helm-test -name "*.yaml" | wc -l)
log_info "Generated $MANIFEST_COUNT manifest files"

if [ "$MANIFEST_COUNT" -ge 5 ]; then
    log_success "Sufficient manifests generated"
else
    log_error "Too few manifests generated"
    ERRORS=$((ERRORS + 1))
fi

# Cleanup
rm -rf /tmp/helm-test
echo ""

# Summary
log_info "========================================="
log_info "Validation Summary"
log_info "========================================="

if [ $ERRORS -eq 0 ]; then
    log_success "All validation checks passed! ✓"
    log_info ""
    log_info "Helm chart is ready for deployment."
    exit 0
else
    log_error "Validation failed with $ERRORS error(s). ✗"
    log_info ""
    log_info "Please fix the errors and run validation again."
    exit 1
fi

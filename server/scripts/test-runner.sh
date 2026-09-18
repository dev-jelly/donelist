#!/bin/bash
# Comprehensive Test Runner Script for Donelist API
# This script runs all test suites with coverage reporting

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_THRESHOLD=70
TEST_TIMEOUT=120s
COVERAGE_DIR="coverage"
REPORTS_DIR="test-reports"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Donelist API Test Suite Runner${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Create directories
mkdir -p "$COVERAGE_DIR" "$REPORTS_DIR"

# Clean previous coverage data
rm -f coverage.out "$COVERAGE_DIR"/*.out

# Function to print section headers
print_section() {
    echo -e "\n${BLUE}>>> $1${NC}\n"
}

# Function to check if tests passed
check_result() {
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ $1 passed${NC}"
        return 0
    else
        echo -e "${RED}✗ $1 failed${NC}"
        return 1
    fi
}

# Run unit tests
print_section "Running Unit Tests"
go test -v -race -timeout "$TEST_TIMEOUT" -coverprofile="$COVERAGE_DIR/unit.out" -covermode=atomic \
    $(go list ./... | grep -v /tests/) || check_result "Unit tests"

# Run integration tests
print_section "Running Integration Tests"
if [ -d "tests/integration" ]; then
    go test -v -timeout "$TEST_TIMEOUT" -tags=integration \
        -coverprofile="$COVERAGE_DIR/integration.out" -covermode=atomic \
        ./tests/integration/... || check_result "Integration tests"
else
    echo -e "${YELLOW}⚠ No integration tests found${NC}"
fi

# Run E2E tests
print_section "Running E2E Tests"
if [ -d "tests/e2e" ]; then
    go test -v -timeout "$TEST_TIMEOUT" -tags=e2e \
        -coverprofile="$COVERAGE_DIR/e2e.out" -covermode=atomic \
        ./tests/e2e/... || check_result "E2E tests"
else
    echo -e "${YELLOW}⚠ No E2E tests found${NC}"
fi

# Merge coverage reports
print_section "Merging Coverage Reports"
echo "mode: atomic" > coverage.out
find "$COVERAGE_DIR" -name "*.out" -type f | while read file; do
    tail -n +2 "$file" >> coverage.out
done

# Generate coverage report
print_section "Coverage Report"
go tool cover -func=coverage.out | tail -n 1

COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
echo -e "\nTotal Coverage: ${BLUE}${COVERAGE}%${NC}"

# Check coverage threshold
if (( $(echo "$COVERAGE < $COVERAGE_THRESHOLD" | bc -l) )); then
    echo -e "${YELLOW}⚠ Coverage is below ${COVERAGE_THRESHOLD}% threshold${NC}"
    echo -e "Current: ${COVERAGE}%, Target: ${COVERAGE_THRESHOLD}%"
else
    echo -e "${GREEN}✓ Coverage meets ${COVERAGE_THRESHOLD}% threshold${NC}"
fi

# Generate HTML coverage report
print_section "Generating HTML Coverage Report"
go tool cover -html=coverage.out -o "$REPORTS_DIR/coverage.html"
echo -e "HTML report: ${BLUE}$REPORTS_DIR/coverage.html${NC}"

# Generate coverage by package
print_section "Coverage by Package"
go tool cover -func=coverage.out | grep -v "total:" | awk '{print $1, $3}' | column -t > "$REPORTS_DIR/coverage-by-package.txt"
cat "$REPORTS_DIR/coverage-by-package.txt" | head -20

# Generate test summary
print_section "Test Summary"
cat > "$REPORTS_DIR/test-summary.txt" <<EOF
Donelist API Test Summary
Generated: $(date)
================================

Coverage: ${COVERAGE}%
Threshold: ${COVERAGE_THRESHOLD}%
Status: $([ $(echo "$COVERAGE >= $COVERAGE_THRESHOLD" | bc -l) -eq 1 ] && echo "PASS" || echo "WARN")

Test Suites:
- Unit Tests: $([ -f "$COVERAGE_DIR/unit.out" ] && echo "✓ PASS" || echo "✗ FAIL")
- Integration Tests: $([ -f "$COVERAGE_DIR/integration.out" ] && echo "✓ PASS" || echo "- SKIP")
- E2E Tests: $([ -f "$COVERAGE_DIR/e2e.out" ] && echo "✓ PASS" || echo "- SKIP")

Reports Generated:
- Coverage HTML: $REPORTS_DIR/coverage.html
- Coverage by Package: $REPORTS_DIR/coverage-by-package.txt
- This Summary: $REPORTS_DIR/test-summary.txt
EOF

cat "$REPORTS_DIR/test-summary.txt"

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}  Test Suite Completed${NC}"
echo -e "${GREEN}========================================${NC}\n"

# Exit with appropriate code
if (( $(echo "$COVERAGE < $COVERAGE_THRESHOLD" | bc -l) )); then
    exit 1
fi

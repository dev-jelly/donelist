#!/bin/bash

# Security Test Suite Runner
# This script runs comprehensive security tests for the Donelist server

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "================================"
echo "Security Test Suite"
echo "================================"
echo ""

# Function to print colored output
print_status() {
    if [ "$1" == "success" ]; then
        echo -e "${GREEN}✓${NC} $2"
    elif [ "$1" == "error" ]; then
        echo -e "${RED}✗${NC} $2"
    elif [ "$1" == "warning" ]; then
        echo -e "${YELLOW}⚠${NC} $2"
    else
        echo "$2"
    fi
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Track test results
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Create results directory
RESULTS_DIR="security-test-results"
mkdir -p "$RESULTS_DIR"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# 1. Run Gosec (Go Security Checker)
print_status "info" "Running Gosec security scan..."
if command_exists gosec; then
    if gosec -fmt json -out "$RESULTS_DIR/gosec_${TIMESTAMP}.json" ./... 2>/dev/null; then
        print_status "success" "Gosec scan completed - no critical issues found"
        ((TESTS_PASSED++))
    else
        print_status "warning" "Gosec found potential security issues (see $RESULTS_DIR/gosec_${TIMESTAMP}.json)"
        ((TESTS_FAILED++))
    fi
else
    print_status "warning" "Gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"
    ((TESTS_SKIPPED++))
fi
echo ""

# 2. Check for hardcoded secrets
print_status "info" "Checking for hardcoded secrets..."
SECRETS_FOUND=0

# Check for common patterns
for pattern in "password\s*=\s*\"" "secret\s*=\s*\"" "api_key\s*=\s*\"" "token\s*=\s*\""; do
    if grep -r --include="*.go" -E "$pattern" . --exclude-dir=.git --exclude-dir=vendor 2>/dev/null | grep -v -E "test|mock|example|config\.go"; then
        SECRETS_FOUND=1
    fi
done

if [ $SECRETS_FOUND -eq 0 ]; then
    print_status "success" "No hardcoded secrets detected"
    ((TESTS_PASSED++))
else
    print_status "error" "Potential hardcoded secrets found!"
    ((TESTS_FAILED++))
fi
echo ""

# 3. Run SQL injection vulnerability checks
print_status "info" "Checking for SQL injection vulnerabilities..."
SQL_VULNS=0

# Check for string concatenation in SQL queries
if grep -r --include="*.go" -E "fmt\.Sprintf.*SELECT|\"SELECT.*\+|\"INSERT.*\+|\"UPDATE.*\+|\"DELETE.*\+" . --exclude-dir=.git --exclude-dir=vendor 2>/dev/null; then
    SQL_VULNS=1
    print_status "warning" "Found potential SQL injection vulnerabilities (string concatenation in queries)"
fi

# Check for proper parameter binding
PARAM_BINDING=$(grep -r --include="*.go" -E "\\\$[0-9]+|\?" . --exclude-dir=.git --exclude-dir=vendor 2>/dev/null | wc -l)
if [ "$PARAM_BINDING" -gt 0 ]; then
    print_status "info" "Found $PARAM_BINDING instances of parameterized queries (good!)"
fi

if [ $SQL_VULNS -eq 0 ]; then
    print_status "success" "No obvious SQL injection vulnerabilities detected"
    ((TESTS_PASSED++))
else
    ((TESTS_FAILED++))
fi
echo ""

# 4. Check authentication middleware
print_status "info" "Checking authentication middleware..."
AUTH_CHECK=0

# Check if JWT validation exists
if grep -r --include="*.go" "VerifyToken\|ValidateJWT\|ParseToken" . --exclude-dir=.git --exclude-dir=vendor >/dev/null 2>&1; then
    print_status "success" "JWT validation found"
    ((TESTS_PASSED++))
else
    print_status "warning" "JWT validation not found - ensure proper authentication"
    ((TESTS_FAILED++))
fi

# Check for rate limiting
if grep -r --include="*.go" "RateLimit\|ratelimit\|Limiter" . --exclude-dir=.git --exclude-dir=vendor >/dev/null 2>&1; then
    print_status "success" "Rate limiting implementation found"
    ((TESTS_PASSED++))
else
    print_status "warning" "Rate limiting not found - consider implementing to prevent abuse"
    ((TESTS_FAILED++))
fi
echo ""

# 5. Check for proper error handling
print_status "info" "Checking error handling..."
UNHANDLED_ERRORS=$(grep -r --include="*.go" "_\s*=.*\(err\|error\)" . --exclude-dir=.git --exclude-dir=vendor --exclude-dir=tests 2>/dev/null | wc -l)

if [ "$UNHANDLED_ERRORS" -gt 0 ]; then
    print_status "warning" "Found $UNHANDLED_ERRORS instances of ignored errors"
    ((TESTS_FAILED++))
else
    print_status "success" "No obviously ignored errors found"
    ((TESTS_PASSED++))
fi
echo ""

# 6. Check CORS configuration
print_status "info" "Checking CORS configuration..."
if grep -r --include="*.go" "AllowOrigins.*\\\*" . --exclude-dir=.git --exclude-dir=vendor 2>/dev/null | grep -v test; then
    print_status "warning" "Found wildcard CORS origin (*) - consider restricting in production"
    ((TESTS_FAILED++))
else
    print_status "success" "No wildcard CORS origins found"
    ((TESTS_PASSED++))
fi
echo ""

# 7. Check for XSS protection
print_status "info" "Checking XSS protection..."
XSS_PROTECTED=0

# Check for HTML escaping
if grep -r --include="*.go" "html\.Escape\|template\.HTML\|SanitizeHTML" . --exclude-dir=.git --exclude-dir=vendor >/dev/null 2>&1; then
    print_status "success" "HTML escaping/sanitization found"
    ((TESTS_PASSED++))
else
    print_status "warning" "No HTML escaping found - ensure proper XSS protection"
    ((TESTS_FAILED++))
fi

# Check for Content-Security-Policy headers
if grep -r --include="*.go" "Content-Security-Policy\|CSP" . --exclude-dir=.git --exclude-dir=vendor >/dev/null 2>&1; then
    print_status "success" "CSP headers found"
    ((TESTS_PASSED++))
else
    print_status "warning" "No CSP headers found - consider adding for XSS protection"
    ((TESTS_FAILED++))
fi
echo ""

# 8. Check TLS/HTTPS configuration
print_status "info" "Checking TLS configuration..."
if grep -r --include="*.go" "tls\.Config\|TLSConfig" . --exclude-dir=.git --exclude-dir=vendor >/dev/null 2>&1; then
    # Check for weak TLS versions
    if grep -r --include="*.go" "tls\.VersionTLS10\|tls\.VersionTLS11" . --exclude-dir=.git --exclude-dir=vendor 2>/dev/null; then
        print_status "warning" "Found references to weak TLS versions (TLS 1.0/1.1)"
        ((TESTS_FAILED++))
    else
        print_status "success" "TLS configuration found (no weak versions detected)"
        ((TESTS_PASSED++))
    fi
else
    print_status "warning" "No TLS configuration found"
    ((TESTS_FAILED++))
fi
echo ""

# 9. Check for dependency vulnerabilities
print_status "info" "Checking for dependency vulnerabilities..."
if command_exists nancy; then
    if go list -json -deps ./... | nancy sleuth 2>/dev/null; then
        print_status "success" "No known vulnerabilities in dependencies"
        ((TESTS_PASSED++))
    else
        print_status "warning" "Found vulnerabilities in dependencies"
        ((TESTS_FAILED++))
    fi
elif command_exists govulncheck; then
    if govulncheck ./... 2>/dev/null; then
        print_status "success" "No known vulnerabilities found by govulncheck"
        ((TESTS_PASSED++))
    else
        print_status "warning" "Vulnerabilities found by govulncheck"
        ((TESTS_FAILED++))
    fi
else
    print_status "warning" "No vulnerability scanner installed (nancy or govulncheck)"
    ((TESTS_SKIPPED++))
fi
echo ""

# 10. Run security-focused unit tests
print_status "info" "Running security-focused unit tests..."
if go test -v -tags=security ./internal/security/... ./internal/auth/... 2>/dev/null; then
    print_status "success" "Security tests passed"
    ((TESTS_PASSED++))
else
    print_status "error" "Security tests failed"
    ((TESTS_FAILED++))
fi
echo ""

# Generate summary report
echo "================================"
echo "Security Test Summary"
echo "================================"
echo ""
print_status "info" "Tests Passed:  $TESTS_PASSED"
print_status "info" "Tests Failed:  $TESTS_FAILED"
print_status "info" "Tests Skipped: $TESTS_SKIPPED"
echo ""

# Generate JSON report
cat > "$RESULTS_DIR/summary_${TIMESTAMP}.json" <<EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "tests_passed": $TESTS_PASSED,
  "tests_failed": $TESTS_FAILED,
  "tests_skipped": $TESTS_SKIPPED,
  "total_tests": $((TESTS_PASSED + TESTS_FAILED + TESTS_SKIPPED))
}
EOF

# Exit with appropriate code
if [ $TESTS_FAILED -gt 0 ]; then
    print_status "error" "Security tests completed with failures"
    echo "Results saved to: $RESULTS_DIR/"
    exit 1
else
    print_status "success" "All security tests passed!"
    echo "Results saved to: $RESULTS_DIR/"
    exit 0
fi
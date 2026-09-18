#!/bin/bash
#
# Test Coverage Checker
# Ensures test coverage meets minimum thresholds
#

set -e

# Configuration
MIN_COVERAGE=${MIN_COVERAGE:-80.0}
COVERAGE_FILE=${COVERAGE_FILE:-coverage.out}
COVERAGE_REPORT=${COVERAGE_REPORT:-coverage.html}
COVERAGE_JSON=${COVERAGE_JSON:-coverage.json}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================="
echo "  Test Coverage Validation"
echo "========================================="
echo ""

# Check if coverage file exists
if [ ! -f "$COVERAGE_FILE" ]; then
    echo -e "${RED}❌ Coverage file not found: $COVERAGE_FILE${NC}"
    echo "Run 'make test-coverage' first to generate coverage data"
    exit 1
fi

# Extract total coverage percentage
COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep total | awk '{print $3}' | sed 's/%//')

if [ -z "$COVERAGE" ]; then
    echo -e "${RED}❌ Failed to extract coverage percentage${NC}"
    exit 1
fi

echo "Current Coverage: ${COVERAGE}%"
echo "Minimum Required: ${MIN_COVERAGE}%"
echo ""

# Generate detailed coverage by package
echo "Coverage by Package:"
echo "-------------------"
go tool cover -func="$COVERAGE_FILE" | grep -v "total:" | awk '{print $1 " - " $3}' | sort -t- -k2 -rn | head -20

echo ""

# Check if coverage meets threshold
if (( $(echo "$COVERAGE >= $MIN_COVERAGE" | bc -l) )); then
    echo -e "${GREEN}✅ Coverage check PASSED: ${COVERAGE}% >= ${MIN_COVERAGE}%${NC}"

    # Generate badge data
    BADGE_COLOR="brightgreen"
    if (( $(echo "$COVERAGE < 90" | bc -l) )); then
        BADGE_COLOR="green"
    fi
    if (( $(echo "$COVERAGE < 85" | bc -l) )); then
        BADGE_COLOR="yellowgreen"
    fi

    # Create JSON for badge
    cat > "$COVERAGE_JSON" <<EOF
{
  "schemaVersion": 1,
  "label": "coverage",
  "message": "${COVERAGE}%",
  "color": "${BADGE_COLOR}"
}
EOF

    echo "Badge data saved to: $COVERAGE_JSON"
    exit 0
else
    DIFF=$(echo "$MIN_COVERAGE - $COVERAGE" | bc)
    echo -e "${RED}❌ Coverage check FAILED${NC}"
    echo -e "${RED}Coverage is ${DIFF}% below minimum threshold${NC}"
    echo ""
    echo "To improve coverage:"
    echo "  1. Run 'make test-coverage' to see uncovered code"
    echo "  2. Add tests for uncovered functions"
    echo "  3. Focus on packages with lowest coverage"
    exit 1
fi

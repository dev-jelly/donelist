#!/bin/bash
#
# Generate comprehensive coverage report
#

set -e

# Configuration
COVERAGE_DIR="${COVERAGE_DIR:-.coverage}"
REPORT_DIR="${REPORT_DIR:-docs/coverage}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "========================================="
echo "  Generating Coverage Reports"
echo "========================================="
echo ""

# Create directories
mkdir -p "$COVERAGE_DIR"
mkdir -p "$REPORT_DIR"

# Run tests with coverage
echo "Running tests with coverage..."
go test -v -coverprofile="$COVERAGE_DIR/coverage.out" -covermode=atomic ./... > "$COVERAGE_DIR/test-output.txt" 2>&1

# Check if tests passed
if [ $? -ne 0 ]; then
    echo "❌ Tests failed. Coverage report incomplete."
    cat "$COVERAGE_DIR/test-output.txt"
    exit 1
fi

echo -e "${GREEN}✅ Tests passed${NC}"
echo ""

# Generate HTML report
echo "Generating HTML coverage report..."
go tool cover -html="$COVERAGE_DIR/coverage.out" -o "$COVERAGE_DIR/coverage.html"

# Generate function-level coverage
echo "Generating function coverage report..."
go tool cover -func="$COVERAGE_DIR/coverage.out" > "$COVERAGE_DIR/coverage-func.txt"

# Extract total coverage
TOTAL_COVERAGE=$(go tool cover -func="$COVERAGE_DIR/coverage.out" | grep total | awk '{print $3}')

echo -e "${BLUE}Total Coverage: ${TOTAL_COVERAGE}${NC}"
echo ""

# Generate coverage by package
echo "Coverage by Package:" | tee "$COVERAGE_DIR/coverage-by-package.txt"
echo "-------------------" | tee -a "$COVERAGE_DIR/coverage-by-package.txt"

go tool cover -func="$COVERAGE_DIR/coverage.out" | \
    awk '/^github.com/ {pkg=$1; getline; total=0; count=0; while ($1 ~ /^[^g]/) {total+=$3; count++; if (getline == 0) break} if (count > 0) print pkg, total/count"%"}' | \
    sort -t% -k2 -rn | \
    tee -a "$COVERAGE_DIR/coverage-by-package.txt"

echo ""

# Generate low coverage files report
echo "Files with Coverage < 80%:" | tee "$COVERAGE_DIR/low-coverage-files.txt"
echo "-------------------------" | tee -a "$COVERAGE_DIR/low-coverage-files.txt"

go tool cover -func="$COVERAGE_DIR/coverage.out" | \
    awk 'NR>1 && $3!="total:" {gsub(/%/,"",$3); if ($3 < 80) print $1, $2, $3"%"}' | \
    sort -t% -k3 -n | \
    tee -a "$COVERAGE_DIR/low-coverage-files.txt"

echo ""

# Copy to report directory with timestamp
cp "$COVERAGE_DIR/coverage.html" "$REPORT_DIR/coverage-${TIMESTAMP}.html"
cp "$COVERAGE_DIR/coverage.html" "$REPORT_DIR/coverage-latest.html"

# Create coverage trend data
TREND_FILE="$REPORT_DIR/coverage-trend.csv"

# Create header if file doesn't exist
if [ ! -f "$TREND_FILE" ]; then
    echo "timestamp,coverage_percentage,total_lines,covered_lines" > "$TREND_FILE"
fi

# Extract coverage data
COVERAGE_PCT=$(echo "$TOTAL_COVERAGE" | sed 's/%//')
TOTAL_LINES=$(go tool cover -func="$COVERAGE_DIR/coverage.out" | tail -1 | awk '{print $2}')
COVERED_LINES=$(go tool cover -func="$COVERAGE_DIR/coverage.out" | tail -1 | awk '{print $3}')

# Append to trend file
echo "$TIMESTAMP,$COVERAGE_PCT,$TOTAL_LINES,$COVERED_LINES" >> "$TREND_FILE"

echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}  Coverage Reports Generated${NC}"
echo -e "${GREEN}=========================================${NC}"
echo ""
echo "Reports Location:"
echo "  - HTML Report: $COVERAGE_DIR/coverage.html"
echo "  - Function Report: $COVERAGE_DIR/coverage-func.txt"
echo "  - Package Report: $COVERAGE_DIR/coverage-by-package.txt"
echo "  - Low Coverage Files: $COVERAGE_DIR/low-coverage-files.txt"
echo "  - Latest Report: $REPORT_DIR/coverage-latest.html"
echo "  - Coverage Trend: $TREND_FILE"
echo ""
echo "Open HTML report: open $COVERAGE_DIR/coverage.html"

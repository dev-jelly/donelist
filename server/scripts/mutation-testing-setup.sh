#!/bin/bash
# Mutation Testing Setup and Validation Script
#
# This script sets up and validates the mutation testing infrastructure
# for the Donelist API project.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================"
echo "Mutation Testing Setup & Validation"
echo "========================================"
echo ""

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "success")
            echo -e "${GREEN}✓${NC} $message"
            ;;
        "error")
            echo -e "${RED}✗${NC} $message"
            ;;
        "warning")
            echo -e "${YELLOW}!${NC} $message"
            ;;
        "info")
            echo -e "  $message"
            ;;
    esac
}

# Check if Go is installed
print_status "info" "Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    print_status "success" "Go is installed: $GO_VERSION"
else
    print_status "error" "Go is not installed. Please install Go first."
    exit 1
fi

# Check if gremlins is installed
print_status "info" "Checking mutation testing tool..."
if [ -f "$HOME/go/bin/gremlins" ]; then
    print_status "success" "Gremlins is installed"
else
    print_status "warning" "Gremlins not found, attempting to install..."
    go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
    if [ -f "$HOME/go/bin/gremlins" ]; then
        print_status "success" "Gremlins installed successfully"
    else
        print_status "error" "Failed to install gremlins"
        print_status "info" "You can install manually: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"
        exit 1
    fi
fi

# Check for configuration file
print_status "info" "Checking configuration..."
if [ -f ".gremlins.yml" ]; then
    print_status "success" "Configuration file found"
else
    print_status "error" "Configuration file .gremlins.yml not found"
    exit 1
fi

# Check documentation
print_status "info" "Checking documentation..."
if [ -f "docs/testing/MUTATION_TESTING.md" ]; then
    print_status "success" "Mutation testing documentation found"
else
    print_status "warning" "Documentation not found at docs/testing/MUTATION_TESTING.md"
fi

# Run basic tests first
print_status "info" "Running unit tests to ensure baseline..."
if go test -short ./internal/health/... &> /dev/null; then
    print_status "success" "Unit tests pass"
else
    print_status "error" "Unit tests failing - fix tests before mutation testing"
    exit 1
fi

# Create output directory for reports
mkdir -p reports/mutation-testing

# Run a small mutation test on health package (quick validation)
print_status "info" "Running validation mutation test on health package..."
print_status "info" "This may take a few minutes..."

# Dry run first to show what would be tested
if $HOME/go/bin/gremlins unleash --dry-run ./internal/health/... 2>&1 | head -20; then
    print_status "success" "Mutation testing tool is functional"
else
    print_status "warning" "Mutation testing tool may have issues"
    print_status "info" "Check that tests compile and run successfully first"
fi

echo ""
echo "========================================"
echo "Setup Summary"
echo "========================================"
echo ""
print_status "success" "Mutation testing infrastructure is configured"
echo ""
echo "Next steps:"
echo "1. Run 'make test-mutation-health' to test the health package"
echo "2. Review the mutation report in reports/mutation-testing/"
echo "3. Check docs/testing/MUTATION_TESTING.md for detailed guidance"
echo "4. Set up CI/CD integration for critical paths"
echo ""
echo "Target mutation kill rates:"
echo "  - Critical paths (auth, payment): 90%+"
echo "  - Business logic: 80%+"
echo "  - Utilities: 70%+"
echo ""

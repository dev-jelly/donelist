# Test Coverage Documentation

This document describes the test coverage strategy, tools, and processes for the DoneList API project.

## Overview

We maintain high test coverage to ensure code quality, prevent regressions, and facilitate confident refactoring. Our minimum coverage threshold is **70%**, with a target of **80%+**.

## Coverage Tools

### Built-in Go Coverage

We use Go's built-in coverage tools:
- `go test -coverprofile=coverage.out` - Generate coverage data
- `go tool cover -html` - Generate HTML report
- `go tool cover -func` - Show function-level coverage

### Custom Scripts

Located in `scripts/coverage/`:

1. **check-coverage.sh** - Validates coverage meets minimum threshold
2. **coverage-report.sh** - Generates comprehensive coverage reports

## Running Coverage Analysis

### Quick Coverage Check

```bash
# Run tests and check coverage threshold
make test-coverage-check

# This will:
# - Run all tests with coverage
# - Check if coverage >= 70%
# - Fail if below threshold
# - Generate coverage.json for badges
```

### Detailed Coverage Report

```bash
# Generate HTML and detailed reports
make test-coverage-report

# View HTML report
open coverage.html
```

### Comprehensive CI Report

```bash
# Generate all coverage artifacts
make test-coverage-ci

# Outputs:
# - .coverage/coverage.html
# - .coverage/coverage-func.txt
# - .coverage/coverage-by-package.txt
# - .coverage/low-coverage-files.txt
# - docs/coverage/coverage-trend.csv
```

## Coverage Thresholds

| Level | Threshold | Status |
|-------|-----------|--------|
| **Minimum** | 70% | Builds fail below this |
| **Target** | 80% | Desired baseline |
| **Excellent** | 90%+ | Exceptional quality |

### Package-Specific Goals

| Package | Target | Notes |
|---------|--------|-------|
| `internal/auth` | 90%+ | Critical security code |
| `internal/user` | 85%+ | Core business logic |
| `internal/checkin` | 85%+ | Primary feature |
| `internal/api/handlers` | 80%+ | User-facing endpoints |
| `internal/webhook` | 80%+ | Integration reliability |
| `pkg/*` | 85%+ | Reusable packages |

## CI/CD Integration

### GitHub Actions Workflow

Located at `.github/workflows/coverage.yml`:

**Triggers**:
- Push to `main` or `develop`
- Pull requests to `main` or `develop`

**Steps**:
1. Run all tests with coverage
2. Generate coverage reports
3. Check minimum threshold (70%)
4. Upload to Codecov
5. Comment on PR with coverage summary
6. Fail build if below threshold

**Artifacts**:
- Coverage reports retained for 30 days
- Available for download from Actions tab

### Pull Request Comments

Automated PR comments include:
- Total coverage percentage
- Coverage by package
- Warning if below threshold
- Link to detailed report

Example:
```
## 📊 Test Coverage Report

**Total Coverage:** 82.5%

<details>
<summary>Coverage by Package</summary>

...
</details>

✅ Coverage meets minimum requirements
```

## Coverage Badges

### Setup

1. **Codecov Badge** (Recommended):
   ```markdown
   ![Coverage](https://codecov.io/gh/yourusername/donelist/branch/main/graph/badge.svg)
   ```

2. **Shields.io Dynamic Badge**:
   ```markdown
   ![Coverage](https://img.shields.io/endpoint?url=https://yourapi.com/coverage.json)
   ```

3. **Generate Badge Data**:
   ```bash
   make test-coverage-badge
   # Creates coverage.json in Shields.io format
   ```

### Badge JSON Format

The `coverage.json` file follows Shields.io endpoint schema:
```json
{
  "schemaVersion": 1,
  "label": "coverage",
  "message": "82.5%",
  "color": "brightgreen"
}
```

Colors based on coverage:
- **90%+**: `brightgreen`
- **85-89%**: `green`
- **80-84%**: `yellowgreen`
- **70-79%**: `yellow`
- **< 70%**: `red`

## Interpreting Coverage Reports

### HTML Report

Open `coverage.html` to see:
- **Green**: Covered lines
- **Red**: Uncovered lines
- **Gray**: Not executable (comments, etc.)

Navigate through packages and files to identify gaps.

### Function Coverage Report

View `coverage-func.txt` for function-level breakdown:
```
github.com/dev-jelly/donelist/internal/auth/jwt.go:63:     NewJWTManager    100.0%
github.com/dev-jelly/donelist/internal/auth/jwt.go:125:    GenerateToken    95.5%
github.com/dev-jelly/donelist/internal/auth/jwt.go:180:    ValidateToken    88.2%
```

### Coverage by Package

View `.coverage/coverage-by-package.txt`:
```
github.com/dev-jelly/donelist/internal/auth     92.5%
github.com/dev-jelly/donelist/internal/user     87.3%
github.com/dev-jelly/donelist/internal/checkin  84.1%
```

### Low Coverage Files

Check `.coverage/low-coverage-files.txt` for files below 80%:
```
internal/webhook/retry.go    handleRetry      65.0%
internal/analytics/cache.go  CacheRefresh     72.5%
```

## Coverage Trends

### Historical Tracking

Coverage trends are tracked in `docs/coverage/coverage-trend.csv`:
```csv
timestamp,coverage_percentage,total_lines,covered_lines
20241120_120000,82.5,15234,12568
20241121_093000,83.1,15301,12715
```

### Analyzing Trends

```bash
# View recent trends
tail -10 docs/coverage/coverage-trend.csv

# Plot trends (requires gnuplot)
gnuplot scripts/coverage/plot-trends.sh
```

Good trends:
- ✅ Steady increase or maintenance
- ✅ Coverage increases with new features

Bad trends:
- ❌ Declining coverage over time
- ❌ Large drops after new features

## Best Practices

### Writing Tests for Coverage

1. **Focus on Critical Paths**
   - Business logic > Boilerplate
   - Error handling is essential
   - Edge cases prevent bugs

2. **Don't Chase 100%**
   - Some code is hard to test (main functions, CLI code)
   - Focus on value, not just numbers
   - 80-90% is often optimal

3. **Test Types**
   - Unit tests: Fast, isolated, high coverage
   - Integration tests: Real dependencies, key flows
   - E2E tests: User scenarios, less coverage impact

### Improving Coverage

1. **Identify Gaps**
   ```bash
   # Generate report and find low coverage files
   make test-coverage-report
   cat .coverage/low-coverage-files.txt
   ```

2. **Prioritize**
   - Critical code first (auth, payments, data integrity)
   - High-traffic endpoints
   - Bug-prone areas

3. **Add Tests**
   - Write unit tests for functions
   - Add integration tests for flows
   - Document why certain code is untested

4. **Verify Improvement**
   ```bash
   # Check new coverage
   make test-coverage-check
   ```

### Coverage in Code Review

**PR Checklist**:
- [ ] Coverage doesn't decrease
- [ ] New code has reasonable coverage (70%+)
- [ ] Critical paths are tested
- [ ] Tests are meaningful (not just for coverage)

**Blocking Issues**:
- Coverage drop > 2%
- New critical code uncovered
- Threshold breach (< 70%)

## Troubleshooting

### Coverage Lower Than Expected

**Check**:
1. Are tests actually running?
   ```bash
   go test -v ./...
   ```

2. Is coverage file being generated?
   ```bash
   ls -la coverage.out
   ```

3. Are tests tagged correctly?
   ```bash
   # Integration tests need explicit flag
   go test -tags=integration ./...
   ```

### Coverage Check Failing in CI

**Common Causes**:
1. Threshold set too high
   - Adjust `MIN_COVERAGE` in workflow
   - Default is 70%

2. Tests failing before coverage check
   - Fix failing tests first
   - Check test logs in CI

3. Missing dependencies
   - Ensure PostgreSQL/Redis services running
   - Check environment variables

### Inconsistent Coverage Locally vs CI

**Causes**:
1. Different test suites
   - CI runs all tests including integration
   - Local might skip some

2. Race detector
   - CI uses `-race` flag
   - Can affect coverage slightly

3. Dependencies
   - CI has fresh environment
   - Local might have cached state

**Solution**:
```bash
# Run exact CI command
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
```

## Advanced Usage

### Coverage for Specific Packages

```bash
# Just auth package
go test -coverprofile=auth-coverage.out ./internal/auth/...
go tool cover -html=auth-coverage.out

# Multiple packages
go test -coverprofile=api-coverage.out ./internal/api/... ./internal/user/...
```

### Combining Coverage Files

```bash
# Run tests in parallel and combine
go test -coverprofile=coverage1.out ./internal/auth/...
go test -coverprofile=coverage2.out ./internal/user/...

# Merge coverage files
gocovmerge coverage1.out coverage2.out > coverage.out
```

### Coverage with Build Tags

```bash
# Include integration tests
go test -tags=integration -coverprofile=coverage.out ./...

# Include E2E tests
go test -tags=e2e -coverprofile=coverage.out ./...

# Multiple tags
go test -tags="integration,e2e" -coverprofile=coverage.out ./...
```

### Coverage Exclusions

Some code cannot/should not be tested:

**Exclude from Coverage**:
```go
// +build !test

package main

// This file is excluded from test coverage
```

**Or with build constraints**:
```go
//go:build !test
// +build !test
```

## Resources

- [Go Testing Documentation](https://golang.org/doc/code.html#Testing)
- [Go Coverage Tool](https://pkg.go.dev/cmd/cover)
- [Codecov Documentation](https://docs.codecov.com/)
- [Testing Best Practices](https://go.dev/doc/effective_go#testing)

## Support

For coverage issues:
1. Check this documentation first
2. Review CI logs for errors
3. Run locally with same commands as CI
4. Open issue with coverage reports attached

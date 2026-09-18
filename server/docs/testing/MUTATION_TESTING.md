# Mutation Testing Guide

## Overview

Mutation testing is a technique to evaluate the quality and effectiveness of your test suite by intentionally introducing bugs (mutations) into the code and verifying that tests catch these bugs.

## What is Mutation Testing?

Mutation testing works by:
1. **Creating Mutants**: Small changes are made to the code (e.g., changing `>` to `>=`, `+` to `-`)
2. **Running Tests**: The test suite runs against each mutant
3. **Evaluating Results**:
   - **Killed Mutant**: Tests failed (good - tests caught the bug)
   - **Survived Mutant**: Tests passed (bad - tests didn't catch the bug)
   - **Mutation Kill Rate**: Percentage of mutants killed by tests

## Why Mutation Testing?

Traditional code coverage metrics (line/branch coverage) only tell you which code is executed by tests, not whether tests actually verify the behavior. Mutation testing ensures your tests:
- **Assert meaningful behavior** rather than just execute code
- **Catch logic errors** and edge cases
- **Verify business rules** are properly enforced
- **Detect weak test assertions** that always pass

## Installation

The mutation testing tool (gremlins) is already installed in this project. If you need to install it manually:

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
```

## Running Mutation Tests

### Quick Start

```bash
# Run mutation tests on all critical paths
make test-mutation

# Run on specific package
make test-mutation-health
make test-mutation-auth
make test-mutation-payment

# Dry run to see what would be tested
make test-mutation-dry-run

# Only test changed files (faster for development)
make test-mutation-diff
```

### Manual Execution

```bash
# Run on specific package
gremlins unleash ./internal/health/...

# Run with specific mutators
gremlins unleash --mutators CONDITIONALS_BOUNDARY,ARITHM ETIC_BASE ./internal/auth/...

# Generate HTML report
gremlins unleash --html-report --html-output report.html
```

## Critical Paths

The following code paths have been identified as critical and require high mutation kill rates (90%+):

### 1. Authentication (`internal/auth/`)
- JWT token generation and validation
- Password hashing and verification
- Token refresh logic
- Session management
- Token blacklist checks

**Key Mutations to Test**:
- Token expiry boundary conditions
- Signature validation logic
- Password comparison operations
- Token refresh timing

### 2. Payment Processing (`internal/payment/`)
- Payment amount calculations
- Webhook signature verification
- Transaction state transitions
- Refund logic
- Subscription management

**Key Mutations to Test**:
- Arithmetic operations on amounts
- Conditional checks for payment states
- Webhook validation logic

### 3. Data Validation (`internal/security/validation.go`, `internal/middleware/validation.go`)
- Input sanitization
- SQL injection prevention
- XSS prevention
- Email validation
- UUID validation

**Key Mutations to Test**:
- Validation boundary conditions
- Regex patterns
- Sanitization logic

### 4. Health Checks (`internal/health/`)
- Database connectivity checks
- Redis connectivity checks
- System resource monitoring
- Health status aggregation

**Key Mutations to Test**:
- Status comparison operations
- Timeout logic
- Error handling paths

### 5. User Management (`internal/user/`)
- User creation and updates
- Permission checks
- User state transitions
- Profile validation

**Key Mutations to Test**:
- Permission boundary checks
- State transition logic
- Validation rules

## Configuration

Mutation testing configuration is defined in `.gremlins.yml`:

```yaml
# Mutation types (mutators)
mutators:
  - CONDITIONALS_BOUNDARY      # < becomes <=
  - CONDITIONALS_NEGATION      # == becomes !=
  - INCREMENT_DECREMENT        # i++ becomes i--
  - ARITHMETIC_BASE            # + becomes -
  # ... more mutators

# Target thresholds
thresholds:
  efficacy: 90                 # 90% mutation kill rate
  mutant-coverage: 80          # 80% of code mutated
```

## Interpreting Results

### Mutation Kill Rate

The **Mutation Kill Rate** is the percentage of mutations detected by your tests:

```
Kill Rate = (Killed Mutants / Total Mutants) × 100%
```

**Target Thresholds**:
- **Critical paths**: 90%+ kill rate
- **Business logic**: 80%+ kill rate
- **Utilities/helpers**: 70%+ kill rate

### Mutation Report

After running mutation tests, check `mutation-report.html` for:
1. **Overall Statistics**: Total mutants, killed, survived
2. **Per-File Breakdown**: Which files have low kill rates
3. **Survived Mutants**: Specific mutations that weren't caught
4. **Mutator Effectiveness**: Which mutation types are most effective

### Example Output

```
Mutation Testing Results
========================
Total Mutants: 150
Killed: 135 (90%)
Survived: 10 (6.7%)
Timeout: 5 (3.3%)

Top Survivors:
- internal/auth/jwt.go:45 - Changed > to >= (CONDITIONALS_BOUNDARY)
- internal/payment/service.go:102 - Changed + to - (ARITHMETIC_BASE)
```

## Improving Test Quality

### When Mutants Survive

If a mutant survives, it means your tests don't adequately verify that code path. To improve:

#### 1. Add Missing Assertions

**Bad** (mutant survives):
```go
func TestCalculateTotal(t *testing.T) {
    result := CalculateTotal(100, 0.1)
    // No assertion - just runs the code
}
```

**Good** (mutant killed):
```go
func TestCalculateTotal(t *testing.T) {
    result := CalculateTotal(100, 0.1)
    assert.Equal(t, 110.0, result) // Verifies the calculation
}
```

#### 2. Test Boundary Conditions

**Bad** (misses boundary):
```go
func TestIsAdult(t *testing.T) {
    assert.True(t, IsAdult(20))
    assert.False(t, IsAdult(10))
}
```

**Good** (tests boundary):
```go
func TestIsAdult(t *testing.T) {
    assert.True(t, IsAdult(20))
    assert.True(t, IsAdult(18))  // Boundary: exactly 18
    assert.False(t, IsAdult(17)) // Boundary: just under 18
    assert.False(t, IsAdult(10))
}
```

#### 3. Test Error Paths

**Bad** (only tests happy path):
```go
func TestDivide(t *testing.T) {
    result := Divide(10, 2)
    assert.Equal(t, 5, result)
}
```

**Good** (tests error case):
```go
func TestDivide(t *testing.T) {
    t.Run("successful division", func(t *testing.T) {
        result, err := Divide(10, 2)
        assert.NoError(t, err)
        assert.Equal(t, 5, result)
    })

    t.Run("division by zero", func(t *testing.T) {
        _, err := Divide(10, 0)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "division by zero")
    })
}
```

#### 4. Verify State Changes

**Bad** (doesn't verify state):
```go
func TestUpdateUser(t *testing.T) {
    service.UpdateUser(userID, "new@email.com")
    // Doesn't verify the update happened
}
```

**Good** (verifies state):
```go
func TestUpdateUser(t *testing.T) {
    service.UpdateUser(userID, "new@email.com")

    updated := service.GetUser(userID)
    assert.Equal(t, "new@email.com", updated.Email)
}
```

## Best Practices

### 1. Focus on Critical Paths First
Start with authentication, payment, and validation logic. These have the highest impact if bugs slip through.

### 2. Run Incrementally
Mutation testing can be slow. Use `--diff` mode during development to only test changed files:
```bash
make test-mutation-diff
```

### 3. Set Realistic Thresholds
- Start with lower thresholds (70%) and gradually increase
- Some code may legitimately have lower kill rates (e.g., simple getters/setters)
- Focus on business logic over boilerplate

### 4. Integrate into CI/CD
Add mutation testing to your CI pipeline for critical packages:
```yaml
# .github/workflows/mutation-tests.yml
- name: Run Mutation Tests
  run: make test-mutation-health

- name: Check Mutation Threshold
  run: |
    KILL_RATE=$(grep "efficacy" mutation-report.json | awk '{print $2}')
    if [ "$KILL_RATE" -lt 90 ]; then
      echo "Mutation kill rate below 90%"
      exit 1
    fi
```

### 5. Review Survived Mutants Regularly
Schedule regular reviews of mutation test results:
- Weekly: Review new survived mutants
- Monthly: Full mutation test run on all packages
- Release: Mutation tests on changed files

### 6. Document Acceptable Survivors
Some mutants may be acceptable to leave alive:
```go
// MUTATION_TESTING: Mutant survival acceptable here because...
// The boundary condition is covered by integration tests
func calculateAge(birthDate time.Time) int {
    // ...
}
```

## Common Mutators

### CONDITIONALS_BOUNDARY
Changes boundary conditions:
- `<` → `<=`
- `>` → `>=`
- Tests if your boundary checks are correct

### CONDITIONALS_NEGATION
Inverts conditional logic:
- `==` → `!=`
- `<` → `>=`
- Tests if you verify both positive and negative cases

### ARITHMETIC_BASE
Changes arithmetic operations:
- `+` → `-`
- `*` → `/`
- Tests if calculations are properly verified

### INCREMENT_DECREMENT
Changes increment/decrement:
- `i++` → `i--`
- `x += 1` → `x -= 1`
- Tests loop and counter logic

### INVERT_LOGICAL
Inverts logical operators:
- `&&` → `||`
- `||` → `&&`
- Tests complex boolean logic

## Troubleshooting

### Tests Timeout
If mutation tests timeout frequently:
1. Increase timeout in `.gremlins.yml`: `timeout: 60m`
2. Reduce the number of mutators tested
3. Test packages individually

### False Positives (Equivalent Mutants)
Some mutations don't change behavior:
```go
// Original
if x > 0 && x < 100 { ... }

// Mutant (equivalent behavior)
if x >= 1 && x < 100 { ... }
```
Document these in `.gremlins.yml` exclusions if they cause noise.

### High Memory Usage
Mutation testing can use significant memory:
1. Test smaller packages individually
2. Reduce `test-cpu` in config
3. Run in CI with more resources

## Continuous Improvement

Track mutation testing metrics over time:

| Sprint | Package | Kill Rate | Trend |
|--------|---------|-----------|-------|
| 1      | auth    | 75%       | -     |
| 2      | auth    | 82%       | ↑ 7%  |
| 3      | auth    | 91%       | ↑ 9%  |

Goals:
- **Short-term**: Achieve 80% kill rate on all critical paths
- **Medium-term**: Achieve 90% kill rate on auth and payment
- **Long-term**: Maintain 85%+ average across all packages

## Further Reading

- [Gremlins Documentation](https://github.com/go-gremlins/gremlins)
- [Mutation Testing Best Practices](https://pitest.org/quickstart/basic_concepts/)
- [Effective Unit Testing (Book)](https://www.manning.com/books/effective-unit-testing)

## Support

For questions or issues with mutation testing:
1. Check this guide first
2. Review `mutation-report.html` for specific mutants
3. Discuss in team code review sessions
4. Document findings in this guide for future reference

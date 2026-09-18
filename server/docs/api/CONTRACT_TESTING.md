# Contract Testing Guide

## Overview

This document describes the contract testing strategy for the DoneList API, including OpenAPI schema validation, Pact consumer-driven contracts, and CI/CD integration.

## Table of Contents

- [What is Contract Testing?](#what-is-contract-testing)
- [Setup](#setup)
- [Running Tests](#running-tests)
- [OpenAPI Schema Validation](#openapi-schema-validation)
- [Pact Contract Tests](#pact-contract-tests)
- [CI/CD Integration](#cicd-integration)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## What is Contract Testing?

Contract testing verifies that services can communicate with each other by testing the "contract" (API specification) between them. This catches integration issues early without requiring full end-to-end tests.

### Benefits

- **Early Detection**: Catch breaking changes before deployment
- **Faster Feedback**: Test in isolation without running full integration tests
- **Better Documentation**: Contracts serve as living documentation
- **Consumer-Driven**: Ensure API meets consumer needs
- **Backward Compatibility**: Verify changes don't break existing clients

## Setup

### Prerequisites

```bash
# Install Node.js dependencies
npm install -g @stoplight/spectral-cli @redocly/cli oasdiff

# Install Go dependencies
go get -u github.com/getkin/kin-openapi/openapi3
go get -u github.com/getkin/kin-openapi/openapi3filter

# Install swag for OpenAPI generation
go install github.com/swaggo/swag/cmd/swag@latest
```

### Directory Structure

```
server/
├── docs/api/
│   ├── openapi.yaml              # Main OpenAPI specification
│   ├── BREAKING_CHANGES.md       # Breaking change policy
│   ├── CONTRACT_TESTING.md       # This file
│   └── index.html                # Generated API docs
├── internal/api/
│   ├── contract_test.go          # OpenAPI contract tests
│   ├── pact_test.go              # Pact contract tests
│   └── schema_validation_test.go # Schema validation tests
├── tests/
│   ├── pacts/                    # Generated Pact contracts
│   │   ├── donelist-mobile-app-donelist-api-categories.json
│   │   ├── donelist-mobile-app-donelist-api-tags.json
│   │   └── donelist-mobile-app-donelist-api-timeline.json
│   └── snapshots/                # Response snapshots for regression
├── scripts/
│   ├── generate-openapi.sh       # Generate OpenAPI from code
│   └── validate-openapi.sh       # Validate OpenAPI spec
├── .spectral.yml                 # Spectral linting rules
└── Makefile                      # Test automation targets
```

## Running Tests

### All Contract Tests

```bash
make test-contract-all
```

### Individual Test Suites

```bash
# OpenAPI schema validation
make test-schema

# Pact contract tests
make test-pact

# Generate Pact contracts
make test-pact-generate

# Verify provider against contracts
make test-pact-verify

# Timeline contract tests
go test -v -tags=contract ./internal/api/handlers -run TestTimelineAPIContract
```

### OpenAPI Validation

```bash
# Validate OpenAPI spec
make validate-openapi

# Generate OpenAPI from code
make generate-openapi

# Lint OpenAPI with Spectral
make lint-openapi

# Check for breaking changes
make api-breaking-changes

# All API checks
make api-check
```

## OpenAPI Schema Validation

### What It Tests

- Request/response schemas match OpenAPI specification
- Required fields are present
- Field types are correct
- Headers match specification
- Error responses follow standard format

### Example Test

```go
func TestCategoryEndpointsContract(t *testing.T) {
    suite := NewContractTestSuite(t, "../../docs/api/openapi.yaml")

    t.Run("CreateCategory", func(t *testing.T) {
        body := map[string]interface{}{
            "name":  "Test Category",
            "color": "#FF5733",
        }

        req := createRequest("POST", "/api/v1/categories", body)
        suite.ValidateRequest(t, req)

        // Test response
        resp := makeRequest(req)
        suite.ValidateResponse(t, req, resp)
    })
}
```

### Schema Validation Features

1. **Request Validation**
   - Body schema
   - Query parameters
   - Path parameters
   - Headers

2. **Response Validation**
   - Status codes
   - Body schema
   - Response headers
   - Content types

3. **Error Response Validation**
   - 400 Bad Request
   - 401 Unauthorized
   - 404 Not Found
   - 429 Rate Limit
   - 500 Internal Server Error

## Pact Contract Tests

### What Are Pact Contracts?

Pact contracts define expectations from the consumer's perspective. They specify:
- Request format (method, path, headers, body)
- Expected response (status, headers, body)
- Provider state setup

### Contract Structure

```json
{
  "consumer": {"name": "donelist-mobile-app"},
  "provider": {"name": "donelist-api"},
  "interactions": [
    {
      "description": "create a new category",
      "providerState": "user is authenticated",
      "request": {
        "method": "POST",
        "path": "/api/v1/categories",
        "headers": {
          "Content-Type": "application/json",
          "Authorization": "Bearer TOKEN"
        },
        "body": {
          "name": "Work",
          "color": "#3498db"
        }
      },
      "response": {
        "status": 201,
        "body": {
          "id": "MATCHER:UUID",
          "name": "Work",
          "color": "#3498db",
          "created_at": "MATCHER:ISO8601"
        }
      }
    }
  ]
}
```

### Matchers

Pact uses matchers to allow flexible matching:

- `MATCHER:UUID` - Matches any valid UUID
- `MATCHER:STRING` - Matches any string
- `MATCHER:INTEGER` - Matches any integer
- `MATCHER:ISO8601` - Matches ISO 8601 timestamps
- `MATCHER:ARRAY` - Matches any array
- `MATCHER:OBJECT` - Matches any object

### Generating Contracts

```bash
# Generate all contracts
make test-pact-generate

# Generate specific contract
go test -v -tags=contract ./internal/api -run TestCategoryContractGeneration
```

### Provider Verification

```bash
# Verify provider against all contracts
make test-pact-verify
```

The verification process:
1. Loads contract files from `tests/pacts/`
2. Sets up provider state
3. Makes actual requests to provider
4. Validates responses match contract expectations

### Provider States

Provider states set up the necessary data for testing:

```go
func setupProviderState(t *testing.T, state string, ...) {
    switch state {
    case "user is authenticated":
        // Create test user
        createTestUser(t, userRepo)

    case "user has categories":
        // Create user with categories
        user := createTestUser(t, userRepo)
        createTestCategory(t, categoryRepo, user.ID)

    case "category exists":
        // Create specific category
        createCategoryWithID(t, categoryRepo, knownID)
    }
}
```

## CI/CD Integration

### GitHub Actions Workflow

Contract tests run automatically on:
- Pull requests
- Pushes to main/develop
- Changes to API files

```yaml
name: Contract Tests

on:
  pull_request:
    paths:
      - 'docs/api/openapi.yaml'
      - 'internal/api/**'
      - 'internal/*/handlers/**'

jobs:
  validate-openapi:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Validate OpenAPI Spec
        run: make validate-openapi

  contract-tests:
    runs-on: ubuntu-latest
    needs: validate-openapi
    steps:
      - name: Run contract tests
        run: make test-contract-all

  breaking-changes:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - name: Check for breaking changes
        run: |
          oasdiff breaking base/openapi.yaml pr/openapi.yaml
```

### Breaking Change Detection

On pull requests, the CI automatically:
1. Compares OpenAPI specs (base vs PR)
2. Detects breaking changes
3. Comments on PR with findings
4. Fails if breaking changes found (unless approved)

### PR Checks

All PRs must pass:
- ✅ OpenAPI spec validation
- ✅ Spectral linting
- ✅ Contract tests
- ✅ Schema validation tests
- ✅ Breaking change check

## Best Practices

### 1. Keep Contracts Up-to-Date

```bash
# Regenerate contracts after API changes
make test-pact-generate

# Commit generated contracts
git add tests/pacts/*.json
git commit -m "Update Pact contracts"
```

### 2. Version Your Contracts

```json
{
  "metadata": {
    "pactSpecification": {"version": "2.0.0"},
    "client": {
      "name": "donelist-contract-tests",
      "version": "1.0.0"
    }
  }
}
```

### 3. Test Negative Scenarios

```go
{
    Description: "create category with missing required field",
    Request: PactRequest{
        Method: "POST",
        Path: "/api/v1/categories",
        Body: map[string]interface{}{
            "color": "#3498db",  // Missing required "name"
        },
    },
    Response: PactResponse{
        Status: 400,
        Body: map[string]interface{}{
            "error": "MATCHER:STRING",
        },
    },
}
```

### 4. Use Meaningful Descriptions

```go
{
    Description: "get daily timeline with pagination",
    ProviderState: "user has 50 checkins for date",
    // ...
}
```

### 5. Test Error Responses

Always test:
- 400 Bad Request (validation errors)
- 401 Unauthorized (missing/invalid auth)
- 404 Not Found (resource doesn't exist)
- 429 Rate Limit (too many requests)
- 500 Internal Server Error (server errors)

### 6. Snapshot Testing

For complex responses, use snapshots:

```go
func TestSchemaSnapshotRegression(t *testing.T) {
    snapshot := loadOrCreateSnapshot(t, "response.json", 200)
    validator.ValidateResponse(t, req, snapshot)
}
```

### 7. Document Breaking Changes

Follow the [Breaking Changes Policy](BREAKING_CHANGES.md):
- Deprecate features before removal
- Provide migration guides
- Give adequate notice (6+ months)
- Run versions in parallel

## Troubleshooting

### Common Issues

#### 1. Contract Validation Failures

**Problem**: Provider verification fails with schema mismatch

**Solution**:
```bash
# Check exact error
go test -v -tags=contract ./internal/api -run TestProviderVerification

# Regenerate contracts
make test-pact-generate

# Verify OpenAPI spec matches implementation
make validate-openapi
```

#### 2. Spectral Linting Errors

**Problem**: OpenAPI spec fails Spectral rules

**Solution**:
```bash
# See detailed errors
npx @stoplight/spectral-cli lint docs/api/openapi.yaml --ruleset .spectral.yml

# Fix spec or update .spectral.yml rules
```

#### 3. Breaking Change False Positives

**Problem**: oasdiff reports breaking changes for safe changes

**Solution**:
```bash
# Review specific changes
oasdiff breaking --format json old.yaml new.yaml

# If safe, document why in PR description
# Update .oasdiff.yaml to adjust rules if needed
```

#### 4. Provider State Setup Failures

**Problem**: Provider verification fails due to missing state

**Solution**:
```go
// Ensure all provider states are implemented
func setupProviderState(t *testing.T, state string, ...) {
    switch state {
    case "new-state":  // Add missing state
        setupNewState(t)
    }
}
```

### Debug Mode

Run tests with verbose output:

```bash
# Verbose contract tests
go test -v -tags=contract ./internal/api -run TestContract

# With race detector
go test -v -race -tags=contract ./internal/api

# Specific test
go test -v -tags=contract ./internal/api -run TestCategoryContractGeneration
```

### Logs

Check logs for details:
```bash
# API logs
tail -f logs/api.log

# Test output
go test -v -tags=contract ./internal/api 2>&1 | tee test.log
```

## Resources

### Tools

- [Spectral](https://stoplight.io/open-source/spectral) - OpenAPI linter
- [oasdiff](https://github.com/Tufin/oasdiff) - OpenAPI diff tool
- [Redoc](https://redocly.com/redoc/) - API documentation
- [kin-openapi](https://github.com/getkin/kin-openapi) - Go OpenAPI validator

### Documentation

- [OpenAPI Specification](https://spec.openapis.org/oas/latest.html)
- [Pact Documentation](https://docs.pact.io/)
- [API Breaking Changes](https://www.apievolutionpatterns.com/)
- [Consumer-Driven Contracts](https://martinfowler.com/articles/consumerDrivenContracts.html)

### Related Docs

- [OpenAPI Spec](openapi.yaml)
- [Breaking Changes Policy](BREAKING_CHANGES.md)
- [API Usage Examples](API_USAGE_EXAMPLES.md)
- [Timeline API Testing Guide](TIMELINE_API_TESTING_GUIDE.md)

## Contributing

When making API changes:

1. Update OpenAPI spec or annotations
2. Regenerate spec: `make generate-openapi`
3. Validate spec: `make validate-openapi`
4. Generate contracts: `make test-pact-generate`
5. Run all tests: `make test-contract-all`
6. Check for breaking changes: `make api-breaking-changes`
7. Update documentation
8. Submit PR

## Questions?

- Open an issue on GitHub
- Check existing documentation
- Ask in #api-development Slack channel

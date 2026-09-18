# Contract Testing Quick Reference

## Quick Commands

### Validate API
```bash
make api-check                # Validate and lint OpenAPI spec
make validate-openapi         # Validate spec only
make lint-openapi            # Lint with Spectral only
```

### Generate Contracts
```bash
make test-pact-generate      # Generate all Pact contracts
make generate-openapi        # Generate OpenAPI from code
```

### Run Tests
```bash
make test-contract-all       # All contract tests
make test-schema            # Schema validation only
make test-pact-verify       # Verify provider against contracts
```

### Check Breaking Changes
```bash
make api-breaking-changes    # Compare with previous spec
```

## Contract Test Tags

Run with build tag:
```bash
go test -v -tags=contract ./tests/contract
```

## Pact Contract Structure

```json
{
  "consumer": {"name": "donelist-mobile-app"},
  "provider": {"name": "donelist-api"},
  "interactions": [{
    "description": "operation description",
    "providerState": "required state",
    "request": {
      "method": "POST",
      "path": "/api/v1/resource",
      "body": {...}
    },
    "response": {
      "status": 201,
      "body": {...}
    }
  }]
}
```

## Matchers

- `MATCHER:UUID` - Validates UUID format
- `MATCHER:STRING` - Any string
- `MATCHER:INTEGER` - Any integer
- `MATCHER:ISO8601` - ISO timestamp
- `MATCHER:ARRAY` - Any array
- `MATCHER:OBJECT` - Any object

## Breaking Changes Checklist

### Safe ✅
- [ ] Add optional field
- [ ] Add new endpoint
- [ ] Add optional parameter
- [ ] Relax validation
- [ ] Add enum value

### Breaking ❌
- [ ] Remove field
- [ ] Change field type
- [ ] Rename field
- [ ] Add required field
- [ ] Change URL path
- [ ] Change HTTP method

## CI Workflow

1. **PR Created** → Validate OpenAPI
2. **Run Contract Tests** → Verify compatibility
3. **Detect Breaking Changes** → Comment on PR
4. **Generate Docs** → Update API documentation

## File Locations

```
server/
├── docs/api/
│   ├── openapi.yaml              # OpenAPI spec
│   ├── CONTRACT_TESTING.md       # Full guide
│   ├── BREAKING_CHANGES.md       # Policy
│   └── QUICK_REFERENCE.md        # This file
├── tests/
│   ├── contract/                 # Contract tests
│   │   ├── pact_test.go
│   │   └── schema_validation_test.go
│   ├── pacts/                    # Generated contracts
│   └── snapshots/                # Response snapshots
├── .spectral.yml                 # Linting rules
└── Makefile                      # Test targets
```

## Common Issues

### Spectral Errors
```bash
npx @stoplight/spectral-cli lint docs/api/openapi.yaml --verbose
```

### Contract Mismatch
```bash
go test -v -tags=contract ./tests/contract -run TestProviderVerification
```

### Breaking Changes
```bash
cp docs/api/openapi.yaml docs/api/openapi.yaml.old
# Make changes
make api-breaking-changes
```

## Resources

- [Full Contract Testing Guide](CONTRACT_TESTING.md)
- [Breaking Changes Policy](BREAKING_CHANGES.md)
- [Spectral Documentation](https://stoplight.io/open-source/spectral)
- [Pact Documentation](https://docs.pact.io/)
- [OpenAPI Specification](https://spec.openapis.org/oas/latest.html)

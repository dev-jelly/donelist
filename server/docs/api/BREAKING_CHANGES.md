# API Breaking Changes Policy

## Overview

This document defines what constitutes a breaking change in the DoneList API and establishes guidelines for managing API evolution.

## Breaking Change Definition

A **breaking change** is any modification to the API that could cause existing client applications to fail or behave unexpectedly.

## Breaking Change Categories

### 1. Endpoint Changes

#### Breaking:
- ❌ Removing an endpoint
- ❌ Changing the HTTP method of an endpoint
- ❌ Changing the URL path of an endpoint
- ❌ Adding required path parameters
- ❌ Removing optional path parameters that clients may use

#### Non-Breaking:
- ✅ Adding new endpoints
- ✅ Adding optional path parameters
- ✅ Deprecating endpoints (with notice period)

### 2. Request Changes

#### Breaking:
- ❌ Adding required request body fields
- ❌ Adding required query parameters
- ❌ Adding required headers
- ❌ Changing field types in request body
- ❌ Removing support for content types (e.g., removing JSON support)
- ❌ Making optional fields required
- ❌ Restricting the range/format of existing fields
- ❌ Changing field validation rules to be more strict

#### Non-Breaking:
- ✅ Adding optional request body fields
- ✅ Adding optional query parameters
- ✅ Adding optional headers
- ✅ Making required fields optional
- ✅ Relaxing field validation rules
- ✅ Adding new supported content types

### 3. Response Changes

#### Breaking:
- ❌ Removing fields from response body
- ❌ Changing field types in response body
- ❌ Changing field names in response body
- ❌ Changing response status codes for existing scenarios
- ❌ Removing content type support
- ❌ Changing error response structure
- ❌ Changing the structure of paginated responses

#### Non-Breaking:
- ✅ Adding new fields to response body
- ✅ Adding new response status codes for new scenarios
- ✅ Adding new content type support
- ✅ Adding new optional headers

### 4. Authentication Changes

#### Breaking:
- ❌ Changing authentication mechanism
- ❌ Changing token format
- ❌ Removing authentication methods
- ❌ Changing authorization requirements (making endpoints more restrictive)

#### Non-Breaking:
- ✅ Adding new authentication methods
- ✅ Making endpoints less restrictive
- ✅ Adding new scopes (if backwards compatible)

### 5. Data Type Changes

#### Breaking:
- ❌ Changing field from string to number
- ❌ Changing field from object to array
- ❌ Changing date/time format
- ❌ Changing enum values (removing or renaming)
- ❌ Changing decimal precision

#### Non-Breaking:
- ✅ Adding new enum values (at the end)
- ✅ Increasing string max length
- ✅ Increasing numeric ranges
- ✅ Making types more specific (e.g., string -> UUID string with pattern)

### 6. Behavior Changes

#### Breaking:
- ❌ Changing default values of optional parameters
- ❌ Changing sorting order without new parameter
- ❌ Changing rate limits (making them more restrictive)
- ❌ Changing idempotency behavior
- ❌ Changing caching behavior in a way that affects clients

#### Non-Breaking:
- ✅ Improving performance
- ✅ Adding new error conditions with new error codes
- ✅ Relaxing rate limits

## Versioning Strategy

### Major Version Changes (v1 -> v2)
- Reserved for breaking changes
- Minimum 6-month deprecation notice
- Support previous version for at least 12 months
- Both versions run simultaneously during transition

### Minor Version Changes (v1.0 -> v1.1)
- Backwards-compatible additions
- New features
- New endpoints
- New optional parameters

### Patch Version Changes (v1.0.0 -> v1.0.1)
- Bug fixes
- Security patches
- Documentation updates
- Performance improvements

## Change Process

### 1. Breaking Change Proposal
```markdown
## Breaking Change Proposal

**Endpoint**: `GET /api/v1/categories`
**Type**: Response field removal
**Reason**: Field is deprecated and unused by 99% of clients

**Current Behavior**:
- Response includes `legacy_field` with type string

**Proposed Behavior**:
- Remove `legacy_field` from response

**Impact Assessment**:
- Affects: Mobile app v1.0-v1.5 (5% of users)
- Migration path: Update to mobile app v1.6+
- Timeline: 6-month deprecation period

**Migration Guide**:
1. Update client applications to not depend on `legacy_field`
2. Test with beta endpoint `/api/v2/categories`
3. Deploy updated clients
4. Switch to v2 API
```

### 2. Deprecation Notice
When deprecating features:

1. Add `deprecated: true` to OpenAPI spec
2. Add `X-API-Deprecated` header to responses
3. Update API documentation
4. Send email notifications to API consumers
5. Log deprecation warnings in API

Example header:
```
X-API-Deprecated: This endpoint will be removed in v2. Use /api/v2/categories instead. Removal date: 2025-01-01
```

### 3. Change Rollout

#### Phase 1: Announcement (Month 0)
- Publish breaking change notice
- Update documentation
- Email API consumers
- Add deprecation warnings

#### Phase 2: Beta Release (Month 1)
- Release new version in beta
- Provide migration guide
- Offer migration assistance

#### Phase 3: Parallel Run (Month 2-6)
- Both versions available
- Monitor usage metrics
- Support migration issues

#### Phase 4: Old Version Deprecation (Month 6)
- Set sunset date
- Final migration reminders
- Increased deprecation warnings

#### Phase 5: Old Version Removal (Month 12)
- Remove old version
- Keep documentation archived

## Detection and Prevention

### Automated Detection

We use `oasdiff` to automatically detect breaking changes in CI:

```bash
# Check for breaking changes
oasdiff breaking old-spec.yaml new-spec.yaml
```

Breaking changes will:
- ❌ Fail PR checks
- 📝 Require explicit approval
- 📋 Generate breaking change report
- 🔔 Notify maintainers

### Pre-commit Checks

```bash
# Run before committing API changes
make api-check
```

### Contract Tests

All API changes must pass:
- Pact contract verification
- OpenAPI schema validation
- Backwards compatibility tests

## Examples

### Example 1: Safe Field Addition

**Before**:
```json
{
  "id": "uuid",
  "name": "Category Name"
}
```

**After** (✅ Non-Breaking):
```json
{
  "id": "uuid",
  "name": "Category Name",
  "description": "Optional description"
}
```

### Example 2: Breaking Field Removal

**Before**:
```json
{
  "id": "uuid",
  "name": "Category Name",
  "color": "#FF0000"
}
```

**After** (❌ Breaking):
```json
{
  "id": "uuid",
  "name": "Category Name"
  // color field removed
}
```

**Required Actions**:
1. Deprecate field for 6 months
2. Add deprecation notice
3. Create v2 endpoint without field
4. Migrate clients
5. Remove from v1 after sunset period

### Example 3: Safe Optional Parameter Addition

**Before**:
```
GET /api/v1/categories
```

**After** (✅ Non-Breaking):
```
GET /api/v1/categories?filter=active
```

Optional parameters don't break existing clients.

### Example 4: Breaking Required Parameter Addition

**Before**:
```
GET /api/v1/timeline/daily
```

**After** (❌ Breaking):
```
GET /api/v1/timeline/daily?date=2024-01-15  // date now required
```

**Required Actions**:
1. Keep date optional or provide sensible default
2. If must be required, create v2 endpoint
3. Follow deprecation process

## Guidelines for API Designers

### DO:
✅ Add optional fields instead of required fields
✅ Use feature flags for gradual rollout
✅ Provide default values for new parameters
✅ Add fields at the end of objects
✅ Use semantic versioning
✅ Write comprehensive migration guides
✅ Test with real client applications

### DON'T:
❌ Remove fields without deprecation
❌ Change field types
❌ Make optional fields required
❌ Change URL structures
❌ Modify error response formats
❌ Rush breaking changes
❌ Ignore backwards compatibility

## Testing Requirements

Before merging any API change:

1. **Schema Validation**
   - OpenAPI spec validates
   - No Spectral linting errors
   - Schema snapshot tests pass

2. **Contract Tests**
   - All Pact contracts verified
   - Provider verification passes
   - Consumer expectations met

3. **Backwards Compatibility**
   - No breaking changes detected by oasdiff
   - Or breaking changes explicitly approved
   - Migration path documented

4. **Integration Tests**
   - E2E tests pass
   - Mock client tests pass
   - Performance tests pass

## Monitoring

After deploying API changes:

- Monitor error rates
- Track deprecated endpoint usage
- Analyze client version distribution
- Review support tickets
- Check API metrics dashboard

## Documentation

All API changes require:

1. OpenAPI spec update
2. API documentation update
3. Changelog entry
4. Migration guide (for breaking changes)
5. Example requests/responses
6. Error code documentation

## Contact

For questions about API changes:
- **API Team**: api-team@donelist.com
- **Slack**: #api-changes
- **GitHub**: Open an issue with `api-breaking-change` label

## References

- [Semantic Versioning](https://semver.org/)
- [API Evolution Patterns](https://www.apievolutionpatterns.com/)
- [OpenAPI Breaking Changes](https://www.oasdiff.com/docs/breaking-changes)
- [Consumer-Driven Contracts](https://pact.io/)

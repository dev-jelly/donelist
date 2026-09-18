# CI/CD Pipeline Documentation

Complete documentation for the Donelist API CI/CD pipeline using GitHub Actions.

## Overview

The CI/CD pipeline automates the entire software delivery process from code commit to production deployment, ensuring quality, security, and reliability at every stage.

## Pipeline Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Code Push/PR                              │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Stage 1: Test & Quality                        │
│                                                                   │
│  • Unit Tests (with coverage)                                   │
│  • Integration Tests                                             │
│  • Linters (golangci-lint, staticcheck)                         │
│  • Security Scans (gosec)                                       │
│  • Code Coverage Checks                                          │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Stage 2: Build & Scan                          │
│                                                                   │
│  • Docker Image Build (multi-arch)                              │
│  • Push to GitHub Container Registry                            │
│  • Container Vulnerability Scan (Trivy)                         │
│  • Image Signing (optional)                                      │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Stage 3: Staging Deployment                     │
│                      (develop branch)                            │
│                                                                   │
│  • Deploy to Staging K8s Cluster                                │
│  • Run Database Migrations                                       │
│  • Health Checks                                                 │
│  • Smoke Tests                                                   │
│  • Rollback on Failure                                           │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                 Stage 4: Production Deployment                   │
│                      (main branch)                               │
│                                                                   │
│  • Manual Approval (GitHub Environments)                         │
│  • Blue-Green Deployment                                         │
│  • Database Migration (zero-downtime)                           │
│  • Traffic Switch                                                │
│  • Production Smoke Tests                                        │
│  • Cleanup Old Version                                           │
│  • Notification (Slack/Email)                                    │
└─────────────────────────────────────────────────────────────────┘
```

## Workflow Files

### 1. Main CI/CD Pipeline (.github/workflows/ci-cd.yaml)

**Triggers**:
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop`
- Git tags matching `v*` pattern

**Jobs**:

#### Job 1: Test
```yaml
Runs on: ubuntu-latest
Services: PostgreSQL, Redis
Steps:
  1. Checkout code
  2. Setup Go environment
  3. Cache dependencies
  4. Run unit tests with coverage
  5. Check coverage threshold (70%)
  6. Upload coverage to Codecov
  7. Run integration tests
  8. Run linters (go vet, staticcheck)
  9. Security scan (gosec)
```

**Exit Criteria**:
- All tests pass
- Coverage >= 70%
- No critical security issues
- Linters pass

#### Job 2: Build
```yaml
Depends on: Test
Runs on: ubuntu-latest
Permissions: packages:write
Steps:
  1. Checkout code
  2. Setup Docker Buildx
  3. Login to GitHub Container Registry
  4. Extract metadata (tags, labels)
  5. Build multi-arch image (amd64, arm64)
  6. Push to registry
  7. Run Trivy vulnerability scan
  8. Upload security results
```

**Outputs**:
- `image-tag`: Full image tag
- `image-digest`: Image SHA256 digest

**Image Tags**:
- `main` branch → `latest`
- `develop` branch → `develop`
- PR → `pr-{number}`
- Tag `v1.0.0` → `v1.0.0`, `1.0`
- Commit → `sha-{commit}`

#### Job 3: Deploy Staging
```yaml
Depends on: Build
Runs on: ubuntu-latest
Condition: push to develop branch
Environment: staging
Steps:
  1. Checkout code
  2. Setup kubectl & Helm
  3. Configure kubeconfig
  4. Create namespace
  5. Deploy with Helm
  6. Verify deployment
  7. Run smoke tests
  8. Notify status
```

**Environment Secrets**:
- `KUBECONFIG_STAGING`: Base64-encoded kubeconfig
- `STAGING_DB_PASSWORD`: Database password
- `STAGING_REDIS_PASSWORD`: Redis password
- `STAGING_JWT_SECRET`: JWT signing key

#### Job 4: Deploy Production
```yaml
Depends on: Build
Runs on: ubuntu-latest
Condition: push to main branch
Environment: production (with approval)
Steps:
  1. Checkout code
  2. Setup kubectl & Helm
  3. Configure kubeconfig
  4. Create namespace
  5. Backup current deployment
  6. Blue-Green deployment
  7. Health check new version
  8. Run smoke tests
  9. Switch traffic
  10. Cleanup old version
  11. Verify production
  12. Notify status
```

**Environment Secrets**:
- `KUBECONFIG_PRODUCTION`: Base64-encoded kubeconfig
- `PRODUCTION_DB_PASSWORD`: Database password
- `PRODUCTION_REDIS_PASSWORD`: Redis password
- `PRODUCTION_JWT_SECRET`: JWT signing key

#### Job 5: Rollback
```yaml
Trigger: Manual workflow_dispatch
Runs on: ubuntu-latest
Environment: production
Steps:
  1. Checkout code
  2. Setup kubectl & Helm
  3. Configure kubeconfig
  4. Rollback to previous version
  5. Verify rollback
```

### 2. Additional Workflows

#### Coverage Reporting (.github/workflows/coverage.yml)
- Runs detailed coverage analysis
- Generates coverage reports
- Updates coverage badges
- Triggers on PR and push to main

#### Nightly Tests (.github/workflows/nightly-tests.yml)
- Comprehensive test suite
- Performance benchmarks
- Long-running integration tests
- Runs daily at 2 AM UTC

#### Security Scanning (.github/workflows/security-scan.yml)
- Dependency vulnerability scanning
- SAST (Static Application Security Testing)
- Container image scanning
- License compliance checking
- Runs weekly and on release

## Environment Configuration

### GitHub Environments

#### Staging Environment
```yaml
Name: staging
Protection Rules:
  - None (auto-deploy)
Environment Secrets:
  - KUBECONFIG_STAGING
  - STAGING_DB_PASSWORD
  - STAGING_REDIS_PASSWORD
  - STAGING_JWT_SECRET
Environment Variables:
  - ENVIRONMENT: staging
  - DOMAIN: api-staging.donelist.example.com
```

#### Production Environment
```yaml
Name: production
Protection Rules:
  - Required reviewers: 1 (DevOps team)
  - Wait timer: 0 minutes
  - Deployment branches: main only
Environment Secrets:
  - KUBECONFIG_PRODUCTION
  - PRODUCTION_DB_PASSWORD
  - PRODUCTION_REDIS_PASSWORD
  - PRODUCTION_JWT_SECRET
Environment Variables:
  - ENVIRONMENT: production
  - DOMAIN: api.donelist.example.com
```

### Secrets Setup

```bash
# Generate kubeconfig secret
cat ~/.kube/config | base64 | pbcopy

# Add to GitHub repository:
# Settings → Secrets and variables → Actions → New repository secret

# Required repository secrets:
# - CODECOV_TOKEN (optional, for code coverage)
# - SLACK_WEBHOOK_URL (optional, for notifications)

# Required environment secrets (staging):
# - KUBECONFIG_STAGING
# - STAGING_DB_PASSWORD
# - STAGING_REDIS_PASSWORD
# - STAGING_JWT_SECRET

# Required environment secrets (production):
# - KUBECONFIG_PRODUCTION
# - PRODUCTION_DB_PASSWORD
# - PRODUCTION_REDIS_PASSWORD
# - PRODUCTION_JWT_SECRET
```

## Deployment Strategies

### Rolling Update (Default for Staging)
```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1
    maxUnavailable: 0
```

**Advantages**:
- Zero downtime
- Gradual rollout
- Easy to configure

**Process**:
1. Create new pod with new version
2. Wait for readiness
3. Terminate old pod
4. Repeat until complete

### Blue-Green Deployment (Production)

**Advantages**:
- Instant rollback
- Full testing before traffic switch
- Zero downtime

**Process**:
1. Determine current color (blue/green)
2. Deploy to opposite color
3. Run health checks on new deployment
4. Switch service selector to new color
5. Monitor for 30 seconds
6. Remove old color deployment

**Implementation**:
```bash
# Get current color
CURRENT_COLOR=$(kubectl get deployment -n donelist \
  -l app.kubernetes.io/name=donelist-api \
  -o jsonpath='{.items[0].metadata.labels.color}')

# Deploy to new color
NEW_COLOR="green"
if [ "$CURRENT_COLOR" = "green" ]; then
  NEW_COLOR="blue"
fi

# Deploy
helm upgrade --install donelist-api-$NEW_COLOR ./helm/donelist-api \
  --set deployment.color=$NEW_COLOR \
  --set image.tag=$NEW_TAG

# Switch traffic
kubectl patch service donelist-api -n donelist \
  -p '{"spec":{"selector":{"color":"'$NEW_COLOR'"}}}'

# Cleanup
helm uninstall donelist-api-$CURRENT_COLOR
```

## Health Checks and Validation

### Pre-Deployment Checks
- [ ] All tests pass
- [ ] Code coverage meets threshold
- [ ] No critical security vulnerabilities
- [ ] Docker image builds successfully
- [ ] Image vulnerability scan passes

### Post-Deployment Checks
- [ ] Pods are running
- [ ] Readiness probe passes
- [ ] Liveness probe passes
- [ ] Health endpoint returns 200
- [ ] Database connectivity confirmed
- [ ] Redis connectivity confirmed

### Smoke Tests
```bash
# Health check
curl -f https://api.donelist.example.com/health

# Readiness check
curl -f https://api.donelist.example.com/readyz

# Liveness check
curl -f https://api.donelist.example.com/livez

# Basic API functionality
curl -f https://api.donelist.example.com/api/v1/health
```

## Rollback Procedures

### Automatic Rollback

Triggered when:
- Health checks fail after deployment
- Smoke tests fail
- Readiness probe consistently fails

Helm's `--atomic` flag ensures automatic rollback:
```bash
helm upgrade --install donelist-api ./helm/donelist-api \
  --atomic \
  --cleanup-on-fail
```

### Manual Rollback

#### Via GitHub Actions
1. Go to Actions → CI/CD Pipeline
2. Click "Run workflow"
3. Select "rollback" job
4. Confirm rollback

#### Via Helm
```bash
# View history
helm history donelist-api -n donelist

# Rollback to previous
helm rollback donelist-api -n donelist

# Rollback to specific revision
helm rollback donelist-api 3 -n donelist
```

#### Via Deployment Script
```bash
# Interactive rollback
./scripts/rollback.sh

# Specify revision
REVISION=3 ./scripts/rollback.sh
```

## Monitoring and Notifications

### Deployment Metrics

Tracked metrics:
- Deployment frequency
- Lead time for changes
- Mean time to recovery (MTTR)
- Change failure rate
- Deployment success rate

### Notifications

#### Slack Integration
```yaml
- name: Notify deployment status
  if: always()
  uses: 8398a7/action-slack@v3
  with:
    status: ${{ job.status }}
    webhook_url: ${{ secrets.SLACK_WEBHOOK_URL }}
    text: |
      Deployment to ${{ github.event.inputs.environment }}
      Status: ${{ job.status }}
      Version: ${{ needs.build.outputs.image-tag }}
```

#### Email Notifications
GitHub automatically sends emails to:
- Committer on workflow failure
- Required reviewers for production deployments
- Repository administrators

## Security Best Practices

### 1. Secret Management
- Use GitHub Encrypted Secrets
- Never commit secrets to repository
- Rotate secrets regularly
- Use separate secrets per environment

### 2. Image Security
- Multi-stage builds (minimize attack surface)
- Run as non-root user
- Scan for vulnerabilities with Trivy
- Sign images with cosign (optional)

### 3. Access Control
- Use GitHub Environments with required reviewers
- Implement RBAC in Kubernetes
- Limit kubeconfig permissions
- Use short-lived tokens

### 4. Audit Trail
- All deployments logged
- Git commits tracked
- Approvals recorded
- Deployment artifacts retained

## Performance Optimization

### 1. Build Caching
```yaml
- uses: docker/build-push-action@v5
  with:
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

### 2. Parallel Jobs
- Tests run in parallel with linting
- Multi-arch builds use BuildKit

### 3. Dependency Caching
```yaml
- uses: actions/cache@v4
  with:
    path: |
      ~/go/pkg/mod
      ~/.cache/go-build
    key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
```

## Troubleshooting

### Pipeline Failures

#### Test Failures
```bash
# View test logs in GitHub Actions
# Check specific test output
# Run locally:
cd server
go test -v -race ./...
```

#### Build Failures
```bash
# Check Docker build logs
# Verify Dockerfile syntax
# Test locally:
docker build -t donelist-api:test .
```

#### Deployment Failures
```bash
# Check deployment logs
kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api

# Check pod status
kubectl describe pod <pod-name> -n donelist

# Check events
kubectl get events -n donelist --sort-by='.lastTimestamp'
```

### Common Issues

1. **Image Pull Errors**
   - Verify registry credentials
   - Check image tag exists
   - Ensure imagePullSecrets configured

2. **Health Check Timeouts**
   - Increase initialDelaySeconds
   - Check application startup time
   - Verify health endpoint functionality

3. **Database Migration Failures**
   - Check migration job logs
   - Verify database connectivity
   - Ensure migration files are correct

4. **Rollback Issues**
   - Verify Helm history exists
   - Check revision number
   - Ensure previous version is available

## Best Practices

### 1. Commit Messages
```
feat: add user authentication
fix: resolve database connection issue
chore: update dependencies
docs: improve deployment guide
```

### 2. Pull Requests
- Link to issue/ticket
- Describe changes clearly
- Include test coverage
- Request appropriate reviewers

### 3. Versioning
- Use semantic versioning (v1.2.3)
- Tag releases appropriately
- Maintain CHANGELOG.md

### 4. Testing
- Write tests first (TDD)
- Maintain >70% coverage
- Include integration tests
- Test rollback procedures

## Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Helm Documentation](https://helm.sh/docs/)
- [Kubernetes Deployment Strategies](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
- [Docker Best Practices](https://docs.docker.com/develop/dev-best-practices/)

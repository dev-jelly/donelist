# Donelist API Deployment Guide

Complete deployment documentation for the Donelist API backend.

## Overview

This directory contains all deployment configurations and documentation for deploying the Donelist API to Kubernetes (K3s).

## Directory Structure

```
deploy/
├── k8s/                          # Kubernetes manifests
│   ├── namespace.yaml            # Namespace definition
│   ├── configmap.yaml            # Application configuration
│   ├── secret.yaml               # Sensitive configuration
│   ├── pvc.yaml                  # Persistent volume claims
│   ├── deployment.yaml           # Application deployment + HPA
│   ├── service.yaml              # Service definition
│   ├── ingress.yaml              # Ingress configuration
│   ├── networkpolicy.yaml        # Network policies
│   ├── cert-manager.yaml         # TLS certificate management
│   ├── kustomization.yaml        # Kustomize configuration
│   ├── HELM_DEPLOYMENT.md        # Helm deployment guide
│   ├── K3S_SETUP.md              # K3s setup guide
│   └── DEPLOYMENT.md             # General deployment guide
│
└── README.md                     # This file

server/
├── helm/                         # Helm chart
│   └── donelist-api/
│       ├── Chart.yaml            # Chart metadata
│       ├── values.yaml           # Default values
│       ├── values-staging.yaml   # Staging environment
│       ├── values-production.yaml # Production environment
│       └── templates/            # Helm templates
│
├── scripts/                      # Deployment scripts
│   ├── deploy.sh                 # Deployment automation
│   ├── rollback.sh               # Rollback automation
│   └── test-deployment.sh        # Deployment testing
│
└── .github/workflows/            # CI/CD pipelines
    └── ci-cd.yaml                # Full CI/CD workflow
```

## Quick Start

### Prerequisites

1. **Kubernetes cluster** (K3s recommended for small deployments)
2. **kubectl** (v1.28+)
3. **Helm** (v3.13+)
4. **Docker** (for building images)

### Installation Steps

1. **Set up K3s cluster** (if needed):
   ```bash
   curl -sfL https://get.k3s.io | sh -
   ```

2. **Configure kubectl**:
   ```bash
   mkdir -p ~/.kube
   sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
   sudo chown $USER:$USER ~/.kube/config
   ```

3. **Install required components**:
   ```bash
   # Nginx Ingress
   kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml

   # Cert-Manager
   kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
   ```

4. **Deploy the application**:
   ```bash
   cd server
   ENVIRONMENT=staging IMAGE_TAG=latest ./scripts/deploy.sh
   ```

## Deployment Methods

### Method 1: Helm (Recommended)

**Advantages**:
- Environment-specific configurations
- Easy rollbacks
- Version management
- Template reusability

**Usage**:
```bash
# Staging
helm install donelist-api ./helm/donelist-api \
  --namespace donelist-staging \
  --values ./helm/donelist-api/values-staging.yaml \
  --create-namespace

# Production
helm install donelist-api ./helm/donelist-api \
  --namespace donelist \
  --values ./helm/donelist-api/values-production.yaml \
  --create-namespace
```

See [HELM_DEPLOYMENT.md](k8s/HELM_DEPLOYMENT.md) for detailed instructions.

### Method 2: kubectl + Kustomize

**Advantages**:
- Simple and straightforward
- No additional tools needed
- Direct Kubernetes manifests

**Usage**:
```bash
# Apply with kustomize
kubectl apply -k deploy/k8s/

# Or apply manifests individually
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/secret.yaml
kubectl apply -f deploy/k8s/pvc.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
kubectl apply -f deploy/k8s/ingress.yaml
```

### Method 3: Deployment Scripts

**Advantages**:
- Automated deployment
- Built-in validation
- Health checks included

**Usage**:
```bash
# Deploy
ENVIRONMENT=production \
NAMESPACE=donelist \
IMAGE_TAG=v1.0.0 \
./scripts/deploy.sh

# Test deployment
NAMESPACE=donelist ./scripts/test-deployment.sh

# Rollback if needed
NAMESPACE=donelist ./scripts/rollback.sh
```

## Deployment Environments

### Staging

- **Purpose**: Pre-production testing
- **Namespace**: `donelist-staging`
- **Domain**: `api-staging.donelist.example.com`
- **Resources**: Minimal (1 replica, 250m CPU, 256Mi RAM)
- **Auto-deployment**: On push to `develop` branch

### Production

- **Purpose**: Live production workload
- **Namespace**: `donelist`
- **Domain**: `api.donelist.example.com`
- **Resources**: Full (3-20 replicas, 500m-2000m CPU, 512Mi-1Gi RAM)
- **Auto-deployment**: On push to `main` branch (with approval)

## CI/CD Pipeline

The application uses GitHub Actions for automated CI/CD:

```
Push to develop/main
    ↓
Run Tests (unit, integration, security)
    ↓
Build Docker Image
    ↓
Push to GitHub Container Registry
    ↓
Deploy to Staging (develop) / Production (main)
    ↓
Run Smoke Tests
    ↓
Notify Team
```

See [.github/workflows/ci-cd.yaml](../.github/workflows/ci-cd.yaml) for pipeline details.

## Configuration Management

### Environment Variables

Configured in ConfigMaps:
- Server settings
- Database connection (non-sensitive)
- Redis connection (non-sensitive)
- CORS settings
- Logging configuration

### Secrets

Stored in Kubernetes Secrets:
- Database password
- Redis password
- JWT secret
- API keys

**Creating Secrets**:
```bash
kubectl create secret generic donelist-secret \
  --from-literal=DB_PASSWORD='your-password' \
  --from-literal=REDIS_PASSWORD='your-redis-password' \
  --from-literal=JWT_SECRET='your-jwt-secret-64-chars' \
  -n donelist
```

**Using Sealed Secrets** (recommended for GitOps):
```bash
# Install sealed-secrets
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.24.0/controller.yaml

# Seal your secret
kubectl create secret generic donelist-secret \
  --from-literal=DB_PASSWORD='your-password' \
  --dry-run=client -o yaml | \
  kubeseal -o yaml > sealed-secret.yaml

# Commit and apply sealed-secret.yaml
```

## Monitoring and Health Checks

### Health Endpoints

- **`/health`**: Basic health check
- **`/readyz`**: Readiness probe (checks all dependencies)
- **`/livez`**: Liveness probe (checks if app is alive)
- **`/metrics`**: Prometheus metrics (port 9090)

### Monitoring Setup

```bash
# Install Prometheus + Grafana
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace

# Access Grafana
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
# Username: admin, Password: prom-operator
```

### Viewing Logs

```bash
# Real-time logs
kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api -f

# Logs from all pods
kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api --all-containers

# Previous pod logs (after crash)
kubectl logs -n donelist <pod-name> --previous
```

## Scaling

### Manual Scaling

```bash
# Scale deployment
kubectl scale deployment donelist-api --replicas=5 -n donelist

# Using Helm
helm upgrade donelist-api ./helm/donelist-api \
  --set replicaCount=5 \
  -n donelist
```

### Auto-scaling (HPA)

Horizontal Pod Autoscaler is enabled by default in production:

```bash
# Check HPA status
kubectl get hpa -n donelist

# View HPA details
kubectl describe hpa donelist-api -n donelist

# Modify HPA
helm upgrade donelist-api ./helm/donelist-api \
  --set autoscaling.minReplicas=5 \
  --set autoscaling.maxReplicas=50 \
  -n donelist
```

## Backup and Disaster Recovery

### Database Backups

```bash
# PostgreSQL backup
kubectl exec -n donelist postgres-0 -- \
  pg_dump -U donelist donelist > backup-$(date +%Y%m%d).sql

# Restore
kubectl exec -i -n donelist postgres-0 -- \
  psql -U donelist donelist < backup-20240101.sql
```

### Helm Release Backup

```bash
# List releases
helm list -n donelist

# Get release values
helm get values donelist-api -n donelist > backup-values.yaml

# Get full manifest
helm get manifest donelist-api -n donelist > backup-manifest.yaml
```

### K3s Cluster Backup

```bash
# Backup etcd (K3s does this automatically)
ls /var/lib/rancher/k3s/server/db/snapshots/

# Manual snapshot
sudo k3s etcd-snapshot save --name manual-backup-$(date +%Y%m%d)

# Restore from snapshot
sudo k3s server \
  --cluster-reset \
  --cluster-reset-restore-path=/var/lib/rancher/k3s/server/db/snapshots/snapshot-name
```

## Rollback Procedures

### Helm Rollback

```bash
# View history
helm history donelist-api -n donelist

# Rollback to previous version
helm rollback donelist-api -n donelist

# Rollback to specific revision
helm rollback donelist-api 3 -n donelist

# Using script
NAMESPACE=donelist REVISION=3 ./scripts/rollback.sh
```

### Kubernetes Rollback

```bash
# Rollback deployment
kubectl rollout undo deployment/donelist-api -n donelist

# Rollback to specific revision
kubectl rollout undo deployment/donelist-api --to-revision=2 -n donelist

# Check rollout status
kubectl rollout status deployment/donelist-api -n donelist
```

## Security Best Practices

1. **Use Sealed Secrets or External Secrets Operator**
2. **Enable Network Policies** (included in manifests)
3. **Run containers as non-root** (already configured)
4. **Implement Pod Security Standards**:
   ```bash
   kubectl label namespace donelist pod-security.kubernetes.io/enforce=restricted
   ```
5. **Use TLS for all external communication** (cert-manager configured)
6. **Regular security scans** (Trivy in CI/CD pipeline)
7. **Implement RBAC** for least-privilege access

## Troubleshooting

### Common Issues

1. **Pods not starting**: Check `kubectl describe pod <pod-name> -n donelist`
2. **Image pull errors**: Verify image exists and pull secrets
3. **Database connection issues**: Check secrets and network policies
4. **Health check failures**: Increase `initialDelaySeconds` in probes
5. **Out of memory**: Increase resource limits

### Debug Commands

```bash
# Get all resources
kubectl get all -n donelist

# Check events
kubectl get events -n donelist --sort-by='.lastTimestamp'

# Exec into pod
kubectl exec -it <pod-name> -n donelist -- /bin/sh

# Port forward for testing
kubectl port-forward -n donelist svc/donelist-api 8080:80

# View resource usage
kubectl top nodes
kubectl top pods -n donelist
```

### Testing Deployment

```bash
# Run full test suite
./scripts/test-deployment.sh

# Run specific test
./scripts/test-deployment.sh health_endpoint

# Manual health check
curl http://api.donelist.example.com/health
curl http://api.donelist.example.com/readyz
```

## Performance Tuning

### Resource Optimization

```yaml
# High-traffic production
resources:
  requests:
    cpu: 1000m
    memory: 1Gi
  limits:
    cpu: 4000m
    memory: 2Gi
```

### Connection Pool Tuning

```yaml
env:
  DB_MAX_CONNECTIONS: "100"
  DB_MAX_IDLE_CONNECTIONS: "25"
  DB_CONNECTION_MAX_LIFETIME: "1h"
```

## Documentation

- [Helm Deployment Guide](k8s/HELM_DEPLOYMENT.md) - Detailed Helm instructions
- [K3s Setup Guide](k8s/K3S_SETUP.md) - K3s cluster setup
- [CI/CD Pipeline]../.github/workflows/ci-cd.yaml) - Automated deployment
- [Deployment Scripts](../scripts/) - Automation scripts

## Support

For issues and questions:
1. Check the troubleshooting section
2. Review pod logs: `kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api`
3. Check cluster events: `kubectl get events -n donelist`
4. Run deployment tests: `./scripts/test-deployment.sh`

## License

See project root LICENSE file.

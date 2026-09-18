# Helm Deployment Guide for Donelist API

This guide covers deploying the Donelist API to Kubernetes using Helm charts.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Deployment Environments](#deployment-environments)
- [Configuration](#configuration)
- [Deployment Strategies](#deployment-strategies)
- [Monitoring and Observability](#monitoring-and-observability)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

- **kubectl** >= 1.28.0
- **Helm** >= 3.13.0
- **K3s** or any Kubernetes cluster >= 1.28
- **Docker** (for building images)
- Access to a container registry (GitHub Container Registry recommended)

### Cluster Requirements

- **Minimum Resources**:
  - 2 CPU cores
  - 4GB RAM
  - 50GB storage
- **Recommended for Production**:
  - 8+ CPU cores
  - 16GB+ RAM
  - 200GB+ SSD storage

### Kubernetes Add-ons

1. **Ingress Controller** (nginx-ingress recommended)
2. **Cert-Manager** (for TLS certificates)
3. **Metrics Server** (for HPA)
4. **Prometheus/Grafana** (optional, for monitoring)

## Quick Start

### 1. Install Required Add-ons

```bash
# Install nginx-ingress
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml

# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Install metrics-server
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

### 2. Create Namespace

```bash
kubectl create namespace donelist
```

### 3. Configure Secrets

Create a `secrets.yaml` file (DO NOT commit this):

```yaml
dbPassword: "your-secure-db-password"
redisPassword: "your-secure-redis-password"
jwtSecret: "your-super-secret-jwt-key-minimum-64-characters-long"
```

### 4. Deploy with Helm

#### Staging Environment

```bash
cd server

# Deploy to staging
helm upgrade --install donelist-api ./helm/donelist-api \
  --namespace donelist-staging \
  --create-namespace \
  --values ./helm/donelist-api/values-staging.yaml \
  --set-file secrets=./secrets.yaml \
  --wait \
  --timeout=600s
```

#### Production Environment

```bash
# Deploy to production
helm upgrade --install donelist-api ./helm/donelist-api \
  --namespace donelist \
  --create-namespace \
  --values ./helm/donelist-api/values-production.yaml \
  --set-file secrets=./secrets.yaml \
  --wait \
  --timeout=600s
```

### 5. Verify Deployment

```bash
# Check deployment status
kubectl rollout status deployment/donelist-api -n donelist

# View pods
kubectl get pods -n donelist -l app.kubernetes.io/name=donelist-api

# Check logs
kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api --tail=100

# Test health endpoint
kubectl port-forward -n donelist svc/donelist-api 8080:80
curl http://localhost:8080/health
```

## Deployment Environments

### Staging

- **Namespace**: `donelist-staging`
- **Domain**: `api-staging.donelist.example.com`
- **Replicas**: 1 (no autoscaling)
- **Resources**: Reduced (250m CPU, 256Mi RAM)
- **Purpose**: Testing and validation before production

### Production

- **Namespace**: `donelist`
- **Domain**: `api.donelist.example.com`
- **Replicas**: 3-20 (with HPA)
- **Resources**: Full (500m-2000m CPU, 512Mi-1Gi RAM)
- **Purpose**: Live production workload

## Configuration

### Helm Values Structure

```yaml
# Basic configuration
replicaCount: 2
image:
  repository: ghcr.io/your-org/donelist-api
  tag: "v1.0.0"
  pullPolicy: IfNotPresent

# Resource allocation
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 1Gi

# Autoscaling
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

# Database configuration
env:
  DB_HOST: "postgres-service.donelist.svc.cluster.local"
  DB_PORT: "5432"
  DB_NAME: "donelist"
```

### Environment Variables

All environment variables are configured in the Helm values files:

- **values.yaml**: Default values
- **values-staging.yaml**: Staging-specific overrides
- **values-production.yaml**: Production-specific overrides

### Secrets Management

#### Option 1: Kubernetes Secrets (Basic)

```bash
kubectl create secret generic donelist-secret \
  --from-literal=DB_PASSWORD='your-password' \
  --from-literal=REDIS_PASSWORD='your-password' \
  --from-literal=JWT_SECRET='your-secret' \
  -n donelist
```

#### Option 2: Sealed Secrets (Recommended)

```bash
# Install sealed-secrets controller
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.24.0/controller.yaml

# Create and seal secret
kubectl create secret generic donelist-secret \
  --from-literal=DB_PASSWORD='your-password' \
  --dry-run=client -o yaml | \
  kubeseal -o yaml > sealed-secret.yaml

# Apply sealed secret
kubectl apply -f sealed-secret.yaml -n donelist
```

#### Option 3: External Secrets Operator

```bash
# Install external-secrets
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets \
  external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace

# Configure secret store (AWS Secrets Manager example)
kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secrets
  namespace: donelist
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-west-2
EOF

# Create external secret
kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: donelist-secret
  namespace: donelist
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets
    kind: SecretStore
  target:
    name: donelist-secret
  data:
  - secretKey: DB_PASSWORD
    remoteRef:
      key: donelist/db-password
EOF
```

## Deployment Strategies

### Rolling Update (Default)

Gradual replacement of pods with zero downtime:

```bash
helm upgrade donelist-api ./helm/donelist-api \
  --namespace donelist \
  --values ./helm/donelist-api/values-production.yaml \
  --set image.tag=v1.0.1 \
  --wait
```

### Blue-Green Deployment

Deploy new version alongside old, then switch traffic:

```bash
# Deploy green version
helm install donelist-api-green ./helm/donelist-api \
  --namespace donelist \
  --values ./helm/donelist-api/values-production.yaml \
  --set fullnameOverride=donelist-api-green \
  --set image.tag=v1.0.1

# Test green deployment
kubectl port-forward svc/donelist-api-green 8081:80 -n donelist

# Switch traffic (update ingress or service selector)
kubectl patch service donelist-api -n donelist \
  -p '{"spec":{"selector":{"app.kubernetes.io/instance":"donelist-api-green"}}}'

# Remove blue deployment
helm uninstall donelist-api-blue -n donelist
```

### Canary Deployment

Gradually shift traffic to new version:

```bash
# Deploy canary with 10% traffic
helm install donelist-api-canary ./helm/donelist-api \
  --namespace donelist \
  --values ./helm/donelist-api/values-production.yaml \
  --set fullnameOverride=donelist-api-canary \
  --set image.tag=v1.0.1 \
  --set replicaCount=1

# Monitor metrics and gradually increase traffic
# Use Istio, Linkerd, or nginx-ingress canary annotations

# Promote canary to stable
helm upgrade donelist-api ./helm/donelist-api \
  --namespace donelist \
  --values ./helm/donelist-api/values-production.yaml \
  --set image.tag=v1.0.1

# Remove canary
helm uninstall donelist-api-canary -n donelist
```

## Monitoring and Observability

### Health Checks

The deployment includes three types of health probes:

1. **Liveness Probe**: `/livez` - Restart pod if unhealthy
2. **Readiness Probe**: `/readyz` - Remove from service if not ready
3. **Startup Probe**: `/health` - Allow time for application startup

### Metrics Collection

#### Prometheus Integration

```bash
# Install Prometheus
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring \
  --create-namespace

# Enable ServiceMonitor in Helm values
helm upgrade donelist-api ./helm/donelist-api \
  --set serviceMonitor.enabled=true \
  -n donelist
```

#### View Metrics

```bash
# Port-forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090

# Port-forward to Grafana
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80

# Default Grafana credentials: admin/prom-operator
```

### Logging

#### Access Logs

```bash
# View logs for all pods
kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api --tail=100 -f

# View logs for specific pod
kubectl logs -n donelist <pod-name> -f

# View previous pod logs (after crash)
kubectl logs -n donelist <pod-name> --previous
```

#### Log Aggregation with Loki

```bash
# Install Loki stack
helm repo add grafana https://grafana.github.io/helm-charts
helm install loki grafana/loki-stack \
  -n monitoring \
  --set grafana.enabled=true \
  --set prometheus.enabled=true
```

## Troubleshooting

### Common Issues

#### 1. Pods Not Starting

```bash
# Check pod status
kubectl get pods -n donelist

# Describe pod for events
kubectl describe pod <pod-name> -n donelist

# Check logs
kubectl logs <pod-name> -n donelist
```

Possible causes:
- Image pull errors
- Insufficient resources
- Failed health checks
- Missing secrets

#### 2. Database Connection Issues

```bash
# Test database connectivity
kubectl run -it --rm debug --image=postgres:15-alpine --restart=Never -- \
  psql -h postgres-service.donelist.svc.cluster.local -U donelist -d donelist

# Check database pod
kubectl get pods -n donelist -l app=postgres
kubectl logs -n donelist -l app=postgres
```

#### 3. Health Check Failures

```bash
# Check health endpoints
kubectl exec -n donelist <pod-name> -- wget -O- http://localhost:8080/health
kubectl exec -n donelist <pod-name> -- wget -O- http://localhost:8080/readyz
kubectl exec -n donelist <pod-name> -- wget -O- http://localhost:8080/livez

# Adjust probe settings if needed
helm upgrade donelist-api ./helm/donelist-api \
  --set livenessProbe.initialDelaySeconds=60 \
  --set readinessProbe.failureThreshold=5
```

#### 4. Out of Memory (OOMKilled)

```bash
# Check resource usage
kubectl top pods -n donelist

# Increase memory limits
helm upgrade donelist-api ./helm/donelist-api \
  --set resources.limits.memory=2Gi \
  --set resources.requests.memory=1Gi
```

#### 5. HPA Not Scaling

```bash
# Check HPA status
kubectl get hpa -n donelist
kubectl describe hpa donelist-api -n donelist

# Verify metrics-server
kubectl get apiservice v1beta1.metrics.k8s.io -o yaml

# Check pod metrics
kubectl top pods -n donelist
```

### Debug Commands

```bash
# Get all resources
kubectl get all -n donelist

# Check events
kubectl get events -n donelist --sort-by='.lastTimestamp'

# Exec into pod
kubectl exec -it <pod-name> -n donelist -- /bin/sh

# Port forward for local testing
kubectl port-forward -n donelist svc/donelist-api 8080:80

# Get resource usage
kubectl top nodes
kubectl top pods -n donelist

# Check network policies
kubectl get networkpolicy -n donelist
kubectl describe networkpolicy donelist-api -n donelist
```

### Rollback

```bash
# View deployment history
helm history donelist-api -n donelist

# Rollback to previous version
helm rollback donelist-api -n donelist

# Rollback to specific revision
helm rollback donelist-api 3 -n donelist

# Verify rollback
kubectl rollout status deployment/donelist-api -n donelist
```

## Performance Tuning

### Resource Optimization

```yaml
# For high-traffic production
resources:
  requests:
    cpu: 1000m      # 1 CPU core
    memory: 1Gi
  limits:
    cpu: 4000m      # 4 CPU cores
    memory: 2Gi

# Adjust connection pools
env:
  DB_MAX_CONNECTIONS: "100"
  DB_MAX_IDLE_CONNECTIONS: "25"
```

### Autoscaling Tuning

```yaml
autoscaling:
  minReplicas: 5
  maxReplicas: 50
  targetCPUUtilizationPercentage: 60
  targetMemoryUtilizationPercentage: 70
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
      - type: Percent
        value: 100
        periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
```

## Security Best Practices

1. **Use Sealed Secrets or External Secrets Operator**
2. **Enable Network Policies** to restrict traffic
3. **Run containers as non-root** (already configured)
4. **Enable Pod Security Standards**:
   ```bash
   kubectl label namespace donelist pod-security.kubernetes.io/enforce=restricted
   ```
5. **Regular security scans** with Trivy or Snyk
6. **Use TLS for all external communication**
7. **Implement RBAC** for least-privilege access

## CI/CD Integration

See `.github/workflows/ci-cd.yaml` for automated deployment pipeline that includes:

- Automated testing
- Docker image building
- Staging deployment
- Production deployment with approval
- Rollback procedures

## Support

For issues and questions:
- Check the [troubleshooting section](#troubleshooting)
- Review pod logs: `kubectl logs -n donelist -l app.kubernetes.io/name=donelist-api`
- Check cluster events: `kubectl get events -n donelist`
- Contact the DevOps team

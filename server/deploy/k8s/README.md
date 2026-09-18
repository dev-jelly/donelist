# Kubernetes Deployment Files

This directory contains production-ready Kubernetes manifests for deploying the Donelist API.

## Quick Start

```bash
# Deploy everything
make deploy

# Or manually:
kubectl apply -f namespace.yaml
kubectl apply -f secret.yaml
kubectl apply -f configmap.yaml
kubectl apply -f pvc.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
kubectl apply -f cert-manager.yaml
kubectl apply -f networkpolicy.yaml
kubectl apply -f monitoring.yaml
```

## Files Overview

| File | Description |
|------|-------------|
| `namespace.yaml` | Namespace definition |
| `secret.yaml` | Sensitive configuration (passwords, keys) |
| `sealedsecret.yaml` | Sealed secrets configuration (safe for Git) |
| `encryption-config.yaml` | Encryption at rest configuration |
| `configmap.yaml` | Non-sensitive application configuration |
| `pvc.yaml` | StatefulSets and PersistentVolumeClaims for PostgreSQL/Redis |
| `deployment.yaml` | Main application Deployment and HorizontalPodAutoscaler |
| `service.yaml` | Kubernetes Service for the API |
| `ingress.yaml` | Ingress with TLS configuration |
| `cert-manager.yaml` | TLS certificate management with Let's Encrypt |
| `networkpolicy.yaml` | Network policies for pod-to-pod security |
| `monitoring.yaml` | Prometheus ServiceMonitor and alerting rules |
| `DEPLOYMENT.md` | Comprehensive deployment guide |
| `test-deployment.sh` | Automated deployment testing script |
| `rollback.sh` | Automated rollback script |
| `Makefile` | Convenience commands for deployment operations |

## Key Features

### 🔄 Zero-Downtime Deployments
- Rolling update strategy with `maxUnavailable: 0`
- Health and readiness probes
- PodDisruptionBudget for availability

### 📈 Auto-Scaling
- HorizontalPodAutoscaler based on CPU and memory
- Configurable scaling behavior
- Min 2, Max 10 replicas

### 🔒 Security
- NetworkPolicies for pod isolation
- Sealed secrets for safe Git storage
- Encryption at rest support
- Non-root containers with security contexts
- TLS termination at ingress

### 📊 Monitoring
- Prometheus ServiceMonitor for metrics scraping
- PrometheusRules for alerting
- Grafana dashboard definition
- Comprehensive alerting rules

### 💾 Stateful Services
- StatefulSets for PostgreSQL and Redis
- PersistentVolumeClaims for data persistence
- Backup-friendly configuration

## Common Commands

### Using Make (Recommended)

```bash
make help              # Show all available commands
make deploy            # Deploy everything
make status            # Show deployment status
make logs              # View application logs
make test              # Run deployment tests
make rollback          # Rollback to previous version
make port-forward      # Access application locally
make scale REPLICAS=5  # Scale to 5 replicas
```

### Using kubectl

```bash
# View status
kubectl get all -n donelist
kubectl get pods -n donelist -w

# View logs
kubectl logs -n donelist -l app=donelist,component=api -f

# Scale manually
kubectl scale deployment/donelist-api -n donelist --replicas=5

# Rollback
kubectl rollout undo deployment/donelist-api -n donelist

# Port forward
kubectl port-forward -n donelist svc/donelist-api-service 8080:80
```

## Testing

### Automated Testing

```bash
# Run comprehensive tests
./test-deployment.sh

# Or with make
make test
```

### Manual Testing

```bash
# Port forward to test locally
kubectl port-forward -n donelist svc/donelist-api-service 8080:80

# Test endpoints
curl http://localhost:8080/healthz        # Liveness probe
curl http://localhost:8080/readyz         # Readiness probe
curl http://localhost:8080/healthz/detail # Detailed health info
curl http://localhost:8080/metrics
```

## Rollback

### Automated Rollback

```bash
# Rollback to previous version
./rollback.sh

# Or with make
make rollback

# Rollback to specific revision
./rollback.sh --revision 3
make rollback-revision REVISION=3
```

### Manual Rollback

```bash
# View rollout history
kubectl rollout history deployment/donelist-api -n donelist

# Rollback to previous
kubectl rollout undo deployment/donelist-api -n donelist

# Rollback to specific revision
kubectl rollout undo deployment/donelist-api -n donelist --to-revision=3
```

## Configuration

### Update Secrets

**Option 1: Regular Kubernetes Secrets (not recommended for Git)**

```bash
# Edit values in secret.yaml
# Apply changes
kubectl apply -f secret.yaml
kubectl rollout restart deployment/donelist-api -n donelist
```

**Option 2: Sealed Secrets (recommended)**

```bash
# Create plain secret (DO NOT commit)
cat > /tmp/secret.yaml <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: donelist-secret
  namespace: donelist
type: Opaque
stringData:
  DB_PASSWORD: "your-password"
  REDIS_PASSWORD: "your-password"
  JWT_SECRET: "your-jwt-secret"
EOF

# Seal it
kubeseal -f /tmp/secret.yaml -w sealedsecret-generated.yaml

# Apply
kubectl apply -f sealedsecret-generated.yaml

# Clean up
rm /tmp/secret.yaml

# Restart to use new secrets
kubectl rollout restart deployment/donelist-api -n donelist
```

### Update ConfigMap

```bash
# Edit configmap.yaml
vim configmap.yaml

# Apply changes
kubectl apply -f configmap.yaml

# Restart to pick up changes
kubectl rollout restart deployment/donelist-api -n donelist
```

### Update Image

```bash
# Update image tag
kubectl set image deployment/donelist-api \
  donelist-api=your-registry/donelist-api:v1.2.0 \
  -n donelist

# Watch rollout
kubectl rollout status deployment/donelist-api -n donelist
```

## Monitoring

### View Metrics

```bash
# Port forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Port forward to Grafana
kubectl port-forward -n monitoring svc/grafana 3000:3000
```

### View Alerts

```bash
# Check PrometheusRules
kubectl get prometheusrule -n donelist

# Describe alert rules
kubectl describe prometheusrule donelist-api-alerts -n donelist
```

### Resource Usage

```bash
# View resource usage
kubectl top pods -n donelist
kubectl top nodes

# Or with make
make top
```

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl get pods -n donelist

# Describe pod
kubectl describe pod -n donelist <pod-name>

# View logs
kubectl logs -n donelist <pod-name>

# View previous logs if crashed
kubectl logs -n donelist <pod-name> --previous
```

### Database Connection Issues

```bash
# Test DNS resolution
kubectl exec -it -n donelist deployment/donelist-api -- nslookup postgres-service

# Test connectivity
kubectl exec -it -n donelist deployment/donelist-api -- nc -zv postgres-service 5432

# Check database logs
kubectl logs -n donelist statefulset/postgres
```

### Certificate Issues

```bash
# Check certificate status
kubectl get certificate -n donelist
kubectl describe certificate donelist-api-cert -n donelist

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager
```

### HPA Not Scaling

```bash
# Check HPA status
kubectl get hpa -n donelist
kubectl describe hpa donelist-api-hpa -n donelist

# Verify metrics server
kubectl get pods -n kube-system -l k8s-app=metrics-server

# Check metrics
kubectl top pods -n donelist
```

## Prerequisites

### Required

- Kubernetes 1.24+ cluster
- kubectl configured
- Ingress controller (nginx recommended)
- Metrics server (for HPA)
- Storage provisioner (for PVCs)

### Optional

- cert-manager (for TLS certificates)
- Prometheus operator (for monitoring)
- sealed-secrets controller (for secret management)

### Installation

```bash
# Nginx Ingress
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml

# Metrics Server
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Sealed Secrets
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.24.0/controller.yaml

# Prometheus Operator
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml
```

## Environment-Specific Configurations

### Development

```yaml
# Lower resource limits
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 200m
    memory: 256Mi

# Fewer replicas
replicas: 1

# Self-signed certificates
cert-manager.io/cluster-issuer: "selfsigned-issuer"
```

### Staging

```yaml
# Moderate resources
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

# Standard replicas
replicas: 2

# Let's Encrypt staging
cert-manager.io/cluster-issuer: "letsencrypt-staging"
```

### Production

```yaml
# Full resources
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi

# High availability
replicas: 3

# Let's Encrypt production
cert-manager.io/cluster-issuer: "letsencrypt-prod"
```

## Security Best Practices

1. ✅ Use NetworkPolicies to restrict pod communication
2. ✅ Use sealed-secrets or external secret managers
3. ✅ Enable encryption at rest for etcd
4. ✅ Run containers as non-root
5. ✅ Use read-only root filesystems
6. ✅ Drop all capabilities
7. ✅ Use TLS for all external traffic
8. ✅ Regularly rotate secrets
9. ✅ Enable audit logging
10. ✅ Use Pod Security Standards

## Support

For detailed documentation, see [DEPLOYMENT.md](./DEPLOYMENT.md)

For issues or questions:
- Check logs: `kubectl logs -n donelist -l app=donelist`
- Review events: `kubectl get events -n donelist`
- Run tests: `./test-deployment.sh`

# Kubernetes Deployment Guide for Donelist API

This guide provides comprehensive instructions for deploying the Donelist API to Kubernetes with production-ready configurations.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Detailed Deployment Steps](#detailed-deployment-steps)
- [Configuration](#configuration)
- [Testing and Validation](#testing-and-validation)
- [Rollback Procedures](#rollback-procedures)
- [Monitoring and Observability](#monitoring-and-observability)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

```bash
# Kubernetes CLI
kubectl version --client

# Helm (optional, for package management)
helm version

# kubeseal (for sealed-secrets)
kubeseal --version

# cert-manager CLI (optional)
cmctl version
```

### Cluster Requirements

- Kubernetes 1.24+
- Ingress controller (nginx recommended)
- Metrics server for HPA
- Storage provisioner for PVCs
- (Optional) Prometheus operator for monitoring
- (Optional) cert-manager for TLS certificates

### Install Prerequisites

```bash
# Install nginx-ingress controller
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml

# Install metrics-server (if not already installed)
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Install Prometheus operator (optional, for monitoring)
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml

# Install sealed-secrets controller
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.24.0/controller.yaml
```

## Quick Start

For a quick deployment (development/testing):

```bash
# 1. Create namespace
kubectl apply -f deploy/k8s/namespace.yaml

# 2. Create secrets (update with your values first!)
kubectl apply -f deploy/k8s/secret.yaml

# 3. Create configmap
kubectl apply -f deploy/k8s/configmap.yaml

# 4. Deploy stateful services (PostgreSQL, Redis)
kubectl apply -f deploy/k8s/pvc.yaml

# 5. Deploy the application
kubectl apply -f deploy/k8s/deployment.yaml

# 6. Create service
kubectl apply -f deploy/k8s/service.yaml

# 7. Deploy ingress with TLS
kubectl apply -f deploy/k8s/cert-manager.yaml
kubectl apply -f deploy/k8s/ingress.yaml

# 8. Apply network policies
kubectl apply -f deploy/k8s/networkpolicy.yaml

# 9. Set up monitoring
kubectl apply -f deploy/k8s/monitoring.yaml

# Check deployment status
kubectl get pods -n donelist -w
```

## Detailed Deployment Steps

### 1. Namespace Setup

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl get namespace donelist
```

### 2. Secrets Management

#### Option A: Basic Kubernetes Secrets (Development)

```bash
# Edit the secret file with your values
# IMPORTANT: Never commit actual secrets to git!
kubectl apply -f deploy/k8s/secret.yaml
```

#### Option B: Sealed Secrets (Recommended for Git)

```bash
# Create a plain secret file (do NOT commit this)
cat <<EOF > /tmp/donelist-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: donelist-secret
  namespace: donelist
type: Opaque
stringData:
  DB_PASSWORD: "your-secure-db-password"
  REDIS_PASSWORD: "your-secure-redis-password"
  JWT_SECRET: "your-super-secret-jwt-key-minimum-32-characters"
EOF

# Seal the secret
kubeseal -f /tmp/donelist-secret.yaml -w deploy/k8s/sealedsecret-generated.yaml

# Apply the sealed secret
kubectl apply -f deploy/k8s/sealedsecret-generated.yaml

# Clean up plain secret
rm /tmp/donelist-secret.yaml
```

#### Option C: External Secrets Operator (Production)

See `deploy/k8s/sealedsecret.yaml` for AWS Secrets Manager, Google Secret Manager, or HashiCorp Vault integration examples.

### 3. Configuration

```bash
# Review and update the ConfigMap with your settings
kubectl apply -f deploy/k8s/configmap.yaml

# Verify
kubectl get configmap donelist-config -n donelist -o yaml
```

### 4. Persistent Storage

```bash
# Apply PVCs and StatefulSets for PostgreSQL and Redis
kubectl apply -f deploy/k8s/pvc.yaml

# Wait for StatefulSets to be ready
kubectl rollout status statefulset/postgres -n donelist
kubectl rollout status statefulset/redis -n donelist

# Verify PVCs are bound
kubectl get pvc -n donelist
```

### 5. Application Deployment

```bash
# Deploy the Donelist API
kubectl apply -f deploy/k8s/deployment.yaml

# Watch the rollout
kubectl rollout status deployment/donelist-api -n donelist

# Check pods
kubectl get pods -n donelist -l app=donelist,component=api
```

### 6. Service and Ingress

```bash
# Create the service
kubectl apply -f deploy/k8s/service.yaml

# Verify service
kubectl get svc -n donelist

# Set up cert-manager issuers
kubectl apply -f deploy/k8s/cert-manager.yaml

# Wait for certificate to be ready
kubectl get certificate -n donelist -w

# Deploy ingress
kubectl apply -f deploy/k8s/ingress.yaml

# Get ingress details
kubectl get ingress -n donelist
kubectl describe ingress donelist-api-ingress -n donelist
```

### 7. Network Policies

```bash
# Apply network policies for security
kubectl apply -f deploy/k8s/networkpolicy.yaml

# Verify
kubectl get networkpolicy -n donelist
```

### 8. Monitoring Setup

```bash
# Apply ServiceMonitor and PrometheusRules
kubectl apply -f deploy/k8s/monitoring.yaml

# Verify ServiceMonitor
kubectl get servicemonitor -n donelist

# Verify PrometheusRules
kubectl get prometheusrule -n donelist
```

## Configuration

### Environment-Specific Settings

#### Development

```bash
kubectl set env deployment/donelist-api SERVER_ENV=development -n donelist
kubectl set env deployment/donelist-api LOG_LEVEL=debug -n donelist
```

#### Staging

```bash
kubectl set env deployment/donelist-api SERVER_ENV=staging -n donelist
kubectl set env deployment/donelist-api LOG_LEVEL=info -n donelist
```

#### Production

```bash
kubectl set env deployment/donelist-api SERVER_ENV=production -n donelist
kubectl set env deployment/donelist-api LOG_LEVEL=info -n donelist
```

### Scaling Configuration

```bash
# Manually scale replicas
kubectl scale deployment/donelist-api --replicas=5 -n donelist

# Update HPA limits
kubectl patch hpa donelist-api-hpa -n donelist -p '{"spec":{"maxReplicas":20}}'

# View HPA status
kubectl get hpa -n donelist -w
```

## Testing and Validation

### 1. Health Checks

```bash
# Port-forward to test locally
kubectl port-forward -n donelist svc/donelist-api-service 8080:80

# Test health endpoint
curl http://localhost:8080/health

# Test readiness endpoint
curl http://localhost:8080/ready

# Test metrics endpoint
curl http://localhost:8080/metrics
```

### 2. Database Connection

```bash
# Get a shell in the API pod
kubectl exec -it -n donelist deployment/donelist-api -- /bin/sh

# Test database connection
psql -h postgres-service -U donelist -d donelist

# Test Redis connection
redis-cli -h redis-service
```

### 3. Load Testing

```bash
# Install k6 or use your preferred tool
k6 run --vus 10 --duration 30s loadtest.js

# Monitor HPA during load test
kubectl get hpa -n donelist -w
```

### 4. Network Policy Testing

```bash
# Test that external pods cannot connect
kubectl run test-pod --image=alpine --rm -it -- sh
# Try: nc -zv donelist-api-service.donelist 80
# Should fail if network policies are working correctly

# Test from an allowed pod
kubectl exec -it -n donelist deployment/donelist-api -- sh
# This should work
```

### 5. TLS Certificate Verification

```bash
# Check certificate status
kubectl get certificate -n donelist

# Describe certificate
kubectl describe certificate donelist-api-cert -n donelist

# Test HTTPS endpoint
curl -v https://api.donelist.example.com/health
```

## Rollback Procedures

### Quick Rollback

```bash
# Rollback to previous deployment
kubectl rollout undo deployment/donelist-api -n donelist

# Rollback to specific revision
kubectl rollout history deployment/donelist-api -n donelist
kubectl rollout undo deployment/donelist-api --to-revision=3 -n donelist

# Watch rollback progress
kubectl rollout status deployment/donelist-api -n donelist
```

### Manual Rollback

```bash
# Scale down current version
kubectl scale deployment/donelist-api --replicas=0 -n donelist

# Update image to previous version
kubectl set image deployment/donelist-api donelist-api=your-registry/donelist-api:v1.0.0 -n donelist

# Scale back up
kubectl scale deployment/donelist-api --replicas=2 -n donelist
```

### Database Migration Rollback

```bash
# Get a shell in a pod
kubectl exec -it -n donelist deployment/donelist-api -- /bin/sh

# Run migration rollback (if your migration tool supports it)
# Example with golang-migrate:
# migrate -path /migrations -database "postgres://..." down 1
```

## Monitoring and Observability

### Logs

```bash
# View logs from all API pods
kubectl logs -n donelist -l app=donelist,component=api --tail=100 -f

# View logs from specific pod
kubectl logs -n donelist donelist-api-xxxxx-yyyyy --tail=100 -f

# View previous container logs (if crashed)
kubectl logs -n donelist donelist-api-xxxxx-yyyyy --previous
```

### Metrics

```bash
# Port-forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Access Prometheus UI at http://localhost:9090

# Port-forward to Grafana
kubectl port-forward -n monitoring svc/grafana 3000:3000

# Access Grafana at http://localhost:3000
```

### Events

```bash
# View events in the namespace
kubectl get events -n donelist --sort-by='.lastTimestamp'

# Watch events in real-time
kubectl get events -n donelist --watch
```

### Resource Usage

```bash
# View resource usage
kubectl top pods -n donelist
kubectl top nodes

# View detailed resource allocation
kubectl describe node <node-name>
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl get pods -n donelist

# Describe pod for events
kubectl describe pod -n donelist <pod-name>

# Check logs
kubectl logs -n donelist <pod-name>

# Check previous logs if crashed
kubectl logs -n donelist <pod-name> --previous
```

### Database Connection Issues

```bash
# Test DNS resolution
kubectl exec -it -n donelist deployment/donelist-api -- nslookup postgres-service

# Test network connectivity
kubectl exec -it -n donelist deployment/donelist-api -- nc -zv postgres-service 5432

# Check database pod logs
kubectl logs -n donelist statefulset/postgres
```

### Certificate Issues

```bash
# Check certificate status
kubectl get certificate -n donelist
kubectl describe certificate donelist-api-cert -n donelist

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager

# Check certificate request
kubectl get certificaterequest -n donelist
kubectl describe certificaterequest -n donelist <cr-name>
```

### HPA Not Scaling

```bash
# Check HPA status
kubectl get hpa -n donelist
kubectl describe hpa donelist-api-hpa -n donelist

# Verify metrics-server is running
kubectl get pods -n kube-system -l k8s-app=metrics-server

# Check pod metrics
kubectl top pods -n donelist
```

### Network Policy Issues

```bash
# List all network policies
kubectl get networkpolicy -n donelist

# Describe network policy
kubectl describe networkpolicy -n donelist <policy-name>

# Temporarily disable for testing (not recommended in production)
kubectl delete networkpolicy -n donelist <policy-name>
```

### Performance Issues

```bash
# Check resource limits
kubectl describe pod -n donelist <pod-name> | grep -A 5 Limits

# Increase resources if needed
kubectl set resources deployment/donelist-api -n donelist \
  --limits=cpu=1000m,memory=1Gi \
  --requests=cpu=500m,memory=512Mi

# Check for throttling
kubectl describe pod -n donelist <pod-name> | grep -i throttl
```

## Zero-Downtime Deployment Strategy

The deployment uses a rolling update strategy with the following configuration:

- `maxSurge: 1` - One extra pod can be created during update
- `maxUnavailable: 0` - No pods will be unavailable during update
- Readiness probes ensure traffic only goes to ready pods
- PodDisruptionBudget ensures minimum availability

### Deployment Best Practices

1. **Always test in staging first**
2. **Monitor metrics during rollout**
3. **Use feature flags for risky changes**
4. **Have rollback plan ready**
5. **Communicate with team before production deployments**

## Additional Resources

- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [Nginx Ingress Documentation](https://kubernetes.github.io/ingress-nginx/)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [Prometheus Operator Documentation](https://prometheus-operator.dev/)
- [Sealed Secrets Documentation](https://github.com/bitnami-labs/sealed-secrets)

## Support

For issues or questions:
- Check application logs: `kubectl logs -n donelist -l app=donelist --tail=100`
- Review events: `kubectl get events -n donelist`
- Contact DevOps team: devops@donelist.example.com

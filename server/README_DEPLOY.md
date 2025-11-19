# Donelist API Deployment Guide

This guide provides comprehensive instructions for deploying the Donelist API server using different deployment methods.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Configuration](#configuration)
- [Deployment Methods](#deployment-methods)
  - [Docker Standalone](#docker-standalone)
  - [Docker Compose](#docker-compose)
  - [Kubernetes/K3s](#kubernetesk3s)
- [Database Migrations](#database-migrations)
- [Health Checks](#health-checks)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)
- [Security Considerations](#security-considerations)

## Prerequisites

### General Requirements

- Go 1.21+ (for local development)
- Docker 20.10+ (for containerized deployment)
- kubectl 1.24+ (for Kubernetes deployment)
- PostgreSQL 15+ (can be containerized)
- Redis 7+ (can be containerized)

### K3s/Kubernetes Requirements

- K3s 1.24+ or Kubernetes 1.24+
- Nginx Ingress Controller
- cert-manager (optional, for automatic TLS)
- Metrics Server (for HPA)

## Configuration

### Environment Variables

Copy `.env.example` to `.env` and configure all required variables:

```bash
cp .env.example .env
```

Key configuration areas:

#### Server Configuration
```bash
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_ENV=production  # development, staging, production
```

#### Database Configuration
```bash
DB_HOST=localhost  # Or postgres service name in Docker
DB_PORT=5432
DB_NAME=donelist
DB_USER=donelist
DB_PASSWORD=your_secure_password  # CHANGE THIS!
DB_SSLMODE=disable  # require for production
```

#### Redis Configuration
```bash
REDIS_HOST=localhost  # Or redis service name in Docker
REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password  # Optional but recommended
REDIS_DB=0
```

#### JWT Configuration
```bash
JWT_SECRET=your-super-secret-jwt-key-minimum-32-chars  # CHANGE THIS!
JWT_ACCESS_TOKEN_EXPIRY=15m
JWT_REFRESH_TOKEN_EXPIRY=7d
```

#### Security Configuration
```bash
CORS_ALLOWED_ORIGINS=https://donelist.example.com
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS,PATCH
CORS_ALLOWED_HEADERS=Accept,Authorization,Content-Type,X-CSRF-Token
CORS_ALLOW_CREDENTIALS=true
```

## Deployment Methods

### Docker Standalone

#### 1. Build the Docker Image

```bash
# Production build
docker build -t donelist-api:latest .

# Development build with hot reload
docker build --target builder -t donelist-api:dev .
```

#### 2. Run PostgreSQL and Redis

```bash
# PostgreSQL
docker run -d \
  --name donelist-postgres \
  -e POSTGRES_DB=donelist \
  -e POSTGRES_USER=donelist \
  -e POSTGRES_PASSWORD=your_password \
  -p 5432:5432 \
  -v postgres_data:/var/lib/postgresql/data \
  postgres:15-alpine

# Redis
docker run -d \
  --name donelist-redis \
  -p 6379:6379 \
  -v redis_data:/data \
  redis:7-alpine redis-server --appendonly yes
```

#### 3. Run Database Migrations

```bash
# Using migrate CLI
migrate -database "postgres://donelist:password@localhost:5432/donelist?sslmode=disable" \
  -path migrations up

# Or using Docker
docker run --rm \
  --network host \
  -v $(pwd)/migrations:/migrations \
  migrate/migrate \
  -path=/migrations \
  -database "postgres://donelist:password@localhost:5432/donelist?sslmode=disable" up
```

#### 4. Run the API Server

```bash
docker run -d \
  --name donelist-api \
  --network host \
  --env-file .env \
  -p 8080:8080 \
  donelist-api:latest
```

### Docker Compose

#### 1. Start All Services

```bash
# Development mode (with volumes for hot reload)
docker-compose up -d

# Production mode (build and run)
docker-compose -f docker-compose.yml up -d --build

# View logs
docker-compose logs -f api
```

#### 2. Run Migrations (if not auto-migrated)

```bash
docker-compose run --rm migrate
```

#### 3. Stop Services

```bash
docker-compose down

# Remove volumes (WARNING: deletes data)
docker-compose down -v
```

### Kubernetes/K3s

#### 1. Create Namespace

```bash
kubectl apply -f deploy/k8s/namespace.yaml
```

#### 2. Configure Secrets

Edit `deploy/k8s/secret.yaml` and encode your actual values:

```bash
# Encode values
echo -n "your-password" | base64
echo -n "your-jwt-secret" | base64

# Apply secret
kubectl apply -f deploy/k8s/secret.yaml
```

#### 3. Deploy Database and Redis (if not external)

```bash
# Install PostgreSQL via Helm
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install postgres bitnami/postgresql \
  --namespace donelist \
  --set auth.username=donelist \
  --set auth.password=your_password \
  --set auth.database=donelist

# Install Redis via Helm
helm install redis bitnami/redis \
  --namespace donelist \
  --set auth.password=your_redis_password
```

#### 4. Apply Configuration

```bash
# ConfigMap for non-sensitive config
kubectl apply -f deploy/k8s/configmap.yaml
```

#### 5. Deploy the Application

```bash
# Deploy the API
kubectl apply -f deploy/k8s/deployment.yaml

# Create service
kubectl apply -f deploy/k8s/service.yaml

# Configure ingress (update domain first)
kubectl apply -f deploy/k8s/ingress.yaml
```

#### 6. Verify Deployment

```bash
# Check pod status
kubectl get pods -n donelist

# Check service
kubectl get svc -n donelist

# Check ingress
kubectl get ingress -n donelist

# View logs
kubectl logs -f deployment/donelist-api -n donelist

# Check HPA status
kubectl get hpa -n donelist
```

#### 7. Access the Application

```bash
# Port forward for local testing
kubectl port-forward -n donelist svc/donelist-api-service 8080:80

# Or access via ingress URL
curl https://api.donelist.example.com/health
```

## Database Migrations

### Using golang-migrate

```bash
# Install migrate CLI
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Create new migration
migrate create -ext sql -dir migrations -seq create_new_table

# Run migrations up
migrate -database "$DATABASE_URL" -path migrations up

# Run migrations down
migrate -database "$DATABASE_URL" -path migrations down 1

# Force version
migrate -database "$DATABASE_URL" -path migrations force VERSION
```

### Migration Best Practices

1. Always test migrations on a copy of production data
2. Create rollback migrations for every change
3. Use transactions for DDL operations when possible
4. Version control all migration files
5. Never edit existing migrations after deployment

## Health Checks

The API provides health check endpoints:

### Liveness Check
```bash
GET /health

# Response
{
  "status": "healthy",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Readiness Check
```bash
GET /ready

# Response
{
  "status": "ready",
  "database": "connected",
  "redis": "connected",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Metrics Endpoint
```bash
GET /metrics

# Prometheus-formatted metrics
# TYPE http_requests_total counter
# HELP http_requests_total Total HTTP requests
http_requests_total{method="GET",path="/api/v1/checkins",status="200"} 42
```

## Monitoring

### Prometheus Integration

1. Configure Prometheus to scrape metrics:

```yaml
scrape_configs:
  - job_name: 'donelist-api'
    static_configs:
      - targets: ['donelist-api-service:9090']
    metrics_path: '/metrics'
```

2. Key metrics to monitor:
   - Request rate and latency
   - Error rate (4xx, 5xx)
   - Database connection pool metrics
   - WebSocket connection count
   - JWT token operations

### Logging

Configure log aggregation (ELK, Loki, etc.):

```bash
# JSON logs for structured logging
LOG_FORMAT=json
LOG_LEVEL=info  # debug, info, warn, error
LOG_OUTPUT=stdout
```

### Alerts

Example alert rules:

```yaml
groups:
  - name: donelist
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        annotations:
          summary: High error rate detected

      - alert: DatabaseConnectionFailure
        expr: up{job="donelist-api"} == 0
        for: 1m
        annotations:
          summary: Database connection lost
```

## Troubleshooting

### Common Issues

#### 1. Database Connection Failed

```bash
# Check database connectivity
psql -h DB_HOST -U DB_USER -d DB_NAME

# Check from container
docker exec -it donelist-api sh
nc -zv postgres-host 5432
```

#### 2. Redis Connection Failed

```bash
# Test Redis connection
redis-cli -h REDIS_HOST ping

# Check from container
docker exec -it donelist-api sh
nc -zv redis-host 6379
```

#### 3. Pod Crashes in Kubernetes

```bash
# Check pod logs
kubectl logs -p deployment/donelist-api -n donelist

# Describe pod for events
kubectl describe pod <pod-name> -n donelist

# Check resource limits
kubectl top pod -n donelist
```

#### 4. WebSocket Connection Issues

- Ensure ingress/proxy supports WebSocket upgrade
- Check timeout settings (should be > 60s)
- Verify session affinity is enabled

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug
SERVER_ENV=development
```

## Security Considerations

### Production Checklist

- [ ] Change all default passwords
- [ ] Use strong JWT secret (32+ characters)
- [ ] Enable TLS/HTTPS
- [ ] Configure CORS properly
- [ ] Enable rate limiting
- [ ] Use read-only root filesystem
- [ ] Run containers as non-root user
- [ ] Set resource limits
- [ ] Enable database SSL
- [ ] Rotate secrets regularly
- [ ] Configure network policies
- [ ] Enable audit logging
- [ ] Set up backup strategy

### Secret Management

Consider using:
- Kubernetes Secrets with encryption at rest
- HashiCorp Vault
- AWS Secrets Manager
- Azure Key Vault
- Google Secret Manager

### Network Security

```yaml
# Example NetworkPolicy
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: donelist-api-netpol
  namespace: donelist
spec:
  podSelector:
    matchLabels:
      app: donelist
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
    ports:
    - protocol: TCP
      port: 8080
```

## Backup and Recovery

### Database Backup

```bash
# Backup
pg_dump -h localhost -U donelist -d donelist > backup.sql

# Restore
psql -h localhost -U donelist -d donelist < backup.sql

# Kubernetes CronJob for automated backups
kubectl apply -f deploy/k8s/backup-cronjob.yaml
```

### Redis Backup

```bash
# Save snapshot
redis-cli BGSAVE

# Copy dump.rdb file
docker cp donelist-redis:/data/dump.rdb ./redis-backup.rdb
```

## Scaling

### Horizontal Scaling

The deployment includes HPA configuration:

```yaml
minReplicas: 2
maxReplicas: 10
targetCPUUtilizationPercentage: 70
targetMemoryUtilizationPercentage: 80
```

### Database Scaling

Consider:
- Read replicas for read-heavy workloads
- Connection pooling optimization
- Database partitioning for large datasets
- Caching strategy with Redis

## Support

For issues or questions:
1. Check the [API documentation](./README.md)
2. Review application logs
3. Check health endpoints
4. Create an issue on GitHub

## License

See LICENSE file for details.
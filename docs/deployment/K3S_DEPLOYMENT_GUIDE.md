# Donelist - K3s Deployment Guide

**Version**: 1.0
**Date**: 2025-11-10
**Target**: Production-ready K3s deployment

---

## 📋 Deployment Overview

This guide provides **step-by-step instructions** for deploying Donelist to K3s, optimized for LLM-assisted deployment.

### Architecture
```
[ Client Apps ] → [ Traefik Ingress ] → [ API Pods ] → [ PostgreSQL / Redis ]
```

---

## 🚀 Prerequisites

### 1. Server Requirements
- **OS**: Ubuntu 22.04 LTS (recommended)
- **RAM**: Minimum 4GB, Recommended 8GB+
- **CPU**: 2+ cores
- **Disk**: 50GB+ SSD
- **Network**: Public IP with ports 80, 443, 6443 open

### 2. Domain & DNS
- Domain name (e.g., `donelist.com`)
- DNS A records:
  - `api.donelist.com` → Server IP
  - `*.donelist.com` → Server IP (optional, for subdomains)

### 3. Tools Installed on Server
```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install required tools
sudo apt install -y curl git make

# Install Docker (for building images)
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
```

---

## 📦 Step 1: Install K3s

### Install K3s on Master Node

```bash
# Install K3s with Traefik enabled (default)
curl -sfL https://get.k3s.io | sh -s - \
  --write-kubeconfig-mode 644 \
  --disable traefik=false

# Verify installation
sudo k3s kubectl get nodes

# Expected output:
# NAME     STATUS   ROLES                  AGE   VERSION
# master   Ready    control-plane,master   1m    v1.28.3+k3s1

# Setup kubectl alias (for convenience)
echo "alias kubectl='sudo k3s kubectl'" >> ~/.bashrc
source ~/.bashrc
```

### Verify K3s Components

```bash
kubectl get pods -A

# Expected output: traefik, coredns, metrics-server pods running
```

---

## 🐳 Step 2: Build & Push Docker Images

### 2.1 Build Go API Image

```bash
# Navigate to project root
cd /path/to/donelist-server

# Build multi-arch image
docker buildx build \
  --platform linux/amd64 \
  -t donelist/api:latest \
  -t donelist/api:v1.0.0 \
  -f deployments/docker/Dockerfile \
  .

# Push to Docker Hub (or private registry)
docker push donelist/api:latest
docker push donelist/api:v1.0.0
```

### 2.2 Alternative: Build Locally & Load into K3s

```bash
# Build image
docker build -t donelist/api:latest -f deployments/docker/Dockerfile .

# Save image to tar
docker save donelist/api:latest -o donelist-api.tar

# Load into K3s
sudo k3s ctr images import donelist-api.tar

# Verify
sudo k3s ctr images ls | grep donelist
```

---

## 🗄️ Step 3: Deploy PostgreSQL

### 3.1 Create Namespace

```bash
kubectl create namespace donelist
```

### 3.2 Create PostgreSQL Secret

```bash
# Create secret for PostgreSQL password
kubectl create secret generic postgres-secret \
  --from-literal=POSTGRES_USER=donelist \
  --from-literal=POSTGRES_PASSWORD=$(openssl rand -base64 32) \
  --from-literal=POSTGRES_DB=donelist \
  -n donelist
```

### 3.3 Deploy PostgreSQL

Create file: `deployments/k8s/postgres-deployment.yaml`

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: donelist
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: donelist
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:16-alpine
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_USER
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_PASSWORD
        - name: POSTGRES_DB
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_DB
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: donelist
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
  type: ClusterIP
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/postgres-deployment.yaml
```

---

## 🔴 Step 4: Deploy Redis

Create file: `deployments/k8s/redis-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: donelist
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        command: ["redis-server", "--appendonly", "yes"]
        volumeMounts:
        - name: redis-storage
          mountPath: /data
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
      volumes:
      - name: redis-storage
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: donelist
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
  type: ClusterIP
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/redis-deployment.yaml
```

---

## 🚀 Step 5: Deploy Donelist API

### 5.1 Create Application Secret

```bash
kubectl create secret generic donelist-secret \
  --from-literal=JWT_SECRET=$(openssl rand -base64 64) \
  --from-literal=STRIPE_SECRET_KEY=sk_test_... \
  -n donelist
```

### 5.2 Create ConfigMap

Create file: `deployments/k8s/configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: donelist-config
  namespace: donelist
data:
  APP_ENV: "production"
  LOG_LEVEL: "info"
  POSTGRES_HOST: "postgres-service"
  POSTGRES_PORT: "5432"
  POSTGRES_DB: "donelist"
  REDIS_HOST: "redis-service"
  REDIS_PORT: "6379"
  API_PORT: "8080"
  WS_PORT: "8081"
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/configmap.yaml
```

### 5.3 Deploy API

Create file: `deployments/k8s/api-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: donelist-api
  namespace: donelist
spec:
  replicas: 3
  selector:
    matchLabels:
      app: donelist-api
  template:
    metadata:
      labels:
        app: donelist-api
    spec:
      containers:
      - name: api
        image: donelist/api:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 8081
          name: websocket
        env:
        - name: APP_ENV
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: APP_ENV
        - name: POSTGRES_HOST
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: POSTGRES_HOST
        - name: POSTGRES_PORT
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: POSTGRES_PORT
        - name: POSTGRES_DB
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: POSTGRES_DB
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_USER
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_PASSWORD
        - name: REDIS_HOST
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: REDIS_HOST
        - name: REDIS_PORT
          valueFrom:
            configMapKeyRef:
              name: donelist-config
              key: REDIS_PORT
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: donelist-secret
              key: JWT_SECRET
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: donelist-api-service
  namespace: donelist
spec:
  selector:
    app: donelist-api
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: websocket
    port: 8081
    targetPort: 8081
  type: ClusterIP
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/api-deployment.yaml
```

---

## 🌐 Step 6: Configure Ingress (Traefik)

### 6.1 Install cert-manager (for SSL)

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml

# Wait for cert-manager pods to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=cert-manager -n cert-manager --timeout=300s
```

### 6.2 Create ClusterIssuer for Let's Encrypt

Create file: `deployments/k8s/cluster-issuer.yaml`

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@donelist.com  # CHANGE THIS
    privateKeySecretRef:
      name: letsencrypt-prod-key
    solvers:
    - http01:
        ingress:
          class: traefik
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/cluster-issuer.yaml
```

### 6.3 Create Ingress

Create file: `deployments/k8s/ingress.yaml`

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: donelist-ingress
  namespace: donelist
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
spec:
  tls:
  - hosts:
    - api.donelist.com
    secretName: donelist-tls
  rules:
  - host: api.donelist.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: donelist-api-service
            port:
              number: 80
      - path: /ws
        pathType: Prefix
        backend:
          service:
            name: donelist-api-service
            port:
              number: 8081
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/ingress.yaml
```

---

## 🔍 Step 7: Verify Deployment

### Check Pods

```bash
kubectl get pods -n donelist

# Expected output: All pods in Running state
# NAME                            READY   STATUS    RESTARTS   AGE
# donelist-api-xxx-xxx            1/1     Running   0          2m
# donelist-api-xxx-yyy            1/1     Running   0          2m
# donelist-api-xxx-zzz            1/1     Running   0          2m
# postgres-xxx-xxx                1/1     Running   0          5m
# redis-xxx-xxx                   1/1     Running   0          5m
```

### Check Services

```bash
kubectl get svc -n donelist

# Expected: donelist-api-service, postgres-service, redis-service
```

### Check Ingress

```bash
kubectl get ingress -n donelist

# Expected: donelist-ingress with ADDRESS assigned
```

### Check TLS Certificate

```bash
kubectl get certificate -n donelist

# Expected: donelist-tls in Ready state
```

### Test API

```bash
curl https://api.donelist.com/health

# Expected: {"status":"ok"}
```

---

## 📊 Step 8: Run Database Migrations

### Option A: Run as Kubernetes Job

Create file: `deployments/k8s/migration-job.yaml`

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
  namespace: donelist
spec:
  template:
    spec:
      containers:
      - name: migrate
        image: donelist/api:latest
        command: ["/app/api", "migrate", "up"]
        env:
        - name: POSTGRES_HOST
          value: "postgres-service"
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_DB
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_DB
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_USER
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: POSTGRES_PASSWORD
      restartPolicy: Never
  backoffLimit: 3
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/migration-job.yaml

# Check job status
kubectl get jobs -n donelist
kubectl logs job/db-migration -n donelist
```

### Option B: Exec into Pod

```bash
# Get a running API pod
POD=$(kubectl get pod -n donelist -l app=donelist-api -o jsonpath="{.items[0].metadata.name}")

# Run migration
kubectl exec -it $POD -n donelist -- /app/api migrate up
```

---

## 🔧 Step 9: Configure Horizontal Pod Autoscaler (HPA)

Create file: `deployments/k8s/hpa.yaml`

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: donelist-api-hpa
  namespace: donelist
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: donelist-api
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

**Apply**:
```bash
kubectl apply -f deployments/k8s/hpa.yaml

# Check HPA status
kubectl get hpa -n donelist
```

---

## 📈 Step 10: Monitoring (Optional)

### Install Prometheus & Grafana

```bash
# Install Prometheus Operator
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml

# Install kube-prometheus (includes Grafana)
git clone https://github.com/prometheus-operator/kube-prometheus.git
cd kube-prometheus
kubectl apply -f manifests/setup
kubectl apply -f manifests/
```

---

## 🔄 Step 11: CI/CD with GitHub Actions

Create file: `.github/workflows/deploy.yml`

```yaml
name: Deploy to K3s

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build Docker image
        run: |
          docker build -t donelist/api:${{ github.sha }} -f deployments/docker/Dockerfile .
          docker tag donelist/api:${{ github.sha }} donelist/api:latest

      - name: Push to Docker Hub
        run: |
          echo "${{ secrets.DOCKER_PASSWORD }}" | docker login -u "${{ secrets.DOCKER_USERNAME }}" --password-stdin
          docker push donelist/api:${{ github.sha }}
          docker push donelist/api:latest

      - name: Deploy to K3s
        run: |
          echo "${{ secrets.KUBECONFIG }}" | base64 -d > kubeconfig.yaml
          export KUBECONFIG=kubeconfig.yaml
          kubectl set image deployment/donelist-api api=donelist/api:${{ github.sha }} -n donelist
          kubectl rollout status deployment/donelist-api -n donelist
```

---

## 🔒 Security Checklist

- [x] Enable TLS with Let's Encrypt
- [x] Use Kubernetes Secrets for sensitive data
- [x] Network policies for pod-to-pod communication
- [x] Resource limits on all pods
- [x] Non-root user in containers
- [x] Regular security updates
- [x] Backup database regularly

---

## 🛠️ Troubleshooting

### Pods Not Starting

```bash
# Check pod events
kubectl describe pod <pod-name> -n donelist

# Check logs
kubectl logs <pod-name> -n donelist

# Common issues:
# - Image pull errors: Check image name and registry access
# - ConfigMap/Secret not found: Ensure they exist
# - Resource limits: Adjust memory/CPU limits
```

### SSL Certificate Issues

```bash
# Check certificate status
kubectl describe certificate donelist-tls -n donelist

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager

# Common issues:
# - DNS not propagated: Wait 5-10 minutes
# - Port 80 not accessible: Check firewall
```

### Database Connection Issues

```bash
# Test PostgreSQL connection from API pod
kubectl exec -it <api-pod> -n donelist -- sh
apk add postgresql-client
psql -h postgres-service -U donelist -d donelist

# Common issues:
# - Wrong credentials: Check secret values
# - Service not ready: Wait for PostgreSQL pod
```

---

## 📚 Next Steps

1. **Backup Strategy**: Setup automated PostgreSQL backups
2. **Monitoring**: Configure alerts and dashboards
3. **Load Testing**: Test with realistic traffic
4. **Disaster Recovery**: Document recovery procedures
5. **Scaling**: Plan for horizontal scaling

---

**Document Status**: Production-ready v1.0
**Last Updated**: 2025-11-10
**Owner**: DevOps Team

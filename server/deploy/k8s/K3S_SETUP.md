# K3s Setup Guide for Donelist API

This guide covers setting up a K3s cluster and deploying the Donelist API.

## Table of Contents

- [What is K3s?](#what-is-k3s)
- [K3s Installation](#k3s-installation)
- [Cluster Configuration](#cluster-configuration)
- [Deploying Donelist API](#deploying-donelist-api)
- [Local Development Setup](#local-development-setup)
- [Production K3s Cluster](#production-k3s-cluster)

## What is K3s?

K3s is a lightweight Kubernetes distribution designed for:
- Edge computing
- IoT devices
- Development environments
- Resource-constrained environments
- Single-node or small clusters

**Key Features**:
- Binary size < 100MB
- Memory footprint < 512MB
- Simple installation (single binary)
- Built-in components (Traefik, CoreDNS, etc.)

## K3s Installation

### Single-Node Cluster (Development)

#### Linux/macOS

```bash
# Install K3s
curl -sfL https://get.k3s.io | sh -

# Check installation
sudo k3s kubectl get nodes

# Copy kubeconfig for kubectl access
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config

# Verify
kubectl get nodes
```

#### Windows (WSL2)

```bash
# In WSL2 terminal
curl -sfL https://get.k3s.io | sh -

# Configure kubectl
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config
```

#### Docker Desktop Alternative

```bash
# Using k3d (K3s in Docker)
brew install k3d  # macOS
# or
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Create cluster
k3d cluster create donelist \
  --agents 2 \
  --port 8080:80@loadbalancer \
  --port 8443:443@loadbalancer

# Set kubeconfig
export KUBECONFIG="$(k3d kubeconfig write donelist)"
```

### Multi-Node Cluster (Production)

#### Server (Master) Node

```bash
# Install K3s server
curl -sfL https://get.k3s.io | sh -s - server \
  --cluster-init \
  --tls-san <your-server-ip> \
  --disable traefik  # We'll use nginx-ingress instead

# Get node token for workers
sudo cat /var/lib/rancher/k3s/server/node-token
```

#### Agent (Worker) Nodes

```bash
# On each worker node
curl -sfL https://get.k3s.io | K3S_URL=https://<server-ip>:6443 \
  K3S_TOKEN=<node-token> sh -
```

## Cluster Configuration

### Install Required Components

#### 1. Nginx Ingress Controller

K3s comes with Traefik by default, but we're using nginx-ingress:

```bash
# Install nginx-ingress
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml

# Wait for deployment
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s

# Verify
kubectl get svc -n ingress-nginx
```

#### 2. Cert-Manager (for TLS)

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --namespace cert-manager \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/instance=cert-manager \
  --timeout=120s

# Verify
kubectl get pods -n cert-manager
```

#### 3. Metrics Server (for HPA)

K3s includes metrics-server by default, but verify:

```bash
# Check if metrics-server is running
kubectl get deployment metrics-server -n kube-system

# If not present, install
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

#### 4. Local Path Provisioner (Default Storage)

K3s includes local-path-provisioner by default:

```bash
# Verify storage class
kubectl get storageclass

# Should see "local-path" as default
```

### Configure Cert-Manager Issuer

```bash
# Create Let's Encrypt staging issuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
    - http01:
        ingress:
          class: nginx
EOF

# Create Let's Encrypt production issuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF
```

## Deploying Donelist API

### Quick Deployment with Helm

```bash
# Navigate to server directory
cd /path/to/donelist/server

# Create namespace
kubectl create namespace donelist

# Create secrets
kubectl create secret generic donelist-secret \
  --from-literal=DB_PASSWORD='your-secure-password' \
  --from-literal=REDIS_PASSWORD='your-redis-password' \
  --from-literal=JWT_SECRET='your-jwt-secret-min-64-chars' \
  -n donelist

# Deploy with Helm
helm install donelist-api ./helm/donelist-api \
  --namespace donelist \
  --values ./helm/donelist-api/values.yaml \
  --set image.tag=latest \
  --wait

# Verify deployment
kubectl get pods -n donelist
kubectl get svc -n donelist
kubectl get ingress -n donelist
```

### Using Deployment Scripts

```bash
# Deploy to staging
ENVIRONMENT=staging \
NAMESPACE=donelist-staging \
IMAGE_TAG=v1.0.0 \
./scripts/deploy.sh

# Deploy to production
ENVIRONMENT=production \
NAMESPACE=donelist \
IMAGE_TAG=v1.0.0 \
./scripts/deploy.sh
```

### Manual Deployment with kubectl

```bash
# Apply Kubernetes manifests
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/secret.yaml  # Update values first!
kubectl apply -f deploy/k8s/pvc.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
kubectl apply -f deploy/k8s/ingress.yaml
```

## Local Development Setup

### Option 1: K3d Cluster

```bash
# Create development cluster
k3d cluster create donelist-dev \
  --agents 1 \
  --port 8080:80@loadbalancer \
  --port 8443:443@loadbalancer \
  --api-port 6443 \
  --volume /tmp/k3d-storage:/var/lib/rancher/k3s/storage

# Install required components
./scripts/setup-k3s-dev.sh

# Deploy application
ENVIRONMENT=staging \
./scripts/deploy.sh
```

### Option 2: Local K3s with Docker Registry

```bash
# Create local registry
docker run -d -p 5000:5000 --restart=always --name registry registry:2

# Configure K3s to use local registry
sudo mkdir -p /etc/rancher/k3s
sudo tee /etc/rancher/k3s/registries.yaml <<EOF
mirrors:
  "localhost:5000":
    endpoint:
      - "http://localhost:5000"
EOF

# Restart K3s
sudo systemctl restart k3s

# Build and push image
cd server
docker build -t localhost:5000/donelist-api:dev .
docker push localhost:5000/donelist-api:dev

# Deploy
helm install donelist-api ./helm/donelist-api \
  --set image.repository=localhost:5000/donelist-api \
  --set image.tag=dev \
  -n donelist
```

### Access Application Locally

```bash
# Using port-forward
kubectl port-forward -n donelist svc/donelist-api 8080:80

# Test
curl http://localhost:8080/health

# Using ingress (if configured with local domain)
# Add to /etc/hosts:
# 127.0.0.1 api.donelist.local

curl http://api.donelist.local/health
```

## Production K3s Cluster

### High Availability Setup

For production, use embedded etcd cluster:

#### Server Nodes (3+ recommended)

```bash
# First server (initializes cluster)
curl -sfL https://get.k3s.io | sh -s - server \
  --cluster-init \
  --tls-san load-balancer-ip \
  --disable traefik \
  --write-kubeconfig-mode 644

# Additional servers
curl -sfL https://get.k3s.io | sh -s - server \
  --server https://<first-server-ip>:6443 \
  --token <token-from-first-server> \
  --tls-san load-balancer-ip \
  --disable traefik
```

#### Load Balancer Configuration

Use HAProxy or nginx for load balancing:

```nginx
# /etc/nginx/nginx.conf
stream {
    upstream k3s_servers {
        server 10.0.0.1:6443 max_fails=3 fail_timeout=5s;
        server 10.0.0.2:6443 max_fails=3 fail_timeout=5s;
        server 10.0.0.3:6443 max_fails=3 fail_timeout=5s;
    }

    server {
        listen 6443;
        proxy_pass k3s_servers;
    }
}
```

### Production Best Practices

1. **Persistent Storage**:
   ```bash
   # Use external storage provider
   kubectl apply -f https://raw.githubusercontent.com/longhorn/longhorn/master/deploy/longhorn.yaml
   ```

2. **Backup and Restore**:
   ```bash
   # Backup etcd snapshots (K3s does this automatically)
   ls /var/lib/rancher/k3s/server/db/snapshots/

   # Manual snapshot
   sudo k3s etcd-snapshot save --name manual-backup

   # Restore from snapshot
   sudo k3s server --cluster-reset --cluster-reset-restore-path=/var/lib/rancher/k3s/server/db/snapshots/snapshot-name
   ```

3. **Monitoring**:
   ```bash
   # Install Prometheus and Grafana
   helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
   helm install prometheus prometheus-community/kube-prometheus-stack \
     -n monitoring --create-namespace
   ```

4. **Security Hardening**:
   ```bash
   # Enable Pod Security Standards
   kubectl label namespace donelist pod-security.kubernetes.io/enforce=restricted

   # Network policies
   kubectl apply -f deploy/k8s/networkpolicy.yaml
   ```

## Upgrading K3s

```bash
# Check current version
k3s --version

# Upgrade K3s
curl -sfL https://get.k3s.io | sh -

# Or specific version
curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION=v1.28.4+k3s1 sh -

# Verify
kubectl get nodes
```

## Uninstalling K3s

```bash
# Server/standalone
sudo /usr/local/bin/k3s-uninstall.sh

# Agent
sudo /usr/local/bin/k3s-agent-uninstall.sh

# k3d
k3d cluster delete donelist
```

## Troubleshooting K3s

### Check K3s Status

```bash
# Service status
sudo systemctl status k3s

# Logs
sudo journalctl -u k3s -f

# Node info
kubectl get nodes -o wide
kubectl describe node <node-name>
```

### Common Issues

#### 1. K3s Not Starting

```bash
# Check logs
sudo journalctl -u k3s --no-pager

# Reset K3s
sudo systemctl stop k3s
sudo rm -rf /var/lib/rancher/k3s
sudo systemctl start k3s
```

#### 2. Permission Issues

```bash
# Fix kubeconfig permissions
sudo chmod 644 /etc/rancher/k3s/k3s.yaml
```

#### 3. Network Issues

```bash
# Check flannel
kubectl get pods -n kube-system -l app=flannel

# Check CoreDNS
kubectl get pods -n kube-system -l k8s-app=kube-dns
```

## Resources

- [K3s Documentation](https://docs.k3s.io/)
- [K3s GitHub](https://github.com/k3s-io/k3s)
- [k3d Documentation](https://k3d.io/)
- [Rancher Forums](https://forums.rancher.com/)

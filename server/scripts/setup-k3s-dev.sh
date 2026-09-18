#!/bin/bash
set -euo pipefail

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# Configuration
CLUSTER_NAME="${CLUSTER_NAME:-donelist-dev}"
USE_K3D="${USE_K3D:-false}"

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if [ "$USE_K3D" = "true" ]; then
        if ! command -v k3d &> /dev/null; then
            log_error "k3d not found. Install with: brew install k3d"
            exit 1
        fi
    else
        if ! command -v k3s &> /dev/null; then
            log_error "k3s not found. Install with: curl -sfL https://get.k3s.io | sh -"
            exit 1
        fi
    fi

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl."
        exit 1
    fi

    if ! command -v helm &> /dev/null; then
        log_error "helm not found. Please install helm."
        exit 1
    fi

    log_info "Prerequisites check passed."
}

# Create K3d cluster
create_k3d_cluster() {
    log_info "Creating K3d cluster '$CLUSTER_NAME'..."

    if k3d cluster list | grep -q "$CLUSTER_NAME"; then
        log_warn "Cluster '$CLUSTER_NAME' already exists. Skipping creation."
        return 0
    fi

    k3d cluster create "$CLUSTER_NAME" \
        --agents 2 \
        --port 8080:80@loadbalancer \
        --port 8443:443@loadbalancer \
        --api-port 6443 \
        --volume /tmp/k3d-storage:/var/lib/rancher/k3s/storage \
        --k3s-arg "--disable=traefik@server:0"

    # Set kubeconfig
    k3d kubeconfig merge "$CLUSTER_NAME" --kubeconfig-switch-context

    log_info "K3d cluster created successfully."
}

# Configure K3s
configure_k3s() {
    log_info "Configuring K3s..."

    # Copy kubeconfig
    mkdir -p ~/.kube
    if [ -f /etc/rancher/k3s/k3s.yaml ]; then
        sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
        sudo chown $USER:$USER ~/.kube/config
        chmod 600 ~/.kube/config
        log_info "Kubeconfig configured."
    else
        log_warn "K3s config not found. Is K3s running?"
    fi
}

# Install nginx-ingress
install_nginx_ingress() {
    log_info "Installing nginx-ingress controller..."

    if kubectl get namespace ingress-nginx &> /dev/null; then
        log_warn "nginx-ingress already installed. Skipping."
        return 0
    fi

    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml

    log_info "Waiting for nginx-ingress to be ready..."
    kubectl wait --namespace ingress-nginx \
        --for=condition=ready pod \
        --selector=app.kubernetes.io/component=controller \
        --timeout=300s

    log_info "nginx-ingress installed successfully."
}

# Install cert-manager
install_cert_manager() {
    log_info "Installing cert-manager..."

    if kubectl get namespace cert-manager &> /dev/null; then
        log_warn "cert-manager already installed. Skipping."
        return 0
    fi

    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

    log_info "Waiting for cert-manager to be ready..."
    kubectl wait --namespace cert-manager \
        --for=condition=ready pod \
        --selector=app.kubernetes.io/instance=cert-manager \
        --timeout=300s

    log_info "cert-manager installed successfully."
}

# Configure cert-manager issuers
configure_cert_issuers() {
    log_info "Configuring cert-manager issuers..."

    # Create staging issuer
    kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: dev@donelist.local
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
    - http01:
        ingress:
          class: nginx
EOF

    # Create production issuer (for later use)
    kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: dev@donelist.local
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF

    log_info "Cert-manager issuers configured."
}

# Verify metrics-server
verify_metrics_server() {
    log_info "Verifying metrics-server..."

    if kubectl get deployment metrics-server -n kube-system &> /dev/null; then
        log_info "metrics-server is installed."
    else
        log_warn "metrics-server not found. Installing..."
        kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

        # Patch for development (skip TLS verification)
        kubectl patch deployment metrics-server -n kube-system --type='json' \
            -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]'

        log_info "metrics-server installed."
    fi
}

# Create development namespace
create_dev_namespace() {
    log_info "Creating development namespace..."

    kubectl create namespace donelist-dev --dry-run=client -o yaml | kubectl apply -f -

    log_info "Development namespace created."
}

# Create local registry (for k3d)
create_local_registry() {
    if [ "$USE_K3D" = "false" ]; then
        return 0
    fi

    log_info "Creating local Docker registry..."

    if docker ps | grep -q "registry.localhost"; then
        log_warn "Local registry already running. Skipping."
        return 0
    fi

    docker run -d \
        --name registry.localhost \
        --restart=always \
        -p 5000:5000 \
        registry:2

    log_info "Local registry created at localhost:5000"
}

# Summary
print_summary() {
    log_info ""
    log_info "========================================="
    log_info "K3s Development Environment Setup Complete!"
    log_info "========================================="
    log_info ""
    log_info "Cluster Information:"
    kubectl cluster-info
    log_info ""
    log_info "Installed Components:"
    log_info "  ✓ nginx-ingress controller"
    log_info "  ✓ cert-manager"
    log_info "  ✓ metrics-server"
    if [ "$USE_K3D" = "true" ]; then
        log_info "  ✓ local Docker registry (localhost:5000)"
    fi
    log_info ""
    log_info "Namespaces:"
    kubectl get namespaces
    log_info ""
    log_info "Next Steps:"
    log_info "1. Build your Docker image:"
    if [ "$USE_K3D" = "true" ]; then
        log_info "   docker build -t localhost:5000/donelist-api:dev ."
        log_info "   docker push localhost:5000/donelist-api:dev"
    else
        log_info "   docker build -t donelist-api:dev ."
    fi
    log_info "2. Deploy the application:"
    log_info "   ENVIRONMENT=staging NAMESPACE=donelist-dev ./scripts/deploy.sh"
    log_info "3. Test the deployment:"
    log_info "   NAMESPACE=donelist-dev ./scripts/test-deployment.sh"
    log_info ""
}

# Main setup flow
main() {
    log_info "Starting K3s development environment setup..."
    log_info "Using K3d: $USE_K3D"

    check_prerequisites

    if [ "$USE_K3D" = "true" ]; then
        create_k3d_cluster
        create_local_registry
    else
        configure_k3s
    fi

    install_nginx_ingress
    install_cert_manager
    configure_cert_issuers
    verify_metrics_server
    create_dev_namespace

    print_summary

    log_info "Setup completed successfully!"
}

# Handle script arguments
case "${1:-setup}" in
    setup)
        main
        ;;
    k3d)
        USE_K3D=true
        main
        ;;
    destroy)
        if [ "$USE_K3D" = "true" ]; then
            log_warn "Destroying K3d cluster '$CLUSTER_NAME'..."
            k3d cluster delete "$CLUSTER_NAME"
            docker rm -f registry.localhost 2>/dev/null || true
        else
            log_error "Destroy only supported for K3d. For K3s, run: sudo /usr/local/bin/k3s-uninstall.sh"
        fi
        ;;
    *)
        echo "Usage: $0 {setup|k3d|destroy}"
        echo ""
        echo "  setup   - Setup K3s development environment (default)"
        echo "  k3d     - Setup K3d cluster instead of K3s"
        echo "  destroy - Destroy K3d cluster"
        exit 1
        ;;
esac

# Makefile for Kubernetes deployment operations
# Usage: make <target>

.PHONY: help deploy deploy-all deploy-stateful deploy-app deploy-monitoring \
        test rollback status logs clean delete scale check-prereqs \
        secrets-seal secrets-apply port-forward dashboard

# Configuration
NAMESPACE ?= donelist
DEPLOYMENT_NAME = donelist-api
IMAGE_TAG ?= latest
REGISTRY ?= your-registry

# Colors for output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "$(BLUE)Kubernetes Deployment Makefile$(NC)"
	@echo ""
	@echo "$(GREEN)Usage:$(NC) make [target]"
	@echo ""
	@echo "$(GREEN)Targets:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(BLUE)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(GREEN)Environment Variables:$(NC)"
	@echo "  NAMESPACE       Kubernetes namespace (default: donelist)"
	@echo "  IMAGE_TAG       Docker image tag (default: latest)"
	@echo "  REGISTRY        Docker registry (default: your-registry)"

check-prereqs: ## Check if required tools are installed
	@echo "$(BLUE)Checking prerequisites...$(NC)"
	@command -v kubectl >/dev/null 2>&1 || { echo "$(YELLOW)kubectl is not installed$(NC)"; exit 1; }
	@command -v kubeseal >/dev/null 2>&1 || echo "$(YELLOW)Warning: kubeseal is not installed (needed for sealed secrets)$(NC)"
	@command -v helm >/dev/null 2>&1 || echo "$(YELLOW)Warning: helm is not installed (optional)$(NC)"
	@echo "$(GREEN)✓ Prerequisites checked$(NC)"

create-namespace: ## Create namespace
	@echo "$(BLUE)Creating namespace $(NAMESPACE)...$(NC)"
	kubectl apply -f namespace.yaml
	@echo "$(GREEN)✓ Namespace created$(NC)"

deploy-secrets: ## Deploy secrets (WARNING: update values first!)
	@echo "$(BLUE)Deploying secrets...$(NC)"
	@echo "$(YELLOW)Warning: Make sure you've updated secret values!$(NC)"
	kubectl apply -f secret.yaml
	@echo "$(GREEN)✓ Secrets deployed$(NC)"

deploy-configmap: ## Deploy configmap
	@echo "$(BLUE)Deploying configmap...$(NC)"
	kubectl apply -f configmap.yaml
	@echo "$(GREEN)✓ Configmap deployed$(NC)"

deploy-stateful: ## Deploy PostgreSQL and Redis StatefulSets with PVCs
	@echo "$(BLUE)Deploying stateful services...$(NC)"
	kubectl apply -f pvc.yaml
	@echo "$(YELLOW)Waiting for StatefulSets to be ready...$(NC)"
	kubectl rollout status statefulset/postgres -n $(NAMESPACE) --timeout=300s || true
	kubectl rollout status statefulset/redis -n $(NAMESPACE) --timeout=300s || true
	@echo "$(GREEN)✓ Stateful services deployed$(NC)"

deploy-app: ## Deploy the Donelist API application
	@echo "$(BLUE)Deploying application...$(NC)"
	kubectl apply -f deployment.yaml
	kubectl apply -f service.yaml
	@echo "$(YELLOW)Waiting for deployment to be ready...$(NC)"
	kubectl rollout status deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE) --timeout=300s
	@echo "$(GREEN)✓ Application deployed$(NC)"

deploy-networking: ## Deploy ingress and network policies
	@echo "$(BLUE)Deploying networking...$(NC)"
	kubectl apply -f cert-manager.yaml
	@echo "$(YELLOW)Waiting for certificates...$(NC)"
	sleep 10
	kubectl apply -f ingress.yaml
	kubectl apply -f networkpolicy.yaml
	@echo "$(GREEN)✓ Networking deployed$(NC)"

deploy-monitoring: ## Deploy monitoring (ServiceMonitor and PrometheusRules)
	@echo "$(BLUE)Deploying monitoring...$(NC)"
	kubectl apply -f monitoring.yaml
	@echo "$(GREEN)✓ Monitoring deployed$(NC)"

deploy-all: check-prereqs create-namespace deploy-configmap deploy-secrets deploy-stateful deploy-app deploy-networking deploy-monitoring ## Deploy everything
	@echo ""
	@echo "$(GREEN)========================================$(NC)"
	@echo "$(GREEN)✓ Full deployment completed!$(NC)"
	@echo "$(GREEN)========================================$(NC)"
	@echo ""
	@make status

deploy: deploy-all ## Alias for deploy-all

update-app: ## Update the application deployment only
	@echo "$(BLUE)Updating application...$(NC)"
	kubectl apply -f deployment.yaml
	kubectl rollout status deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE) --timeout=300s
	@echo "$(GREEN)✓ Application updated$(NC)"

status: ## Show deployment status
	@echo "$(BLUE)Deployment Status$(NC)"
	@echo ""
	@echo "$(GREEN)Namespace:$(NC)"
	kubectl get namespace $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)Deployments:$(NC)"
	kubectl get deployments -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)StatefulSets:$(NC)"
	kubectl get statefulsets -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)Pods:$(NC)"
	kubectl get pods -n $(NAMESPACE) -o wide
	@echo ""
	@echo "$(GREEN)Services:$(NC)"
	kubectl get services -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)Ingress:$(NC)"
	kubectl get ingress -n $(NAMESPACE)
	@echo ""
	@echo "$(GREEN)HPA:$(NC)"
	kubectl get hpa -n $(NAMESPACE) || echo "No HPA found"

test: ## Run deployment tests
	@echo "$(BLUE)Running deployment tests...$(NC)"
	./test-deployment.sh

rollback: ## Rollback to previous deployment
	@echo "$(BLUE)Rolling back deployment...$(NC)"
	./rollback.sh

rollback-revision: ## Rollback to specific revision (usage: make rollback-revision REVISION=3)
	@echo "$(BLUE)Rolling back to revision $(REVISION)...$(NC)"
	./rollback.sh --revision $(REVISION)

logs: ## Show application logs
	@echo "$(BLUE)Showing logs from $(DEPLOYMENT_NAME)...$(NC)"
	kubectl logs -n $(NAMESPACE) -l app=donelist,component=api --tail=100 -f

logs-previous: ## Show logs from previous container (if crashed)
	@echo "$(BLUE)Showing previous logs...$(NC)"
	kubectl logs -n $(NAMESPACE) -l app=donelist,component=api --previous --tail=100

events: ## Show namespace events
	@echo "$(BLUE)Recent events in namespace $(NAMESPACE):$(NC)"
	kubectl get events -n $(NAMESPACE) --sort-by='.lastTimestamp'

describe-pods: ## Describe all pods
	@echo "$(BLUE)Describing pods...$(NC)"
	kubectl describe pods -n $(NAMESPACE) -l app=donelist,component=api

describe-deployment: ## Describe deployment
	@echo "$(BLUE)Describing deployment...$(NC)"
	kubectl describe deployment $(DEPLOYMENT_NAME) -n $(NAMESPACE)

port-forward: ## Port-forward to application (localhost:8080)
	@echo "$(BLUE)Port-forwarding to $(DEPLOYMENT_NAME) on localhost:8080...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	kubectl port-forward -n $(NAMESPACE) svc/donelist-api-service 8080:80

port-forward-metrics: ## Port-forward to metrics endpoint (localhost:9090)
	@echo "$(BLUE)Port-forwarding to metrics endpoint on localhost:9090...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	kubectl port-forward -n $(NAMESPACE) svc/donelist-api-service 9090:9090

shell: ## Get a shell in a pod
	@echo "$(BLUE)Opening shell in pod...$(NC)"
	kubectl exec -it -n $(NAMESPACE) deployment/$(DEPLOYMENT_NAME) -- /bin/sh

scale: ## Scale deployment (usage: make scale REPLICAS=5)
	@echo "$(BLUE)Scaling deployment to $(REPLICAS) replicas...$(NC)"
	kubectl scale deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE) --replicas=$(REPLICAS)
	kubectl rollout status deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE)
	@echo "$(GREEN)✓ Scaled to $(REPLICAS) replicas$(NC)"

restart: ## Restart deployment
	@echo "$(BLUE)Restarting deployment...$(NC)"
	kubectl rollout restart deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE)
	kubectl rollout status deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE)
	@echo "$(GREEN)✓ Deployment restarted$(NC)"

secrets-seal: ## Seal secrets using kubeseal (usage: make secrets-seal INPUT=/tmp/secret.yaml OUTPUT=sealedsecret.yaml)
	@echo "$(BLUE)Sealing secrets...$(NC)"
	kubeseal -f $(INPUT) -w $(OUTPUT)
	@echo "$(GREEN)✓ Secrets sealed to $(OUTPUT)$(NC)"
	@echo "$(YELLOW)Remember to delete the plain secret file: rm $(INPUT)$(NC)"

secrets-view: ## View decoded secrets (requires base64)
	@echo "$(BLUE)Decoding secrets...$(NC)"
	kubectl get secret donelist-secret -n $(NAMESPACE) -o json | jq -r '.data | map_values(@base64d)'

configmap-edit: ## Edit configmap
	@echo "$(BLUE)Editing configmap...$(NC)"
	kubectl edit configmap donelist-config -n $(NAMESPACE)

top: ## Show resource usage
	@echo "$(BLUE)Resource usage:$(NC)"
	kubectl top pods -n $(NAMESPACE) || echo "$(YELLOW)Metrics server not available$(NC)"

dashboard: ## Open Kubernetes dashboard (if installed)
	@echo "$(BLUE)Opening Kubernetes dashboard...$(NC)"
	kubectl proxy &
	@echo "$(GREEN)Dashboard available at: http://localhost:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/$(NC)"

clean: ## Delete application deployment (keeps stateful services)
	@echo "$(BLUE)Cleaning up application deployment...$(NC)"
	kubectl delete -f deployment.yaml || true
	kubectl delete -f service.yaml || true
	kubectl delete -f ingress.yaml || true
	@echo "$(GREEN)✓ Application deployment cleaned$(NC)"

delete: ## Delete everything in namespace (WARNING: destructive!)
	@echo "$(YELLOW)WARNING: This will delete everything in namespace $(NAMESPACE)!$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to cancel, or press Enter to continue...$(NC)"
	@read confirmation
	@echo "$(BLUE)Deleting all resources...$(NC)"
	kubectl delete namespace $(NAMESPACE)
	@echo "$(GREEN)✓ Namespace deleted$(NC)"

validate: ## Validate all YAML files
	@echo "$(BLUE)Validating YAML files...$(NC)"
	@for file in *.yaml; do \
		echo "Validating $$file..."; \
		kubectl apply -f $$file --dry-run=client || exit 1; \
	done
	@echo "$(GREEN)✓ All YAML files are valid$(NC)"

diff: ## Show differences between current and new config
	@echo "$(BLUE)Showing differences...$(NC)"
	kubectl diff -f deployment.yaml || true

history: ## Show rollout history
	@echo "$(BLUE)Rollout history:$(NC)"
	kubectl rollout history deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE)

history-revision: ## Show specific revision details (usage: make history-revision REVISION=3)
	@echo "$(BLUE)Revision $(REVISION) details:$(NC)"
	kubectl rollout history deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE) --revision=$(REVISION)

wait: ## Wait for deployment to be ready
	@echo "$(BLUE)Waiting for deployment to be ready...$(NC)"
	kubectl rollout status deployment/$(DEPLOYMENT_NAME) -n $(NAMESPACE) --timeout=300s
	@echo "$(GREEN)✓ Deployment is ready$(NC)"

cert-status: ## Check TLS certificate status
	@echo "$(BLUE)Certificate status:$(NC)"
	kubectl get certificate -n $(NAMESPACE)
	kubectl describe certificate -n $(NAMESPACE)

hpa-status: ## Check HPA status
	@echo "$(BLUE)HPA status:$(NC)"
	kubectl get hpa -n $(NAMESPACE)
	kubectl describe hpa -n $(NAMESPACE)

network-policy-status: ## Check NetworkPolicy status
	@echo "$(BLUE)NetworkPolicy status:$(NC)"
	kubectl get networkpolicy -n $(NAMESPACE)

monitoring-status: ## Check monitoring resources
	@echo "$(BLUE)Monitoring status:$(NC)"
	@echo ""
	@echo "$(GREEN)ServiceMonitors:$(NC)"
	kubectl get servicemonitor -n $(NAMESPACE) || echo "None found (Prometheus operator may not be installed)"
	@echo ""
	@echo "$(GREEN)PrometheusRules:$(NC)"
	kubectl get prometheusrule -n $(NAMESPACE) || echo "None found"

quick-check: ## Quick health check
	@echo "$(BLUE)Quick health check...$(NC)"
	@kubectl get pods -n $(NAMESPACE) -l app=donelist,component=api
	@echo ""
	@kubectl get deployment $(DEPLOYMENT_NAME) -n $(NAMESPACE)
	@echo ""
	@kubectl top pods -n $(NAMESPACE) -l app=donelist,component=api 2>/dev/null || echo "$(YELLOW)Metrics not available$(NC)"

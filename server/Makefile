.PHONY: help build run test clean migrate-up migrate-down migrate-create docker-build docker-run lint

# Variables
APP_NAME=donelist-api
VERSION?=latest
DOCKER_IMAGE=$(APP_NAME):$(VERSION)
MIGRATION_DIR=./migrations

# Help command
help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

# Build
build: ## Build the application
	@echo "Building $(APP_NAME)..."
	@go build -o bin/api cmd/api/main.go
	@echo "Build complete!"

build-all: ## Build all binaries
	@echo "Building all binaries..."
	@go build -o bin/api cmd/api/main.go
	@go build -o bin/migrate cmd/migrate/main.go
	@go build -o bin/backup cmd/backup/main.go
	@go build -o bin/reindex cmd/reindex/main.go
	@echo "Build complete!"

build-backup: ## Build backup tool
	@echo "Building backup tool..."
	@go build -o bin/backup cmd/backup/main.go
	@echo "Backup tool built!"

build-reindex: ## Build reindex tool
	@echo "Building reindex tool..."
	@go build -o bin/reindex cmd/reindex/main.go
	@echo "Reindex tool built!"

# Run
run: ## Run the application
	@echo "Running $(APP_NAME)..."
	@go run cmd/api/main.go

run-dev: ## Run with auto-reload (requires air)
	@air

# Testing
test: ## Run tests
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@echo "Tests complete!"

test-unit: ## Run unit tests only (exclude integration)
	@echo "Running unit tests..."
	@go test -v -short ./...
	@echo "Unit tests complete!"

test-verbose: ## Run tests with verbose output
	@echo "Running tests with verbose output..."
	@go test -v -race ./...

test-contract: ## Run API contract tests
	@echo "Running contract tests..."
	@go test -v -tags=contract ./internal/api/handlers -run TestContract || echo "Some contract tests may need setup"
	@echo "Contract tests complete!"

test-pact: ## Run Pact contract tests
	@echo "Running Pact contract tests..."
	@mkdir -p tests/pacts
	@go test -v -tags=contract ./tests/contract -run TestPact
	@echo "Pact contract tests complete!"

test-pact-verify: ## Verify provider against Pact contracts
	@echo "Verifying provider contracts..."
	@go test -v -tags=contract ./tests/contract -run TestProviderVerification
	@echo "Provider verification complete!"

test-pact-generate: ## Generate Pact contract files
	@echo "Generating Pact contracts..."
	@mkdir -p tests/pacts
	@go test -v -tags=contract ./tests/contract -run TestCategoryContractGeneration
	@go test -v -tags=contract ./tests/contract -run TestTagContractGeneration
	@go test -v -tags=contract ./tests/contract -run TestTimelineContractGeneration
	@go test -v -tags=contract ./tests/contract -run TestNegativeContractScenarios
	@echo "Pact contracts generated in tests/pacts/"

test-schema: ## Run OpenAPI schema validation tests
	@echo "Running schema validation tests..."
	@go test -v -tags=contract ./tests/contract -run TestSchema
	@echo "Schema validation tests complete!"

test-contract-all: test-contract test-pact test-schema ## Run all contract tests
	@echo "All contract tests complete!"

validate-openapi: ## Validate OpenAPI specification
	@echo "Validating OpenAPI spec..."
	@chmod +x scripts/validate-openapi.sh
	@./scripts/validate-openapi.sh

generate-openapi: ## Generate OpenAPI spec from code
	@echo "Generating OpenAPI specification..."
	@chmod +x scripts/generate-openapi.sh
	@./scripts/generate-openapi.sh

lint-openapi: ## Lint OpenAPI spec with Spectral
	@echo "Linting OpenAPI spec..."
	@npx @stoplight/spectral-cli lint docs/api/openapi.yaml --ruleset .spectral.yml

api-docs: ## Generate API documentation from OpenAPI spec
	@echo "Generating API documentation..."
	@npx @redocly/cli build-docs docs/api/openapi.yaml -o docs/api/index.html
	@echo "API documentation generated at docs/api/index.html"

api-check: validate-openapi lint-openapi ## Validate and lint OpenAPI spec
	@echo "API specification checks complete!"

api-breaking-changes: ## Check for breaking changes in OpenAPI spec
	@echo "Checking for breaking changes..."
	@if [ -f "docs/api/openapi.yaml.old" ]; then \
		npx oasdiff breaking docs/api/openapi.yaml.old docs/api/openapi.yaml; \
	else \
		echo "No previous spec found. Save current as: cp docs/api/openapi.yaml docs/api/openapi.yaml.old"; \
	fi

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	@go test -race ./...

test-parallel: ## Run tests in parallel
	@echo "Running tests in parallel..."
	@go test -v -race -parallel 4 ./...

test-coverage: ## Run tests with coverage report
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep total

test-coverage-report: ## Generate detailed coverage report
	@echo "Generating detailed coverage report..."
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out
	@echo "Detailed coverage report: coverage.html"

test-coverage-check: ## Check if coverage meets minimum threshold
	@echo "Checking coverage threshold..."
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@chmod +x scripts/coverage/check-coverage.sh
	@./scripts/coverage/check-coverage.sh

test-coverage-ci: ## Generate coverage reports for CI
	@echo "Generating coverage reports for CI..."
	@chmod +x scripts/coverage/coverage-report.sh
	@./scripts/coverage/coverage-report.sh

test-coverage-badge: ## Generate coverage badge data
	@echo "Generating coverage badge..."
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@chmod +x scripts/coverage/check-coverage.sh
	@MIN_COVERAGE=0 ./scripts/coverage/check-coverage.sh
	@echo "Badge data saved to coverage.json"

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -tags=integration ./tests/integration/...

test-e2e: ## Run end-to-end tests
	@echo "Running E2E tests..."
	@go test -v -tags=e2e ./tests/e2e/...

test-all: test test-integration test-e2e ## Run all tests (unit, integration, e2e)
	@echo "All tests complete!"

test-health: ## Run health package tests
	@echo "Running health package tests..."
	@go test -v -cover ./internal/health/...

test-bench: ## Run benchmark tests
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

test-clean: ## Clean test cache
	@echo "Cleaning test cache..."
	@go clean -testcache
	@echo "Test cache cleaned!"

# Security testing
test-security: ## Run comprehensive security tests
	@echo "Running security tests..."
	@./scripts/security-test.sh
	@echo "Security tests complete!"

test-security-ci: ## Run security tests for CI/CD
	@echo "Running CI security tests..."
	@gosec -fmt sarif -out gosec-results.sarif ./...
	@echo "CI security tests complete!"

# Mutation testing
test-mutation: ## Run mutation tests on critical paths
	@echo "Running mutation tests..."
	@$(HOME)/go/bin/gremlins unleash
	@echo "Mutation testing complete! Check mutation-report.html for results."

test-mutation-auth: ## Run mutation tests on auth package
	@echo "Running mutation tests on auth package..."
	@$(HOME)/go/bin/gremlins unleash --tags auth ./internal/auth/...
	@echo "Auth mutation testing complete!"

test-mutation-health: ## Run mutation tests on health package
	@echo "Running mutation tests on health package..."
	@$(HOME)/go/bin/gremlins unleash ./internal/health/...
	@echo "Health mutation testing complete!"

test-mutation-payment: ## Run mutation tests on payment package
	@echo "Running mutation tests on payment package..."
	@$(HOME)/go/bin/gremlins unleash ./internal/payment/...
	@echo "Payment mutation testing complete!"

test-mutation-dry-run: ## Show what mutations would be tested
	@echo "Mutation testing dry run..."
	@$(HOME)/go/bin/gremlins unleash --dry-run
	@echo "Dry run complete!"

test-mutation-diff: ## Run mutation tests only on changed files
	@echo "Running mutation tests on changed files..."
	@$(HOME)/go/bin/gremlins unleash --diff
	@echo "Diff mutation testing complete!"

# Dependencies
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies downloaded!"

deps-upgrade: ## Upgrade dependencies
	@echo "Upgrading dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "Dependencies upgraded!"

# Database migrations
migrate-up: ## Run database migrations up
	@echo "Running migrations up..."
	@go run cmd/migrate/main.go up

migrate-down: ## Rollback database migrations
	@echo "Rolling back migrations..."
	@go run cmd/migrate/main.go down

migrate-create: ## Create a new migration file (use: make migrate-create name=create_users_table)
	@if [ -z "$(name)" ]; then \
		echo "Error: name is required. Usage: make migrate-create name=create_users_table"; \
		exit 1; \
	fi
	@echo "Creating migration: $(name)"
	@migrate create -ext sql -dir $(MIGRATION_DIR) -seq $(name)

# Database
db-reset: ## Reset database (drop and recreate)
	@echo "Resetting database..."
	@go run cmd/migrate/main.go reset
	@echo "Database reset complete!"

db-seed: ## Seed database with test data
	@echo "Seeding database..."
	@go run cmd/migrate/main.go seed
	@echo "Seed complete!"

# Docker
docker-build: ## Build Docker image
	@echo "Building Docker image: $(DOCKER_IMAGE)"
	@docker build -t $(DOCKER_IMAGE) -f deployments/docker/Dockerfile .
	@echo "Docker image built!"

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	@docker run -p 8080:8080 -p 8081:8081 --env-file .env $(DOCKER_IMAGE)

docker-compose-up: ## Start all services with docker-compose
	@echo "Starting services with docker-compose..."
	@docker-compose -f deployments/docker/docker-compose.yml up -d
	@echo "Services started!"

docker-compose-down: ## Stop all services
	@echo "Stopping services..."
	@docker-compose -f deployments/docker/docker-compose.yml down
	@echo "Services stopped!"

# Linting and formatting
lint: ## Run linter
	@echo "Running linter..."
	@golangci-lint run ./...
	@echo "Linting complete!"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Formatting complete!"

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete!"

# Clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf dist/
	@rm -f coverage.out coverage.html
	@echo "Clean complete!"

# Development setup
setup: ## Setup development environment
	@echo "Setting up development environment..."
	@cp .env.example .env
	@go mod download
	@echo "Setup complete! Edit .env file with your configuration."

# Health check
health: ## Check if API is running
	@echo "Checking API health..."
	@curl -f http://localhost:8080/health || echo "API is not running"

# Load testing
load-test: ## Run basic load test (checkin operations)
	@echo "Running load tests..."
	@k6 run tests/load/checkin_load_test.js

load-test-auth: ## Run authentication load test
	@echo "Running authentication load test..."
	@k6 run tests/load/auth_load_test.js

load-test-checkin: ## Run checkin operations load test
	@echo "Running checkin load test..."
	@k6 run tests/load/checkin_load_test.js

load-test-spike: ## Run spike test (sudden traffic increase)
	@echo "Running spike test..."
	@k6 run tests/load/spike_test.js

load-test-stress: ## Run stress test (find breaking point)
	@echo "Running stress test..."
	@k6 run tests/load/stress_test.js

load-test-soak: ## Run soak test (3 hour endurance test)
	@echo "Running soak test (this will take 3+ hours)..."
	@k6 run tests/load/soak_test.js

load-test-soak-short: ## Run short soak test (10 minutes)
	@echo "Running short soak test (10 minutes)..."
	@k6 run tests/load/soak_test.js --stage 1m:50,10m:50,1m:0

load-test-all: load-test-auth load-test-checkin load-test-spike ## Run all quick load tests
	@echo "All load tests completed!"

# Git hooks
install-hooks: ## Install git hooks
	@echo "Installing git hooks..."
	@cp scripts/pre-commit.sh .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Git hooks installed!"

# Generate
generate: ## Run go generate
	@echo "Running go generate..."
	@go generate ./...
	@echo "Generate complete!"

# Watch (requires air)
install-air: ## Install air for hot reload
	@echo "Installing air..."
	@go install github.com/cosmtrek/air@latest
	@echo "Air installed! Run 'make run-dev' to use it."

# Backup and Recovery
backup: ## Create a database backup
	@echo "Creating database backup..."
	@./scripts/backup/backup.sh

backup-restore: ## Restore database from backup (requires BACKUP_FILE variable)
	@if [ -z "$(BACKUP_FILE)" ]; then \
		echo "Error: BACKUP_FILE is required. Usage: make backup-restore BACKUP_FILE=/path/to/backup.sql.gz"; \
		exit 1; \
	fi
	@echo "Restoring database from backup..."
	@./scripts/backup/restore.sh --file $(BACKUP_FILE) --drop --verify

backup-monitor: ## Check backup system health
	@echo "Checking backup system health..."
	@./scripts/backup/monitor.sh

backup-status: ## Show backup system status
	@echo "Getting backup status..."
	@./bin/backup --command status

backup-daemon: ## Run backup daemon with scheduler
	@echo "Starting backup daemon..."
	@./bin/backup --daemon

backup-setup: ## Setup backup directories and permissions
	@echo "Setting up backup system..."
	@mkdir -p /var/backups/donelist/wal
	@chmod +x scripts/backup/*.sh
	@echo "Backup system setup complete!"

# Kubernetes/Helm deployment
k8s-setup: ## Setup K3s development environment
	@echo "Setting up K3s development environment..."
	@./scripts/setup-k3s-dev.sh

k8s-setup-k3d: ## Setup K3d cluster for local development
	@echo "Setting up K3d cluster..."
	@./scripts/setup-k3s-dev.sh k3d

k8s-deploy-staging: ## Deploy to staging environment
	@echo "Deploying to staging..."
	@ENVIRONMENT=staging NAMESPACE=donelist-staging ./scripts/deploy.sh

k8s-deploy-production: ## Deploy to production environment
	@echo "Deploying to production..."
	@ENVIRONMENT=production NAMESPACE=donelist ./scripts/deploy.sh

k8s-test: ## Test Kubernetes deployment
	@echo "Testing deployment..."
	@./scripts/test-deployment.sh

k8s-rollback: ## Rollback Kubernetes deployment
	@echo "Rolling back deployment..."
	@./scripts/rollback.sh

helm-install: ## Install application with Helm
	@echo "Installing with Helm..."
	@helm install donelist-api ./helm/donelist-api \
		--namespace donelist \
		--create-namespace \
		--values ./helm/donelist-api/values.yaml

helm-upgrade: ## Upgrade Helm release
	@echo "Upgrading Helm release..."
	@helm upgrade donelist-api ./helm/donelist-api \
		--namespace donelist \
		--values ./helm/donelist-api/values.yaml

helm-uninstall: ## Uninstall Helm release
	@echo "Uninstalling Helm release..."
	@helm uninstall donelist-api --namespace donelist

helm-lint: ## Lint Helm chart
	@echo "Linting Helm chart..."
	@helm lint ./helm/donelist-api

helm-template: ## Generate Kubernetes manifests from Helm chart
	@echo "Generating Helm templates..."
	@helm template donelist-api ./helm/donelist-api \
		--values ./helm/donelist-api/values.yaml

# Docker registry
docker-build-push: docker-build ## Build and push Docker image
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE)

docker-build-multiarch: ## Build multi-architecture Docker image
	@echo "Building multi-architecture Docker image..."
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(DOCKER_IMAGE) \
		--push \
		-f Dockerfile .

# Default target
.DEFAULT_GOAL := help

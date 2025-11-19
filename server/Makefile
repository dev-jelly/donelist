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
	@echo "Build complete!"

build-backup: ## Build backup tool
	@echo "Building backup tool..."
	@go build -o bin/backup cmd/backup/main.go
	@echo "Backup tool built!"

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

test-coverage: ## Run tests with coverage report
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -tags=integration ./tests/integration/...

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
load-test: ## Run load tests with k6
	@echo "Running load tests..."
	@k6 run tests/load/checkin_load_test.js

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

# Default target
.DEFAULT_GOAL := help

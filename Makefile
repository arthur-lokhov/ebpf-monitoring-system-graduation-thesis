.PHONY: build test clean run dev docker-build docker-run ui-dev docker-build-builder docker-build-runtime docker-up docker-down docker-restart docker-logs docker-clean migrate-up migrate-down plugin-build help git-safe-dir clean-plugins

# Variables
BINARY_NAME=epbf-monitor
GO=go
GOFLAGS=-v
GOCMD=$(GO)
DOCKER_COMPOSE=docker-compose -f deployments/docker-compose.yml

# UI Development
ui-dev:
	cd ui && npm install && npm run dev

# Build
build:
	$(GOCMD) build -o bin/$(BINARY_NAME) ./cmd/epbf-monitor

# Run
run: build
	sudo ./bin/$(BINARY_NAME)

# Dev mode with hot reload (requires air)
dev:
	air --cmd "$(GO) run ./cmd/epbf-monitor"

# Test
test:
	$(GOCMD) test ./...

# Test with coverage
test-coverage:
	$(GOCMD) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Clean
clean:
	rm -rf bin/
	rm -rf coverage.out coverage.html

# Generate (mocks, etc.)
generate:
	$(GOCMD) generate ./...

# Lint
lint:
	golangci-lint run ./...

# Docker build (builder image)
docker-build-builder:
	docker build -f deployments/docker/builder.Dockerfile -t epbf-monitor-builder .

# Docker build (runtime image)
docker-build-runtime:
	docker build -f deployments/docker/runtime.Dockerfile -t epbf-monitor-runtime .

# Docker compose (local dev - DB, S3, Prometheus, UI only)
docker-up:
	$(DOCKER_COMPOSE) up -d --build

docker-down:
	$(DOCKER_COMPOSE) down

docker-restart:
	$(DOCKER_COMPOSE) restart

docker-logs:
	$(DOCKER_COMPOSE) logs -f

docker-clean:
	$(DOCKER_COMPOSE) down -v

# Run backend on host (for eBPF access)
run-backend:
	source deployments/.env && \
	DB_HOST=localhost \
	DB_PORT=5432 \
	DB_USER=epbf \
	DB_PASSWORD=epbf_password \
	DB_NAME=epbf \
	S3_ENDPOINT=http://localhost:3900 \
	S3_REGION=garage \
	S3_BUCKET=epbf-plugins \
	S3_ACCESS_KEY=$$GARAGE_ADMIN_TOKEN \
	S3_SECRET_KEY=$$GARAGE_SECRET_KEY \
	ENABLE_DOCKER=true \
	BUILD_DIR=/tmp/epbf-builds \
	LOG_LEVEL=info \
	go run ./cmd/epbf-monitor

# Run backend with .env file (simplified)
run-with-env:
	cd deployments && source .env && cd .. && go run ./cmd/epbf-monitor

# Run backend (auto-loads deployments/.env)
run-dev:
	go run ./cmd/epbf-monitor

# Run database migrations inside container
migrate-run:
	bash deployments/run-migrations.sh

# Git safe directory for container
git-safe-dir:
	docker exec epbf-monitor-server git config --global --add safe.directory '*'

# Clean plugins table
clean-plugins:
	docker exec -i epbf-postgres psql -U epbf -d epbf -c "DELETE FROM plugins;"

# Database migrations
migrate-up:
	migrate -path internal/storage/postgres/migrations -database "postgresql://localhost:5432/epbf?sslmode=disable" up

migrate-down:
	migrate -path internal/storage/postgres/migrations -database "postgresql://localhost:5432/epbf?sslmode=disable" down

# Plugin build templates
plugin-build:
	@echo "Usage: make plugin-build PLUGIN=<plugin-path>"
	@echo "Example: make plugin-build PLUGIN=plugins/network"

# Help
help:
	@echo "Available targets:"
	@echo "  build              - Build the binary"
	@echo "  run                - Build and run"
	@echo "  dev                - Run with hot reload"
	@echo "  ui-dev             - Run UI development server"
	@echo "  test               - Run tests"
	@echo "  test-coverage      - Run tests with coverage"
	@echo "  clean              - Clean build artifacts"
	@echo "  lint               - Run linter"
	@echo "  docker-up          - Start Docker services (DB, S3, Prometheus, UI)"
	@echo "  docker-down        - Stop Docker services"
	@echo "  docker-restart     - Restart Docker services"
	@echo "  docker-logs        - Show Docker logs"
	@echo "  docker-clean       - Stop and remove volumes"
	@echo "  run-backend        - Run backend on host (for eBPF access, loads .env)"
	@echo "  run-with-env       - Run backend with .env file (manual sourcing)"
	@echo "  run-dev            - Run backend (auto-loads deployments/.env)"
	@echo "  migrate-run        - Run database migrations"
	@echo "  migrate-up         - Run database migrations (external)"
	@echo "  migrate-down       - Rollback database migrations"
	@echo "  git-safe-dir       - Add safe.directory to git config in container"
	@echo "  clean-plugins      - Delete all records from plugins table"
	@echo ""
	@echo "Quick start:"
	@echo "  make run-dev       # Runs the app, auto-loads deployments/.env"
	@echo "  make docker-up     # Start all services in Docker"

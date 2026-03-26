.PHONY: build test clean run dev docker-build docker-run

# Variables
BINARY_NAME=epbf-monitor
GO=go
GOFLAGS=-v
GOCMD=$(GO) $(GOFLAGS)

# Build
build:
	$(GOCMD) -o bin/$(BINARY_NAME) ./cmd/epbf-monitor

# Run
run: build
	./bin/$(BINARY_NAME)

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
	docker build -f build/docker/builder.Dockerfile -t epbf-monitor-builder .

# Docker build (runtime image)
docker-build-runtime:
	docker build -f build/docker/runtime.Dockerfile -t epbf-monitor-runtime .

# Docker compose (local dev)
docker-up:
	docker-compose -f deployments/docker-compose.yml up -d

docker-down:
	docker-compose -f deployments/docker-compose.yml down

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
	@echo "  test               - Run tests"
	@echo "  test-coverage      - Run tests with coverage"
	@echo "  clean              - Clean build artifacts"
	@echo "  lint               - Run linter"
	@echo "  docker-up          - Start docker-compose"
	@echo "  docker-down        - Stop docker-compose"
	@echo "  migrate-up         - Run database migrations"
	@echo "  migrate-down       - Rollback database migrations"

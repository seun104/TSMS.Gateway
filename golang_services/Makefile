# Makefile for Arad SMS Gateway Go Services

.PHONY: all build test run lint clean help

# Variables
DOCKER_COMPOSE_FILE := docker-compose.yml
GO_CMD := go
GO_LINT_CMD := golangci-lint
GO_TEST_CMD := $(GO_CMD) test
GO_BUILD_CMD := $(GO_CMD) build

# Service names (derived from cmd directory)
USER_SERVICE_DIR := ./cmd/user_service
USER_SERVICE_BIN := ./bin/user_service
PUBLIC_API_SERVICE_DIR := ./cmd/public_api_service
PUBLIC_API_SERVICE_BIN := ./bin/public_api_service
SMS_SENDING_SERVICE_DIR := ./cmd/sms_sending_service
SMS_SENDING_SERVICE_BIN := ./bin/sms_sending_service
BILLING_SERVICE_DIR := ./cmd/billing_service
BILLING_SERVICE_BIN := ./bin/billing_service
DELIVERY_RETRIEVAL_SERVICE_DIR := ./cmd/delivery_retrieval_service
DELIVERY_RETRIEVAL_SERVICE_BIN := ./bin/delivery_retrieval_service

# Go packages for linting and testing (adjust as needed)
GO_PACKAGES := ./...

# Default target
all: build lint test

# Build services
build: $(USER_SERVICE_BIN) $(PUBLIC_API_SERVICE_BIN) $(SMS_SENDING_SERVICE_BIN) $(BILLING_SERVICE_BIN) $(DELIVERY_RETRIEVAL_SERVICE_BIN)

$(USER_SERVICE_BIN):
	@echo "Building User Service..."
	@mkdir -p ./bin
	$(GO_BUILD_CMD) -o $(USER_SERVICE_BIN) $(USER_SERVICE_DIR)/main.go

$(PUBLIC_API_SERVICE_BIN):
	@echo "Building Public API Service..."
	@mkdir -p ./bin
	$(GO_BUILD_CMD) -o $(PUBLIC_API_SERVICE_BIN) $(PUBLIC_API_SERVICE_DIR)/main.go

$(SMS_SENDING_SERVICE_BIN):
	@echo "Building SMS Sending Service..."
	@mkdir -p ./bin
	$(GO_BUILD_CMD) -o $(SMS_SENDING_SERVICE_BIN) $(SMS_SENDING_SERVICE_DIR)/main.go

$(BILLING_SERVICE_BIN):
	@echo "Building Billing Service..."
	@mkdir -p ./bin
	$(GO_BUILD_CMD) -o $(BILLING_SERVICE_BIN) $(BILLING_SERVICE_DIR)/main.go

$(DELIVERY_RETRIEVAL_SERVICE_BIN):
	@echo "Building Delivery Retrieval Service..."
	@mkdir -p ./bin
	$(GO_BUILD_CMD) -o $(DELIVERY_RETRIEVAL_SERVICE_BIN) $(DELIVERY_RETRIEVAL_SERVICE_DIR)/main.go

# Run services (using Docker Compose)
run:
	@echo "Starting services with Docker Compose..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) up --build

# Stop services
stop:
	@echo "Stopping services with Docker Compose..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) down

# Test services
test:
	@echo "Running tests..."
	$(GO_TEST_CMD) $(GO_PACKAGES) -v

# Lint code
# Ensure golangci-lint is installed: https://golangci-lint.run/usage/install/
lint:
	@echo "Linting code..."
	$(GO_LINT_CMD) run $(GO_PACKAGES)

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf ./bin

# Help
help:
	@echo "Available targets:"
	@echo "  all        - Build, lint, and test all services (default)"
	@echo "  build      - Build all Go services"
	@echo "  run        - Start all services using Docker Compose"
	@echo "  stop       - Stop all services started with Docker Compose"
	@echo "  test       - Run unit tests for all Go services"
	@echo "  lint       - Lint all Go service code"
	@echo "  clean      - Remove build artifacts"
	@echo "  help       - Show this help message"
	@echo ""
	@echo "Individual service build targets:"
	@echo "  $(USER_SERVICE_BIN)"
	@echo "  $(PUBLIC_API_SERVICE_BIN)"
	@echo "  $(SMS_SENDING_SERVICE_BIN)"
	@echo "  $(BILLING_SERVICE_BIN)"
	@echo "  $(DELIVERY_RETRIEVAL_SERVICE_BIN)"

# Example of building a specific service: make ./bin/user_service

.PHONY: build test clean install lint run help

# Variables
BINARY_NAME=gtool
BUILD_DIR=bin
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-X github.com/oswaldo-montano/gtool/internal/cli.Version=$(VERSION) \
				  -X github.com/oswaldo-montano/gtool/internal/cli.GitCommit=$(GIT_COMMIT) \
				  -X github.com/oswaldo-montano/gtool/internal/cli.BuildDate=$(BUILD_DATE)"

VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

## help: Show this help message
help:
	@echo 'Usage:'
	@echo '  make <target>'
	@echo ''
	@echo 'Targets:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/gtool

## test: Run tests
test:
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...

## test-coverage: Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	open coverage.html || xdg-open coverage.html || echo "Please open coverage.html manually."

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

## install: Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

## lint: Run linters
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run --config configs/golangci-lint.yml

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	goimports -w .

## mod: Download dependencies
mod:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

## run: Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

## dev: Run in development mode
dev:
	@echo "Running in development mode..."
	$(GO) run ./cmd/gtool $3

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .

## release: Build for multiple platforms
release:
	@echo "Building releases..."
	@mkdir -p $(BUILD_DIR)/releases
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/releases/$(BINARY_NAME)-linux-amd64 ./cmd/gtool
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/releases/$(BINARY_NAME)-linux-arm64 ./cmd/gtool
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/releases/$(BINARY_NAME)-darwin-amd64 ./cmd/gtool
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/releases/$(BINARY_NAME)-darwin-arm64 ./cmd/gtool
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/releases/$(BINARY_NAME)-windows-amd64.exe ./cmd/gtool

.DEFAULT_GOAL := help

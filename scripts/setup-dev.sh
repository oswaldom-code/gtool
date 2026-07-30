#!/bin/bash
set -e

# Development Environment Setup Script

echo "Setting up GTOOL development environment..."

# Check Go version
echo "Checking Go version..."
if ! command -v go &> /dev/null; then
    echo "✗ Go is not installed. Please install Go 1.24+"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✓ Go $GO_VERSION installed"

# Check Docker
echo "Checking Docker..."
if ! command -v docker &> /dev/null; then
    echo "✗ Docker is not installed. Please install Docker"
    exit 1
fi
echo "✓ Docker installed"

# Check yq
echo "Checking yq..."
if ! command -v yq &> /dev/null; then
    echo "⚠ yq is not installed. Installing..."
    # Install yq based on OS
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    if [ "$OS" = "darwin" ]; then
        brew install yq
    elif [ "$OS" = "linux" ]; then
        sudo wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64
        sudo chmod +x /usr/local/bin/yq
    fi
fi
echo "✓ yq installed"

# Download Go dependencies
echo "Downloading Go dependencies..."
go mod download
echo "✓ Dependencies downloaded"

# Install development tools
echo "Installing development tools..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest
echo "✓ Development tools installed"

# Build the project
echo "Building gtool..."
make build
echo "✓ Build successful"

# Run tests
echo "Running tests..."
make test
echo "✓ Tests passed"

echo ""
echo "✓ Development environment setup complete!"
echo ""
echo "Next steps:"
echo "  make dev     - Run in development mode"
echo "  make test    - Run tests"
echo "  make lint    - Run linters"
echo "  make help    - Show all available commands"

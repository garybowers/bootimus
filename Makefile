.PHONY: help build build-all build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 run push clean

DOCKER_USER ?= garybowers
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -w -s -X bootimus/internal/server.Version=$(VERSION)

# Default target
help:
	@echo "Bootimus Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  make build              - Build for Linux AMD64 (default)"
	@echo "  make build-all          - Build for all platforms"
	@echo "  make build-linux-amd64  - Build for Linux AMD64"
	@echo "  make build-linux-arm64  - Build for Linux ARM64"
	@echo "  make build-darwin-amd64 - Build for macOS Intel"
	@echo "  make build-darwin-arm64 - Build for macOS Apple Silicon"
	@echo "  make build-windows-amd64- Build for Windows AMD64"
	@echo "  make clean              - Remove all build artifacts"
	@echo "  make run                - Start Docker Compose"
	@echo "  make push               - Build and push Docker images"
	@echo ""
	@echo "Set VERSION to override version:"
	@echo "  VERSION=1.0.0 make build"
	@echo ""

# Build all platforms
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64
	@echo "All builds complete!"

# Default build (Linux AMD64)
build: build-linux-amd64

# Linux AMD64
build-linux-amd64:
	@echo "Building for Linux AMD64..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bootimus-amd64-linux .

# Linux ARM64
build-linux-arm64:
	@echo "Building for Linux ARM64..."
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bootimus-arm64-linux .

# macOS AMD64 (Intel)
build-darwin-amd64:
	@echo "Building for macOS AMD64..."
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bootimus-amd64-darwin .

# macOS ARM64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building for macOS ARM64..."
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bootimus-arm64-darwin .

# Windows AMD64
build-windows-amd64:
	@echo "Building for Windows AMD64..."
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bootimus-amd64-windows.exe .

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f bootimus-*-linux bootimus-*-darwin bootimus-*-windows.exe bootimus

run:
	docker-compose up -d

push:
	docker buildx create --use --name bootimus-builder --driver docker-container || docker buildx use bootimus-builder
	docker buildx build -f Dockerfile.multistage \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		-t $(DOCKER_USER)/bootimus:$(VERSION) \
		-t $(DOCKER_USER)/bootimus:latest \
		--push \
		--no-cache \
		.


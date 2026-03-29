.PHONY: help build run clean docker-build docker-up docker-down docker-push release

VERSION    ?= $(shell cat VERSION)
DOCKER_USER ?= garybowers
IMAGE      := $(DOCKER_USER)/bootimus
LDFLAGS    := -w -s -X bootimus/internal/server.Version=$(VERSION)
BINARY     := bootimus

## Help -----------------------------------------------------------------------

help:
	@echo "Bootimus Build System"
	@echo ""
	@echo "Local (binary):"
	@echo "  make build            - Build binary for current platform"
	@echo "  make run              - Build and run locally"
	@echo "  make clean            - Remove build artifacts"
	@echo ""
	@echo "Local (container):"
	@echo "  make docker-build     - Build container image locally"
	@echo "  make docker-up        - Start services via docker compose"
	@echo "  make docker-down      - Stop services"
	@echo ""
	@echo "Publish:"
	@echo "  make release          - Build all platform binaries for GitHub release"
	@echo "  make docker-push      - Build and push multi-arch images to Docker Hub"
	@echo ""
	@echo "Override version:  VERSION=1.0.0 make build"

## Local (binary) -------------------------------------------------------------

build:
	@echo "Building bootimus $(VERSION)..."
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

run: build
	./$(BINARY) serve

clean:
	rm -f bootimus bootimus-*

## Local (container) ----------------------------------------------------------

docker-build:
	docker build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest \
		--build-arg VERSION=$(VERSION) .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

## Publish --------------------------------------------------------------------

release: clean
	@echo "Building release v$(VERSION)..."
	CGO_ENABLED=1 GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bootimus-linux-amd64 .
	CGO_ENABLED=1 GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bootimus-linux-arm64 .
	CGO_ENABLED=1 GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bootimus-darwin-amd64 .
	CGO_ENABLED=1 GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bootimus-darwin-arm64 .
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bootimus-windows-amd64.exe .
	@echo ""
	@echo "Release v$(VERSION) binaries built:"
	@ls -lh bootimus-*
	@echo ""
	@echo "Upload these to GitHub: Repo -> Releases -> Draft a new release -> Tag: v$(VERSION)"

PLATFORMS ?= linux/amd64,linux/arm64

docker-push:
	docker buildx create --use --name bootimus-builder --driver docker-container 2>/dev/null || docker buildx use bootimus-builder
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE):$(VERSION) \
		-t $(IMAGE):latest \
		--push .

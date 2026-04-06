.PHONY: help build run clean docker-build docker-up docker-down docker-push release binaries secureboot sync-profiles

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
	@echo "  make binaries         - Build multi-arch binaries via docker buildx"
	@echo "  make release          - Build binaries and show upload instructions"
	@echo "  make docker-push      - Build and push multi-arch images to Docker Hub"
	@echo ""
	@echo "Override version:  VERSION=1.0.0 make build"

secureboot:
	@echo "Downloading Secure Boot bootloaders..."
	./scripts/download-secureboot.sh

## Local (binary) -------------------------------------------------------------

sync-profiles:
	@cp distro-profiles.json internal/profiles/distro-profiles.json

build: sync-profiles
	@echo "Building bootimus $(VERSION)..."
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

run: sync-profiles
	CGO_ENABLED=1 go run -ldflags="$(LDFLAGS)" . serve

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

PLATFORMS ?= linux/amd64,linux/arm64

release: clean binaries
	@echo ""
	@echo "Release v$(VERSION) binaries built:"
	@ls -lh bootimus-*
	@echo ""
	@echo "Upload these to GitHub: Repo -> Releases -> Draft a new release -> Tag: v$(VERSION)"

binaries:
	@echo "Building binaries v$(VERSION) via docker buildx..."
	docker buildx create --use --name bootimus-builder --driver docker-container 2>/dev/null || docker buildx use bootimus-builder
	docker buildx build \
		--platform $(PLATFORMS) \
		--target binaries \
		--build-arg VERSION=$(VERSION) \
		--output type=local,dest=./dist .
	@# Flatten platform directories into release binaries
	@for dir in dist/*/; do \
		for f in "$$dir"bootimus-*; do \
			cp "$$f" "./"; \
		done; \
	done
	@rm -rf dist

docker-push:
	docker buildx create --use --name bootimus-builder --driver docker-container 2>/dev/null || docker buildx use bootimus-builder
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE):$(VERSION) \
		-t $(IMAGE):latest \
		--push .

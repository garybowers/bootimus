.PHONY: help build run clean docker-build docker-up docker-down docker-push release binaries bootloaders sync-profiles appliance appliance-amd64 appliance-arm64 test-appliance build-website push-website

VERSION     ?= $(shell cat VERSION)
DOCKER_USER ?= garybowers
IMAGE       := $(DOCKER_USER)/bootimus
LDFLAGS     := -w -s -X bootimus/internal/server.Version=$(VERSION)
BINARY      := bootimus

# --- Website (marketing site) publish to Google Artifact Registry ------------
# Versioned independently of the app — defaults to short git SHA for rollback.
# Override with:  WEBSITE_VERSION=v1.2 make push-website
GCP_PROJECT      ?= b1-services-230040
GCP_REGION       ?= europe-west2
GCP_REPO         ?= bootimus
WEBSITE_VERSION  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
WEBSITE_IMAGE    := $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(GCP_REPO)/website

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
	@echo "Bootloaders:"
	@echo "  make bootloaders      - Build iPXE and download Secure Boot binaries"
	@echo ""
	@echo "Appliance:"
	@echo "  make appliance        - Build USB image for the host's native arch"
	@echo "  make appliance-amd64  - Force amd64 (cross-build needs qemu-user-static on host)"
	@echo "  make appliance-arm64  - Force arm64 (cross-build needs qemu-user-static on host)"
	@echo "  make test-appliance   - Boot the amd64 image in QEMU (UI: http://localhost:18081)"
	@echo ""
	@echo "Publish:"
	@echo "  make binaries         - Build multi-arch binaries via docker buildx"
	@echo "  make release          - Build binaries and show upload instructions"
	@echo "  make docker-push      - Build and push multi-arch images to Docker Hub"
	@echo ""
	@echo "Website (marketing site -> Google Artifact Registry):"
	@echo "  make build-website    - Build amd64 image from repo root into GCP AR"
	@echo "  make push-website     - Build + push to $(GCP_REGION)-docker.pkg.dev"
	@echo ""
	@echo "Override version:  VERSION=1.0.0 make build"

bootloaders:
	@echo "Building iPXE bootloaders and downloading Secure Boot binaries..."
	./scripts/build-bootloaders.sh

HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

appliance:
	@echo "Building Bootimus USB appliance image for native arch ($(HOST_ARCH))..."
	APPLIANCE_ARCH=$(HOST_ARCH) ./appliance/build.sh

appliance-amd64:
	APPLIANCE_ARCH=amd64 ./appliance/build.sh

appliance-arm64:
	APPLIANCE_ARCH=arm64 ./appliance/build.sh

test-appliance:
	@IMG=$$(ls -t appliance/build/bootimus-appliance-*-amd64.img 2>/dev/null | head -1); \
	if [ -z "$$IMG" ]; then echo "No amd64 appliance image found. Run 'make appliance-amd64' first."; exit 1; fi; \
	echo "Booting $$IMG in QEMU (admin UI will be at http://localhost:18081)…"; \
	cp /usr/share/edk2-ovmf/x64/OVMF_VARS.4m.fd /tmp/bootimus-ovmf-vars.fd; \
	qemu-system-x86_64 \
	    -m 2G -smp 2 -enable-kvm \
	    -machine q35 \
	    -drive if=pflash,format=raw,readonly=on,file=/usr/share/edk2-ovmf/x64/OVMF_CODE.4m.fd \
	    -drive if=pflash,format=raw,file=/tmp/bootimus-ovmf-vars.fd \
	    -drive file=$$IMG,format=raw,if=virtio \
	    -netdev user,id=n0,hostfwd=tcp::18081-:8081,hostfwd=tcp::18080-:8080 \
	    -device virtio-net-pci,netdev=n0 \
	    -serial mon:stdio

## Local (binary) -------------------------------------------------------------

sync-profiles:
	@cp distro-profiles.json internal/profiles/distro-profiles.json
	@cp tools-profiles.json internal/tools/tools-profiles.json

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

release: sync-profiles clean binaries docker-push
	@echo ""
	@echo "Release v$(VERSION) artefacts built:"
	@ls -lh bootimus-* appliance/build/bootimus-appliance-$(VERSION)-*.img 2>/dev/null
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

## Website (marketing site) --------------------------------------------------

# Build context MUST be the repo root so docs/ is included (see website/Dockerfile).
# Cloud Run only needs amd64.

build-website:
	@echo "Building website image ($(WEBSITE_IMAGE):$(WEBSITE_VERSION))…"
	docker build --platform=linux/amd64 \
		-f website/Dockerfile \
		-t $(WEBSITE_IMAGE):$(WEBSITE_VERSION) \
		-t $(WEBSITE_IMAGE):latest .

push-website: build-website
	@echo "Pushing website image to $(GCP_REGION) Artifact Registry…"
	docker push $(WEBSITE_IMAGE):$(WEBSITE_VERSION)
	docker push $(WEBSITE_IMAGE):latest
	@echo ""
	@echo "Cloud Run image URL:"
	@echo "  $(WEBSITE_IMAGE):latest"

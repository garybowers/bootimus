.PHONY: help build run test test-integration test-postgres test-e2e e2e-binary test-ui openapi clean docker-build docker-up docker-down docker-push release binaries bootloaders sync-profiles bump appliance appliance-amd64 appliance-arm64 test-appliance build-website push-website deploy-website

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
CLOUDRUN_SERVICE ?= bootimus
WEBSITE_VERSION  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
WEBSITE_IMAGE    := $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(GCP_REPO)/website

# --- Integration tests --------------------------------------------------------
# Throwaway OIDC provider used by the auth integration tests.
OIDC_TEST_IMAGE     ?= ghcr.io/navikt/mock-oauth2-server:6.0.4
OIDC_TEST_PORT      ?= 18080
OIDC_TEST_CONTAINER := bootimus-oidc-test
PG_TEST_MAJOR       := $(shell pg_dump --version 2>/dev/null | grep -oE '[0-9]+' | head -1)
PG_TEST_IMAGE       ?= postgres:$(if $(PG_TEST_MAJOR),$(PG_TEST_MAJOR),16)
PG_TEST_PORT        ?= 15432
PG_TEST_CONTAINER   := bootimus-postgres-test
PG_TEST_DSN         := postgres://bootimus:bootimus@localhost:$(PG_TEST_PORT)/bootimus?sslmode=disable

## Help -----------------------------------------------------------------------

help:
	@echo "Bootimus Build System"
	@echo ""
	@echo "Local (binary):"
	@echo "  make build            - Build binary for current platform"
	@echo "  make run              - Build and run locally"
	@echo "  make clean            - Remove build artifacts"
	@echo "  make test             - Run unit tests"
	@echo "  make test-integration - Run integration tests against a throwaway OIDC provider (needs Docker)"
	@echo "  make test-postgres    - Run storage, server and admin tests against a throwaway PostgreSQL (needs Docker, pg_dump, psql)"
	@echo "  make test-e2e         - PXE boot QEMU VMs against a real bootimus (needs qemu-system-x86, OVMF; see CONTRIBUTING.md)"
	@echo "  make test-ui          - Playwright smoke tests of the admin UI against a real bootimus (needs Node and Chromium; see CONTRIBUTING.md)"
	@echo "  make openapi          - Write the OpenAPI document to docs/openapi.json"
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
	@echo "  make binaries         - Build linux + windows binaries (amd64, arm64)"
	@echo "  make release          - Build binaries, push Docker images and show upload instructions"
	@echo "  make docker-push      - Push multi-arch images (latest only for stable versions)"
	@echo ""
	@echo "Website (marketing site -> Google Artifact Registry):"
	@echo "  make build-website    - Build amd64 image from repo root into GCP AR"
	@echo "  make push-website     - Build + push to $(GCP_REGION)-docker.pkg.dev"
	@echo "  make deploy-website   - Push + roll Cloud Run service '$(CLOUDRUN_SERVICE)' to the new image"
	@echo ""
	@echo "Versioning:"
	@echo "  make bump NEW_VERSION=X.Y.Z  - Update VERSION, profile versions and the OpenAPI document"
	@echo ""
	@echo "Override version:  VERSION=1.0.0 make build"

test:
	go test ./...

openapi:
	go run -ldflags="$(LDFLAGS)" . openapi > docs/openapi.json

E2E_BIN := $(CURDIR)/.e2e/bootimus

e2e-binary:
	@mkdir -p .e2e
	go build -ldflags="$(LDFLAGS)" -o $(E2E_BIN) .

test-ui:
	@mkdir -p .ui-tests
	go build -ldflags="$(LDFLAGS)" -o $(CURDIR)/.ui-tests/bootimus .
	BOOTIMUS_UI_BINARY=$(CURDIR)/.ui-tests/bootimus ui-tests/run.sh

test-e2e:
	@if [ -z "$$BOOTIMUS_E2E_BINARY" ]; then $(MAKE) --no-print-directory e2e-binary; fi
	BOOTIMUS_E2E=1 BOOTIMUS_E2E_BINARY=$${BOOTIMUS_E2E_BINARY:-$(E2E_BIN)} go test -count=1 -v -timeout 25m ./e2e/

test-integration:
	@docker rm -f $(OIDC_TEST_CONTAINER) >/dev/null 2>&1 || true
	docker run -d --rm --name $(OIDC_TEST_CONTAINER) -p $(OIDC_TEST_PORT):8080 \
	    -e 'JSON_CONFIG={"interactiveLogin":true}' $(OIDC_TEST_IMAGE) >/dev/null
	@trap 'docker rm -f $(OIDC_TEST_CONTAINER) >/dev/null' EXIT; \
	echo "Waiting for OIDC provider on port $(OIDC_TEST_PORT)..."; \
	for i in $$(seq 1 60); do \
	    curl -sf http://localhost:$(OIDC_TEST_PORT)/bootimus/.well-known/openid-configuration >/dev/null && break; \
	    if [ $$i -eq 60 ]; then echo "OIDC provider did not start"; docker logs $(OIDC_TEST_CONTAINER); exit 1; fi; \
	    sleep 1; \
	done; \
	BOOTIMUS_TEST_OIDC_URL=http://localhost:$(OIDC_TEST_PORT) go test -count=1 -v -run Integration ./internal/auth/

test-postgres:
	@docker rm -f $(PG_TEST_CONTAINER) >/dev/null 2>&1 || true
	docker run -d --rm --name $(PG_TEST_CONTAINER) -p $(PG_TEST_PORT):5432 \
	    -e POSTGRES_USER=bootimus -e POSTGRES_PASSWORD=bootimus -e POSTGRES_DB=bootimus $(PG_TEST_IMAGE) >/dev/null
	@trap 'docker rm -f $(PG_TEST_CONTAINER) >/dev/null' EXIT; \
	echo "Waiting for PostgreSQL ($(PG_TEST_IMAGE)) on port $(PG_TEST_PORT)..."; \
	for i in $$(seq 1 60); do \
	    docker exec $(PG_TEST_CONTAINER) pg_isready -h 127.0.0.1 -U bootimus >/dev/null 2>&1 && break; \
	    if [ $$i -eq 60 ]; then echo "PostgreSQL did not start"; docker logs $(PG_TEST_CONTAINER); exit 1; fi; \
	    sleep 1; \
	done; \
	BOOTIMUS_TEST_POSTGRES_DSN='$(PG_TEST_DSN)' go test -count=1 -p 1 ./internal/storage/ ./internal/server/ ./internal/admin/

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
	@sed -i 's/"version": "[^"]*"/"version": "$(VERSION)"/' distro-profiles.json
	@sed -i 's/"version": "[^"]*"/"version": "$(VERSION)"/' tools-profiles.json
	@cp distro-profiles.json internal/profiles/distro-profiles.json
	@cp tools-profiles.json internal/tools/tools-profiles.json

bump: export RELEASE_VERSION = $(NEW_VERSION)
bump:
	@test -n "$$RELEASE_VERSION" || (echo "Usage: make bump NEW_VERSION=X.Y.Z" && exit 1)
	@scripts/release-metadata.sh "$$RELEASE_VERSION" >/dev/null
	@printf '%s\n' "$$RELEASE_VERSION" > VERSION
	@$(MAKE) --no-print-directory sync-profiles VERSION="$$RELEASE_VERSION"
	@$(MAKE) --no-print-directory openapi VERSION="$$RELEASE_VERSION"
	@echo "Bumped to $$RELEASE_VERSION: VERSION, profile versions and docs/openapi.json"

build: sync-profiles
	@echo "Building bootimus $(VERSION)..."
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

run: sync-profiles
	CGO_ENABLED=0 go run -ldflags="$(LDFLAGS)" . serve

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
	@ls -lh bootimus-* appliance/build/bootimus-appliance-$(VERSION)-*.img 2>/dev/null || true
	@echo ""
	@echo "Upload these to GitHub: Repo -> Releases -> Draft a new release -> Tag: v$(VERSION)"

BINARY_TARGETS ?= linux/amd64 linux/arm64 windows/amd64 windows/arm64

binaries: sync-profiles
	@echo "Building binaries v$(VERSION)..."
	@for target in $(BINARY_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		out=bootimus-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then out=$$out.exe; fi; \
		printf '  %-20s -> %s\n' "$$os/$$arch" "$$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags="$(LDFLAGS)" -o "$$out" . || exit 1; \
	done
	@echo "Done."
	@ls -lh bootimus-*

docker-push: export RELEASE_VERSION = $(VERSION)
docker-push: export RELEASE_IMAGE = $(IMAGE)
docker-push:
	@set -eu; \
	metadata="$$(scripts/release-metadata.sh "$$RELEASE_VERSION")"; \
	set -- -t "$$RELEASE_IMAGE:$$RELEASE_VERSION"; \
	if printf '%s\n' "$$metadata" | grep -qx 'latest=true'; then \
		set -- "$$@" -t "$$RELEASE_IMAGE:latest"; \
	fi; \
	docker buildx create --use --name bootimus-builder --driver docker-container 2>/dev/null || docker buildx use bootimus-builder; \
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg "VERSION=$$RELEASE_VERSION" \
		"$$@" \
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

deploy-website: push-website
	@echo "Rolling Cloud Run service '$(CLOUDRUN_SERVICE)' in $(GCP_REGION) to $(WEBSITE_IMAGE):$(WEBSITE_VERSION)…"
	gcloud run services update $(CLOUDRUN_SERVICE) \
		--project=$(GCP_PROJECT) \
		--region=$(GCP_REGION) \
		--image=$(WEBSITE_IMAGE):$(WEBSITE_VERSION)
	@echo "Deployed. Live at the Cloud Run service URL."

.PHONY: build run push

DOCKER_USER ?= garybowers
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -w -s -X bootimus/internal/server.Version=$(VERSION)

build:
	CGO_ENABLED=1 GOOS=linux go build -ldflags="$(LDFLAGS)" -o bootimus .

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


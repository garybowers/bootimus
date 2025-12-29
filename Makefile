.PHONY: build run push

DOCKER_USER ?= garybowers
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -w -s -X bootimus/internal/server.Version=$(VERSION)

build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="$(LDFLAGS)" -o bootimus .

run:
	docker-compose up -d

push:
	docker build -f Dockerfile.multistage --build-arg VERSION=$(VERSION) -t $(DOCKER_USER)/bootimus:$(VERSION) .
	docker tag $(DOCKER_USER)/bootimus:$(VERSION) $(DOCKER_USER)/bootimus:latest
	docker push $(DOCKER_USER)/bootimus:$(VERSION)
	docker push $(DOCKER_USER)/bootimus:latest


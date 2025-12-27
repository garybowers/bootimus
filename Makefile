.PHONY: build docker release

DOCKER_USER ?= youruser
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -w -s -X bootimus/cmd.version=$(VERSION)

build:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags="$(LDFLAGS)" -o bootimus .
	docker build -t $(DOCKER_USER)/bootimus:$(VERSION) .
	docker tag $(DOCKER_USER)/bootimus:$(VERSION) $(DOCKER_USER)/bootimus:latest

docker:
	docker push $(DOCKER_USER)/bootimus:$(VERSION)
	docker push $(DOCKER_USER)/bootimus:latest

release:
	gh release create $(VERSION) bootimus --title "Release $(VERSION)" --notes "Release $(VERSION)"

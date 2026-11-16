BINARY  := dossier
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= wllengineering/dossier
LDFLAGS := -s -w -X dossier/internal/cli.version=$(VERSION)

.PHONY: build run test vet fmt clean web docker docker-push

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: build
	./bin/$(BINARY) $(ARGS)

web:
	go run ./web

docker:
	docker build -f web/Dockerfile --build-arg VERSION=$(VERSION) -t $(BINARY)-web .

# Builds amd64 and arm64 images and pushes them to Docker Hub as
# $(IMAGE):$(VERSION) and $(IMAGE):latest. Needs `docker login` first.
# Uses Podman's manifest commands (the docker CLI here is Podman).
docker-push:
	-docker manifest rm $(IMAGE):$(VERSION) 2>/dev/null
	docker build -f web/Dockerfile --build-arg VERSION=$(VERSION) \
		--platform linux/amd64,linux/arm64 --manifest $(IMAGE):$(VERSION) .
	docker manifest push --all $(IMAGE):$(VERSION) docker://docker.io/$(IMAGE):$(VERSION)
	docker manifest push --all $(IMAGE):$(VERSION) docker://docker.io/$(IMAGE):latest

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

clean:
	rm -rf bin

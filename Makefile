BINARY  := dossier
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X dossier/internal/cli.version=$(VERSION)

.PHONY: build run test vet fmt clean web docker

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: build
	./bin/$(BINARY) $(ARGS)

web:
	go run ./web

docker:
	docker build -f web/Dockerfile --build-arg VERSION=$(VERSION) -t $(BINARY)-web .

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

clean:
	rm -rf bin

BINARY  := dossier
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X dossier/internal/cli.version=$(VERSION)

.PHONY: build run test vet fmt clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: build
	./bin/$(BINARY) $(ARGS)

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

clean:
	rm -rf bin

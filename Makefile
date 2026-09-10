# mcpeach — local MCP gateway with TUI

.PHONY: build test lint vet fmt clean install

BINARY := mcpeach
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/mcpeach

test:
	go test ./... -race -cover

lint:
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed; skipping"

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/mcpeach

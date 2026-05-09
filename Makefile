# 8l-cli Makefile

BINARY_NAME ?= 8l
PKG          := github.com/OneZero1ai/8l-cli
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-dev")
COMMIT       ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE         ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X $(PKG)/pkg/version.Version=$(VERSION) \
           -X $(PKG)/pkg/version.Commit=$(COMMIT) \
           -X $(PKG)/pkg/version.BuildDate=$(DATE)

.PHONY: all build test vet fmt fmt-check tidy clean lint install

all: fmt vet test build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/8l

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/8l

test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

test-short:
	go test -short ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@diff=$$(gofmt -l . 2>&1); if [ -n "$$diff" ]; then echo "gofmt diffs:"; echo "$$diff"; exit 1; fi

tidy:
	go mod tidy

lint: fmt-check vet

clean:
	rm -f $(BINARY_NAME) coverage.txt

# Cross-compile for release (V1: macOS + Linux, amd64 + arm64)
.PHONY: release-build
release-build:
	mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64 ./cmd/8l
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64 ./cmd/8l
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64  ./cmd/8l
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64  ./cmd/8l

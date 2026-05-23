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

# Cross-compile for release (macOS + Linux + Windows, amd64 + arm64)
.PHONY: release-build release-tarballs
release-build:
	mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64      ./cmd/8l
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64      ./cmd/8l
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64       ./cmd/8l
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64       ./cmd/8l
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-arm64.exe ./cmd/8l
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/8l

# Produce the exact tarball layout the CodeBuild pipeline publishes to
# s3://8l-cli-releases-.../8l/vX.Y.Z/. The installer at
# install.8th-layer.ai consumes these names verbatim — keep this target
# and ci/buildspecs/cli-release.yml in sync. Windows ships as .zip; the
# four Unix targets as .tar.gz.
release-tarballs:
	rm -rf dist && mkdir -p dist
	@for pair in "Darwin arm64 darwin arm64" "Darwin x86_64 darwin amd64" "Linux arm64 linux arm64" "Linux x86_64 linux amd64"; do \
	  set -- $$pair; \
	  GOOS=$$3 GOARCH=$$4 go build -ldflags "$(LDFLAGS)" -o /tmp/$(BINARY_NAME) ./cmd/8l; \
	  tar -C /tmp -czf dist/$(BINARY_NAME)_$$1_$$2.tar.gz $(BINARY_NAME); \
	  rm /tmp/$(BINARY_NAME); \
	done
	@for pair in "x86_64 amd64" "arm64 arm64"; do \
	  set -- $$pair; \
	  GOOS=windows GOARCH=$$2 go build -ldflags "$(LDFLAGS)" -o /tmp/$(BINARY_NAME).exe ./cmd/8l; \
	  ( cd /tmp && zip -q $(CURDIR)/dist/$(BINARY_NAME)_Windows_$$1.zip $(BINARY_NAME).exe ); \
	  rm /tmp/$(BINARY_NAME).exe; \
	done
	cd dist && sha256sum $(BINARY_NAME)_* > SHA256SUMS
	@ls -la dist
	@echo "--- SHA256SUMS ---"
	@cat dist/SHA256SUMS

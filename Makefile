BINARY  := envseal
PKG     := github.com/PeacexF/envseal
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)

.DEFAULT_GOAL := help
.PHONY: help build test cover vet fmt check install clean

help:
	@echo 'envseal — make targets'
	@echo
	@awk -F: '/^# /{ d = substr($$0, 3); next } \
		/^[a-z][a-z-]*:/ { if (d != "") printf "  \033[1m%-9s\033[0m %s\n", $$1, d; d = "" } \
		/^$$/ { d = "" }' $(MAKEFILE_LIST)
	@echo
	@echo 'Version: $(VERSION)  (override with `make build VERSION=v1.0.0`)'

# Build bin/envseal with version info baked in
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/envseal

# Run all tests
test:
	go test ./...

# Run tests and report total coverage
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

# Run go vet
vet:
	go vet ./...

# Format all Go source in place
fmt:
	gofmt -l -w .

# Run what CI runs
check: vet test
	@test -z "$$(gofmt -l .)" || { echo 'not gofmt'"'"'d:'; gofmt -l .; exit 1; }

# Install envseal into $(GOPATH)/bin
install:
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/envseal

# Remove build artifacts
clean:
	rm -rf bin coverage.out

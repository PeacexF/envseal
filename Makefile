BINARY  := envseal
PKG     := github.com/PeacexF/envseal
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)

.DEFAULT_GOAL := help
.PHONY: help build test cover vet fmt check install clean

help:
	@echo 'envseal — make targets'
	@echo
	@grep -hE '^[a-z-]+:.*##' $(MAKEFILE_LIST) | \
		awk -F':.*##' '{ printf "  \033[1m%-9s\033[0m %s\n", $$1, $$2 }'
	@echo
	@echo 'Version: $(VERSION)  (override with `make build VERSION=v1.0.0`)'

# bin/envseal with version info
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/envseal

# tests
test:
	go test ./...

# coverage
cover: 
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt: 
	gofmt -l -w .

# ci
check: vet test 

# Install envseal into $(GOPATH)/bin
install:
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/envseal

# Remove build artifacts
clean:
	rm -rf bin coverage.out

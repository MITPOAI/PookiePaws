# PookiePaws development Makefile
# Run targets via `make <target>` (Linux/macOS/git-bash) or `mingw32-make <target>` (Windows).

.DEFAULT_GOAL := help
SHELL := /bin/bash

GO         ?= go
GOFMT      ?= gofmt
BIN_DIR    ?= ./bin
BIN        := $(BIN_DIR)/pookie
AD_BIN     := $(BIN_DIR)/pookiepaws
LDFLAGS    ?= -s -w
PKGS       := ./...

.PHONY: help build build-pookiepaws test test-race vet fmt lint smoke release-snapshot clean install-tools

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Compile pookie binary into ./bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN) ./cmd/pookie

build-pookiepaws: ## Compile pookiepaws ad automation CLI into ./bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(AD_BIN) ./cmd/pookiepaws

test: ## Run all tests (no race detector)
	$(GO) test $(PKGS)

test-race: ## Run tests with race detector (requires gcc)
	$(GO) test -race $(PKGS)

vet: ## Run go vet
	$(GO) vet $(PKGS)

fmt: ## Format Go sources with gofmt
	$(GOFMT) -w cmd internal

lint: vet ## Alias for vet (extend with golangci-lint if installed)

smoke: build ## Run operator smoke checks against the built binary
	$(BIN) smoke

release-snapshot: ## Build a snapshot release with goreleaser (requires goreleaser)
	goreleaser release --snapshot --clean

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist

install-tools: ## Install dev tools (currently goreleaser hint)
	@echo "Install goreleaser from https://goreleaser.com/install/"

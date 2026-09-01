GO ?= go
GOBIN_DIR := $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT_VERSION := v2.13.2

.PHONY: all build vet test lint govulncheck

all: build vet lint test

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

lint:
	@$(GOBIN_DIR)/golangci-lint version 2>/dev/null | grep -qF "version $(GOLANGCI_LINT_VERSION:v%=%) " || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOBIN_DIR) $(GOLANGCI_LINT_VERSION)
	$(GOBIN_DIR)/golangci-lint run

govulncheck:
	@test -x $(GOBIN_DIR)/govulncheck || GOBIN=$(GOBIN_DIR) $(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GOBIN_DIR)/govulncheck ./...

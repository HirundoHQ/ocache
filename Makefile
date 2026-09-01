GO ?= go

.PHONY: all build vet test lint govulncheck

all: build vet lint test

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$($(GO) env GOPATH)/bin v2.13.1
	golangci-lint run

govulncheck:
	@command -v govulncheck >/dev/null 2>&1 || $(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

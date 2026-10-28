GO ?= go
GOLANGCI = go tool -modfile=tools/go.mod golangci-lint
GORELEASER = go run github.com/goreleaser/goreleaser/v2@v2.19.0

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
FUZZTIME ?= 30s
# The 12 Turkish letters as UTF-8 byte sequences (octal), so this file stays ASCII.
TR_CHARS := $(shell printf '\303\247|\303\207|\304\237|\304\236|\304\261|\304\260|\303\266|\303\226|\305\237|\305\236|\303\274|\303\234')

.DEFAULT_GOAL := help

.PHONY: help build install test cover fuzz lint fmt fmt-check vuln english check tidy release-check snapshot demo screenshots clean

## help: show this help
help:
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'

## build: build bin/gdrift with version information
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/gdrift ./cmd/gdrift

## install: install gdrift into GOBIN with version information
install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/gdrift

## test: run all tests with the race detector and shuffled order
test:
	$(GO) test -race -shuffle=on ./...

## cover: write coverage.out and coverage.html
cover:
	$(GO) test -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## fuzz: fuzz the porcelain parsers (FUZZTIME=30s)
fuzz:
	$(GO) test -run=^$$ -fuzz=FuzzParseStatusV2 -fuzztime=$(FUZZTIME) ./internal/repo
	$(GO) test -run=^$$ -fuzz=FuzzParseLog -fuzztime=$(FUZZTIME) ./internal/repo

## lint: run golangci-lint
lint:
	$(GOLANGCI) run ./...

## fmt: format the code
fmt:
	$(GOLANGCI) fmt ./...

## fmt-check: fail when the code is not formatted
fmt-check:
	$(GOLANGCI) fmt --diff ./...

## vuln: scan dependencies for known vulnerabilities
vuln:
	$(GO) tool -modfile=tools/go.mod govulncheck ./...

## english: fail when non-English characters are found outside the docs
english:
	@if grep -rnIE '$(TR_CHARS)' --exclude-dir=.git --exclude-dir=.agents --exclude-dir=.claude --exclude-dir=.superpowers --exclude-dir=superpowers .; then \
		echo "non-English characters found"; exit 1; \
	fi

## check: run every check
check: fmt-check lint test vuln english

## tidy: tidy both modules
tidy:
	$(GO) mod tidy
	cd tools && $(GO) mod tidy

## release-check: validate the GoReleaser configuration
release-check:
	$(GORELEASER) check

## snapshot: build a local release snapshot
snapshot:
	$(GORELEASER) release --snapshot --clean

## demo: build the demo workspace in ./demo (replaces a previous one)
demo:
	rm -rf demo
	scripts/demo-workspace.sh demo

## screenshots: render assets/images with vhs (needs vhs, ttyd and ffmpeg)
screenshots: build
	for tape in assets/vhs/*.tape; do vhs "$$tape" || exit 1; done

## clean: remove build artifacts
clean:
	rm -rf bin dist demo coverage.out coverage.html

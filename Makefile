.DEFAULT_GOAL := help

BINARY_NAME := bumpit
MAIN_PACKAGE := ./cmd/bumpit
BUILD_DIR := bin
VERSION_VAR := github.com/pragmabits/bumpit/internal/app.buildVersion

VERSION ?= $(shell git describe --tags --always --dirty --match 'v*' 2>/dev/null || printf 'dev')
LDFLAGS := -X $(VERSION_VAR)=$(VERSION)

RELEASE_TARGETS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64
RELEASE_BUILD_TARGETS := build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64

.PHONY: help fmt test lint tidy build install run clean release build-target $(RELEASE_BUILD_TARGETS)

help:
	@printf '%s\n' \
		'make build        Build the local binary into ./bin with buildVersion ldflags' \
		'make install      Install the CLI with buildVersion ldflags' \
		'make run          Run the CLI locally, pass ARGS="..." to forward arguments' \
		'make test         Run the Go test suite' \
		'make lint         Run golangci-lint with .golangci.yml' \
		'make fmt          Format all Go files' \
		'make tidy         Run go mod tidy' \
		'make clean        Remove build artifacts' \
		'make release      Build a small cross-platform release matrix into ./bin' \
		'make build-linux-amd64   Build a specific release artifact'

fmt:
	@gofmt -w $$(find . -name '*.go' -type f)

test:
	@go test ./...

lint:
	@golangci-lint run ./...

tidy:
	@go mod tidy

build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "$(LDFLAGS)" -o "$(BUILD_DIR)/$(BINARY_NAME)" $(MAIN_PACKAGE)

install:
	@go install -ldflags "$(LDFLAGS)" $(MAIN_PACKAGE)

run: build
	@./$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

clean:
	@rm -rf $(BUILD_DIR)

release: $(RELEASE_BUILD_TARGETS)

build-target:
	@mkdir -p $(BUILD_DIR)
	@extension=''; \
	if [ "$(GOOS)" = windows ]; then extension='.exe'; fi; \
	GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build -ldflags "$(LDFLAGS)" -o "$(BUILD_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$$extension" $(MAIN_PACKAGE)

build-linux-amd64:
	@$(MAKE) build-target GOOS=linux GOARCH=amd64

build-linux-arm64:
	@$(MAKE) build-target GOOS=linux GOARCH=arm64

build-darwin-amd64:
	@$(MAKE) build-target GOOS=darwin GOARCH=amd64

build-darwin-arm64:
	@$(MAKE) build-target GOOS=darwin GOARCH=arm64

build-windows-amd64:
	@$(MAKE) build-target GOOS=windows GOARCH=amd64

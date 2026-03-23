.PHONY: all version fmt lint test coverage benchmark air deps gen release clean build help

BINARY_NAME := $(shell basename $(PWD))
GIT_VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "0.0.0")
VERSION ?= $(GIT_VERSION)
DOCKER_CR ?= $(shell basename $$(dirname $(PWD)))
DOCKER_IMAGE := ${DOCKER_CR}/$(BINARY_NAME):$(VERSION)

all: fmt gen lint coverage ## Run all tests and checks

version: ## Display current version
	@echo "Current version: $(VERSION)"

fmt: ## Format code
	golangci-lint fmt

lint: ## Run linter
	golangci-lint run --timeout=5m

test: ## Run tests
	go test -race -shuffle=on -count=1 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...

coverage: test ## Generate coverage
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html

benchmark: ## Run benchmarks
	go test -run=^$$ -bench=. -benchmem ./... | tee benchmark.txt

air: ## Run development server
	@command -v air >/dev/null 2>&1 || { \
      echo "Please install air: go install github.com/air-verse/air@latest"; \
      exit 1; \
    }
	@echo "Starting development server with air..."
	TZ=UTC DEBUG=1 air

deps: ## Install dependencies
	go mod download

gen: ## Generate code
	go generate ./...

release: ## Create release
	goreleaser release --snapshot --clean

clean: ## Clean build artifacts
	rm -f coverage.* benchmark.txt
	rm -rf dist bin

build: ## Build the project
	@echo "Building the project..."
	@mkdir -p bin
	go build -o bin/$(BINARY_NAME) .

help: ## Show help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Use the wildcard function to expand the pattern to a list of existing files
# and then include that list of files.
include $(wildcard *.mk)

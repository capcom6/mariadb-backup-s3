# Makefile for mariadb-backup-s3 CLI tool

.PHONY: all build test clean release lint

all: build

# Build the binary
build:
	go build -o mariadb-backup-s3 .

# Run all tests
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

# Clean build artifacts and temporary files
clean:
	rm -f mariadb-backup-s3
	go clean -cache -testcache
	find . -type f -name "*.out" -delete

# Execute goreleaser for versioned releases
release:
	goreleaser release --snapshot --clean

# Run golangci-lint
lint:
	golangci-lint run --timeout=5m
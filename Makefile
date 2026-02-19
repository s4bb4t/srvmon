BINARY_NAME := srvmon
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
SERVICE_NAME := srvmon

.PHONY: all build build-cli run test lint clean docker proto swagger generate deps help

# Default target
all: generate build test lint

# Build the binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/$(BINARY_NAME) ./example

# Build the CLI client
build-cli:
	go build -ldflags="-s -w" -o bin/$(BINARY_NAME)-cli ./cmd/srvmon-cli

install:
	go install $(LDFLAGS) ./cmd/srvmon-cli

# Run the example
run: build
	./bin/$(BINARY_NAME)

# Run tests
test:
	go test -race -cover -coverprofile=coverage.out ./...

# Run tests with verbose output
test-verbose:
	go test -race -cover -v ./...

# Generate coverage report
coverage: test
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

check: lint vet fmt

# Run linter
lint:
	golangci-lint run

fmt:
	go fmt ./...
	gofmt -s -w .

vet:
	go vet ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f ./srvmon-cli

# Generate protobuf code
proto:
	rm -rf pkg/grpc
	mkdir -p pkg/grpc/$(SERVICE_NAME)/v1
	protoc -I api/proto \
		--go_out=pkg/grpc/$(SERVICE_NAME) --go_opt=paths=source_relative \
		--go-grpc_out=pkg/grpc/$(SERVICE_NAME) --go-grpc_opt=paths=source_relative \
		api/proto/v1/srvmon.proto

# Generate all code
generate: proto

# Install dependencies
deps:
	go mod download
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-swagger/go-swagger/cmd/swagger@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest


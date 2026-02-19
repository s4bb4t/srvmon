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

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f ./srvmon-cli

# Build Docker image
docker:
	docker build -t $(BINARY_NAME):$(VERSION) -f deploy/docker/Dockerfile .

# Generate protobuf code
proto:
	rm -rf pkg/grpc
	mkdir -p pkg/grpc/$(SERVICE_NAME)/v1
	protoc -I api/proto \
		--go_out=pkg/grpc/$(SERVICE_NAME) --go_opt=paths=source_relative \
		--go-grpc_out=pkg/grpc/$(SERVICE_NAME) --go-grpc_opt=paths=source_relative \
		api/proto/v1/srvmon.proto

# Generate protobuf code with buf
proto-buf:
	buf generate

# Validate OpenAPI spec
swagger-validate:
	swagger validate api/swagger/swagger.yaml

# Generate Go client from OpenAPI
swagger-client:
	swagger generate client -f api/swagger/swagger.yaml -t pkg/rest

# Generate Go server from OpenAPI
swagger-server:
	swagger generate server -f api/swagger/v1/srvmon.yaml -t pkg/rest

# Generate all code
generate: proto

# Install dependencies
deps:
	go mod download
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-swagger/go-swagger/cmd/swagger@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install protobuf dependencies (macOS)
deps-proto-mac:
	brew install protobuf
	brew install buf

# Install protobuf dependencies (Linux)
deps-proto-linux:
	sudo apt-get install -y protobuf-compiler
	go install github.com/bufbuild/buf/cmd/buf@latest

# Update go.mod dependencies
update:
	go get -u ./...
	go mod tidy

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .

# Run example with hot reload (requires air)
dev:
	air -c .air.toml

# Benchmark tests
bench:
	go test -bench=. -benchmem ./...

# Check for vulnerabilities
vuln:
	govulncheck ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  all            - Generate, build, test, and lint"
	@echo "  build          - Build the binary"
	@echo "  run            - Build and run the example"
	@echo "  test           - Run tests with coverage"
	@echo "  test-verbose   - Run tests with verbose output"
	@echo "  coverage       - Generate HTML coverage report"
	@echo "  lint           - Run golangci-lint"
	@echo "  clean          - Remove build artifacts"
	@echo "  docker         - Build Docker image"
	@echo "  proto          - Generate protobuf code"
	@echo "  proto-buf      - Generate protobuf code with buf"
	@echo "  swagger-validate - Validate OpenAPI spec"
	@echo "  swagger-client - Generate Go client from OpenAPI"
	@echo "  swagger-server - Generate Go server from OpenAPI"
	@echo "  generate       - Generate all code"
	@echo "  deps           - Install Go dependencies"
	@echo "  deps-proto-mac - Install protobuf (macOS)"
	@echo "  deps-proto-linux - Install protobuf (Linux)"
	@echo "  update         - Update dependencies"
	@echo "  fmt            - Format code"
	@echo "  dev            - Run with hot reload"
	@echo "  bench          - Run benchmarks"
	@echo "  vuln           - Check for vulnerabilities"
	@echo "  help           - Show this help"

# srvmon

Production-grade service health monitoring package for Go with gRPC and REST support.

[![Go Reference](https://pkg.go.dev/badge/github.com/s4bb4t/srvmon.svg)](https://pkg.go.dev/github.com/s4bb4t/srvmon)
[![Go Report Card](https://goreportcard.com/badge/github.com/s4bb4t/srvmon)](https://goreportcard.com/report/github.com/s4bb4t/srvmon)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- **Dual Protocol Support** — Both gRPC and REST/HTTP endpoints
- **Kubernetes Ready** — `/healthz`, `/readyz` endpoints out of the box
- **Pluggable Health Checks** — Register custom checkers implementing the `Checker` interface
- **Critical Dependencies** — Mark checkers as critical to fail fast on dependency issues
- **Status Aggregation** — Intelligent status aggregation (UP, DOWN, DEGRADED)
- **Graceful Shutdown** — Clean shutdown with context cancellation
- **Observable** — Structured logging with zap, OpenTelemetry instrumentation for gRPC

## Installation

**Library:**

```bash
go get github.com/s4bb4t/srvmon
```

**CLI client:**

```bash
go install github.com/s4bb4t/srvmon/cmd/srvmon-cli@latest
```

## Quick Start

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/s4bb4t/srvmon"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // Create configuration
    cfg := srvmon.Config{
        Version:     "1.0.0",
        GRPCAddress: ":50051",
        HTTPAddress: ":8080",
    }

    // Create monitor
    monitor := srvmon.New(cfg, logger)

    // Add health checkers
    monitor.AddDependencies(
        NewPingChecker("redis", "localhost:6379", 5*time.Second, true),    // critical
        NewPingChecker("postgres", "localhost:5432", 5*time.Second, true), // critical
        NewHTTPChecker("api", "https://api.example.com/health", false),    // non-critical
    )

    // Setup graceful shutdown
    ctx, cancel := context.WithCancel(context.Background())
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        cancel()
    }()

    // Mark service as ready after initialization
    monitor.SetReady()

    // Run blocks until context is canceled
    monitor.Run(ctx)
}
```

## Core Concepts

### The Checker Interface

Every health check must implement the `Checker` interface:

```go
type Checker interface {
    // MustOK returns true if this is a critical dependency.
    // If a critical dependency fails, the service status becomes DOWN.
    // If a non-critical dependency fails, the service status becomes DEGRADED.
    MustOK(ctx context.Context) bool

    // Check performs the health check and returns the result.
    Check(ctx context.Context) (*pb.CheckResult, error)
}
```

### Health Status Values

| Status | Value | Description |
|--------|-------|-------------|
| `STATUS_UP` | 1 | Component is healthy |
| `STATUS_DOWN` | 2 | Component is unhealthy |
| `STATUS_DEGRADED` | 3 | Works with reduced capacity |
| `STATUS_UNKNOWN` | 4 | Status cannot be determined |

### Status Aggregation

```
Service starts with STATUS_UP

For each registered checker:
  ├─ If check returns STATUS_DOWN:
  │   ├─ MustOK() == true  → Service = STATUS_DOWN
  │   └─ MustOK() == false → Service = STATUS_DEGRADED
  └─ Otherwise: no change
```

## API Endpoints

### REST/HTTP

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Full health report with all dependency checks |
| `GET /healthz` | Kubernetes-style health endpoint (alias for /health) |
| `GET /ready` | Readiness probe |
| `GET /readyz` | Kubernetes-style readiness endpoint (alias for /ready) |

### gRPC Service

```protobuf
service srvmon {
  // Health returns the overall health status of the service.
  rpc Health(HealthRequest) returns (HealthResponse);

  // Ready indicates if the service is ready to accept traffic.
  rpc Ready(ReadinessRequest) returns (ReadinessResponse);
}
```

## Example Checkers

### TCP Ping Checker

Check TCP connectivity to any host:port:

```go
package checkers

import (
    "context"
    "net"
    "time"

    pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type PingChecker struct {
    name     string
    addr     string
    timeout  time.Duration
    critical bool
}

func NewPingChecker(name, addr string, timeout time.Duration, critical bool) *PingChecker {
    return &PingChecker{
        name:     name,
        addr:     addr,
        timeout:  timeout,
        critical: critical,
    }
}

func (c *PingChecker) MustOK(_ context.Context) bool {
    return c.critical
}

func (c *PingChecker) Check(ctx context.Context) (*pb.CheckResult, error) {
    result := &pb.CheckResult{
        Name:      c.name,
        Timestamp: timestamppb.Now(),
    }

    conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
    if err != nil {
        result.Status = pb.Status_STATUS_DOWN
        result.Message = "connection failed"
        result.Error = err.Error()
        return result, nil
    }
    defer conn.Close()

    result.Status = pb.Status_STATUS_UP
    result.Message = "connection successful"
    return result, nil
}
```

**Usage:**

```go
// Critical: service cannot function without Redis
monitor.AddDependencies(NewPingChecker("redis", "localhost:6379", 5*time.Second, true))

// Non-critical: service degrades gracefully without metrics
monitor.AddDependencies(NewPingChecker("metrics", "localhost:9090", 2*time.Second, false))
```

### HTTP Health Checker

Check HTTP endpoint health:

```go
package checkers

import (
    "context"
    "fmt"
    "net/http"
    "time"

    pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type HTTPChecker struct {
    name     string
    url      string
    client   *http.Client
    critical bool
}

func NewHTTPChecker(name, url string, timeout time.Duration, critical bool) *HTTPChecker {
    return &HTTPChecker{
        name:     name,
        url:      url,
        client:   &http.Client{Timeout: timeout},
        critical: critical,
    }
}

func (c *HTTPChecker) MustOK(_ context.Context) bool {
    return c.critical
}

func (c *HTTPChecker) Check(ctx context.Context) (*pb.CheckResult, error) {
    result := &pb.CheckResult{
        Name:      c.name,
        Timestamp: timestamppb.Now(),
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
    if err != nil {
        result.Status = pb.Status_STATUS_DOWN
        result.Error = err.Error()
        return result, nil
    }

    resp, err := c.client.Do(req)
    if err != nil {
        result.Status = pb.Status_STATUS_DOWN
        result.Message = "request failed"
        result.Error = err.Error()
        return result, nil
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        result.Status = pb.Status_STATUS_UP
        result.Message = fmt.Sprintf("status %d", resp.StatusCode)
    } else {
        result.Status = pb.Status_STATUS_DOWN
        result.Message = fmt.Sprintf("unhealthy status %d", resp.StatusCode)
    }

    return result, nil
}
```

**Usage:**

```go
// Check external API health
monitor.AddDependencies(NewHTTPChecker(
    "payment-api",
    "https://payments.example.com/health",
    10*time.Second,
    true, // critical - can't process orders without payments
))
```

### SQL Database Checker

Check database connectivity:

```go
package checkers

import (
    "context"
    "database/sql"
    "time"

    pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type SQLChecker struct {
    name     string
    db       *sql.DB
    timeout  time.Duration
    critical bool
}

func NewSQLChecker(name string, db *sql.DB, timeout time.Duration, critical bool) *SQLChecker {
    return &SQLChecker{
        name:     name,
        db:       db,
        timeout:  timeout,
        critical: critical,
    }
}

func (c *SQLChecker) MustOK(_ context.Context) bool {
    return c.critical
}

func (c *SQLChecker) Check(ctx context.Context) (*pb.CheckResult, error) {
    result := &pb.CheckResult{
        Name:      c.name,
        Timestamp: timestamppb.Now(),
    }

    ctx, cancel := context.WithTimeout(ctx, c.timeout)
    defer cancel()

    if err := c.db.PingContext(ctx); err != nil {
        result.Status = pb.Status_STATUS_DOWN
        result.Message = "database unreachable"
        result.Error = err.Error()
        return result, nil
    }

    result.Status = pb.Status_STATUS_UP
    result.Message = "database healthy"
    return result, nil
}
```

**Usage:**

```go
db, _ := sql.Open("postgres", "postgres://user:pass@localhost/mydb?sslmode=disable")
monitor.AddDependencies(NewSQLChecker("postgres", db, 5*time.Second, true))
```

### Redis Checker

Check Redis connectivity (works with go-redis):

```go
package checkers

import (
    "context"
    "time"

    pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
    "google.golang.org/protobuf/types/known/timestamppb"
)

// Pinger interface for Redis client compatibility
type Pinger interface {
    Ping(ctx context.Context) error
}

type RedisChecker struct {
    name     string
    client   Pinger
    timeout  time.Duration
    critical bool
}

func NewRedisChecker(name string, client Pinger, timeout time.Duration, critical bool) *RedisChecker {
    return &RedisChecker{
        name:     name,
        client:   client,
        timeout:  timeout,
        critical: critical,
    }
}

func (c *RedisChecker) MustOK(_ context.Context) bool {
    return c.critical
}

func (c *RedisChecker) Check(ctx context.Context) (*pb.CheckResult, error) {
    result := &pb.CheckResult{
        Name:      c.name,
        Timestamp: timestamppb.Now(),
    }

    ctx, cancel := context.WithTimeout(ctx, c.timeout)
    defer cancel()

    if err := c.client.Ping(ctx); err != nil {
        result.Status = pb.Status_STATUS_DOWN
        result.Message = "redis unreachable"
        result.Error = err.Error()
        return result, nil
    }

    result.Status = pb.Status_STATUS_UP
    result.Message = "redis healthy"
    return result, nil
}
```

**Usage with go-redis:**

```go
import "github.com/redis/go-redis/v9"

// Wrapper to match Pinger interface
type redisWrapper struct {
    *redis.Client
}

func (r *redisWrapper) Ping(ctx context.Context) error {
    return r.Client.Ping(ctx).Err()
}

// Usage
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
monitor.AddDependencies(NewRedisChecker("redis", &redisWrapper{rdb}, 5*time.Second, true))
```

## Configuration

```go
type Config struct {
    Version     string // Service version (returned in health responses)
    GRPCAddress string // gRPC listen address (e.g., ":50051")
    HTTPAddress string // HTTP listen address (e.g., ":8080")
}
```

### Server Configuration

**gRPC Server:**
| Setting | Value |
|---------|-------|
| Max concurrent streams | 10 |
| Max message size | 4 MB |
| Keepalive idle | 1 minute |
| Keepalive interval | 5 seconds |
| OpenTelemetry | Enabled |

**HTTP Server:**
| Setting | Value |
|---------|-------|
| Read timeout | 5 seconds |
| Write timeout | 5 seconds |
| Idle timeout | 10 seconds |

## Response Examples

### GET /health

**Healthy service:**

```json
{
  "status": "STATUS_UP",
  "version": "1.0.0",
  "checks": [
    {
      "name": "redis",
      "status": "STATUS_UP",
      "message": "connection successful",
      "timestamp": "2024-01-15T10:30:00Z"
    },
    {
      "name": "postgres",
      "status": "STATUS_UP",
      "message": "database healthy",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Degraded service (non-critical failure):**

```json
{
  "status": "STATUS_DEGRADED",
  "version": "1.0.0",
  "checks": [
    {
      "name": "redis",
      "status": "STATUS_UP",
      "message": "connection successful",
      "timestamp": "2024-01-15T10:30:00Z"
    },
    {
      "name": "metrics",
      "status": "STATUS_DOWN",
      "message": "connection failed",
      "error": "dial tcp: connection refused",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Unhealthy service (critical failure):**

```json
{
  "status": "STATUS_DOWN",
  "version": "1.0.0",
  "checks": [
    {
      "name": "redis",
      "status": "STATUS_DOWN",
      "message": "connection failed",
      "error": "dial tcp: connection refused",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### GET /ready

**Service ready:**

```json
{
  "ready": true,
  "reason": "",
  "checks": [
    {
      "name": "redis",
      "status": "STATUS_UP",
      "message": "connection successful",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Service not ready (SetReady not called):**

```json
{
  "ready": false,
  "reason": "service is not ready",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Service not ready (critical dependency down):**

```json
{
  "ready": false,
  "reason": "connection failed",
  "checks": [
    {
      "name": "redis",
      "status": "STATUS_DOWN",
      "message": "connection failed",
      "error": "dial tcp: connection refused",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## CLI Client

`srvmon-cli` is a terminal tool for checking service health with colored, in-place status output.

```
  srvmon — service health monitor
  ────────────────────────────────────────────────

  HEALTH  Health:  UP   v1.0.0

  ├── redis            ● UP    connection successful
  ├── postgres         ● UP    database healthy
  └── external-api     ● DOWN  connection failed
        dial tcp: connection refused

  READY  Readiness:  READY
```

**Usage:**

```bash
# Check default address (localhost:8080)
srvmon-cli

# Custom address
srvmon-cli -a localhost:9090

# Live monitoring (updates in-place, no scroll spam)
srvmon-cli --watch
srvmon-cli -w -i 1s -a localhost:8085

# Health or readiness only
srvmon-cli health -a localhost:8085
srvmon-cli ready -a localhost:8085
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--addr` | `-a` | `localhost:8080` | srvmon HTTP address |
| `--timeout` | `-t` | `3s` | Request timeout |
| `--watch` | `-w` | `false` | Continuously poll and update in-place |
| `--interval` | `-i` | `2s` | Poll interval (with `--watch`) |

## Kubernetes Integration

### Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-service
  template:
    metadata:
      labels:
        app: my-service
    spec:
      containers:
      - name: my-service
        image: my-service:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 50051
          name: grpc
        livenessProbe:
          httpGet:
            path: /healthz
            port: http
          initialDelaySeconds: 10
          periodSeconds: 15
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        startupProbe:
          httpGet:
            path: /healthz
            port: http
          initialDelaySeconds: 0
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 30
```

### gRPC Native Health Check (Kubernetes 1.24+)

```yaml
livenessProbe:
  grpc:
    port: 50051
  initialDelaySeconds: 10
  periodSeconds: 15
readinessProbe:
  grpc:
    port: 50051
  initialDelaySeconds: 5
  periodSeconds: 5
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  selector:
    app: my-service
  ports:
  - name: http
    port: 8080
    targetPort: http
  - name: grpc
    port: 50051
    targetPort: grpc
```

### Testing the Service

```bash
# Start the example
make run

# In another terminal:

# Check health (HTTP)
curl -s http://localhost:8080/health | jq

# Check readiness (HTTP)
curl -s http://localhost:8080/ready | jq

# Check health (gRPC with grpcurl)
grpcurl -plaintext localhost:50051 srvmon.v1.srvmon/Health

# Check readiness (gRPC)
grpcurl -plaintext localhost:50051 srvmon.v1.srvmon/Ready
```

## Project Structure

```
srvmon/
├── srvmon.go              # Main SrvMon struct and server lifecycle
├── checks.go              # Health() and Ready() RPC implementations
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums
├── Makefile               # Build automation
├── LICENSE                # MIT license
├── README.md              # This file
├── CLAUDE.md              # AI assistant context
├── api/
│   ├── proto/v1/
│   │   └── srvmon.proto   # gRPC service definition
│   └── swagger/v1/
│       └── srvmon.yaml    # OpenAPI 3.0 specification
├── pkg/
│   └── grpc/srvmon/v1/    # Generated gRPC code (do not edit)
├── cmd/
│   └── srvmon-cli/        # CLI client for health checking
├── example/
│   └── main.go            # Example implementation
└── test/
    └── ...                # Test files
```

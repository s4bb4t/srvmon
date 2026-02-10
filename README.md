# srvmon

Production-grade service health monitoring package for Go with gRPC and REST/OpenAPI support.

[![Go Reference](https://pkg.go.dev/badge/github.com/s4bb4t/srvmon.svg)](https://pkg.go.dev/github.com/s4bb4t/srvmon)
[![Go Report Card](https://goreportcard.com/badge/github.com/s4bb4t/srvmon)](https://goreportcard.com/report/github.com/s4bb4t/srvmon)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- **Pluggable Health Checks**: Register custom checkers implementing the `Checker` interface
- **Built-in Checkers**: TCP ping, HTTP, SQL database, Redis, memory usage
- **Dual Protocol Support**: Both gRPC and REST/HTTP endpoints
- **Kubernetes Ready**: `/healthz`, `/readyz`, `/livez` endpoints out of the box
- **OpenAPI 3.0**: Full OpenAPI specification with Swagger UI support
- **Concurrent Execution**: Parallel health check execution with configurable limits
- **Result Caching**: Configurable TTL-based caching to reduce load
- **Critical Dependencies**: Mark checkers as critical to fail fast on dependency issues
- **Status Aggregation**: Intelligent status aggregation with multiple strategies
- **Graceful Shutdown**: Clean shutdown with configurable timeout
- **Observable**: Structured logging with zap, ready for Prometheus metrics
- **Functional Options**: Clean, extensible configuration pattern

## Installation

```bash
go get github.com/s4bb4t/srvmon
```

## Quick Start

```go
package main

import (
    "context"
    "time"

    "github.com/s4bb4t/srvmon"
    "github.com/s4bb4t/srvmon/server"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()

    // Create monitor
    monitor, _ := srvmon.New(
        srvmon.WithServiceName("my-service"),
        srvmon.WithVersion("1.0.0"),
        srvmon.WithLogger(logger),
        srvmon.WithHTTPAddress(":8080"),
        srvmon.WithGRPCAddress(":50051"),
    )

    // Register health checkers
    monitor.Register(srvmon.NewPingChecker("redis", "localhost:6379", 5*time.Second))
    monitor.RegisterCritical(srvmon.NewHTTPChecker("api", "http://api:8080/health", 10*time.Second))

    // Setup servers
    monitor.SetHTTPServer(server.NewHTTPServer(monitor, ":8080"))
    monitor.SetGRPCServer(server.NewGRPCServer(monitor, ":50051"))

    // Start
    monitor.Start(context.Background())
}
```

## API Endpoints

### REST/HTTP

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Full health report with all dependency checks |
| `GET /ready` | Readiness probe for load balancer |
| `GET /live` | Liveness probe for container orchestration |
| `GET /healthz` | Kubernetes-style health endpoint |
| `GET /readyz` | Kubernetes-style readiness endpoint |
| `GET /livez` | Kubernetes-style liveness endpoint |
| `GET /openapi.json` | OpenAPI 3.0 specification |

### gRPC

```protobuf
service HealthService {
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Readiness(ReadinessRequest) returns (ReadinessResponse);
  rpc Liveness(LivenessRequest) returns (LivenessResponse);
  rpc Check(CheckRequest) returns (CheckResponse);
  rpc Watch(WatchRequest) returns (stream HealthResponse);
}
```

## Health Status Values

| Status | Description | HTTP Code |
|--------|-------------|-----------|
| `UP` | Component is healthy | 200 |
| `DOWN` | Component is unhealthy | 503 |
| `DEGRADED` | Component works with reduced capacity | 200 |
| `UNKNOWN` | Status cannot be determined | 200 |

## Built-in Checkers

### PingChecker

Checks TCP connectivity to a host:port.

```go
checker := srvmon.NewPingChecker("redis", "localhost:6379", 5*time.Second)
monitor.Register(checker)
```

### HTTPChecker

Checks HTTP endpoint health:

```go
checker := srvmon.NewHTTPChecker(
    "api-gateway",
    "http://gateway:8080/health",
    10*time.Second,
    srvmon.WithHTTPExpectCode(200),
    srvmon.WithHTTPMethod("HEAD"),
)
monitor.Register(checker)
```

### SQLChecker

Checks database connectivity:

```go
db, _ := sql.Open("postgres", dsn)
checker := srvmon.NewSQLChecker("postgres", db, "SELECT 1", 5*time.Second)
monitor.RegisterCritical(checker)
```

### RedisChecker

Checks Redis connectivity (compatible with go-redis):

```go
type redisClient struct {
    *redis.Client
}

func (c *redisClient) Ping(ctx context.Context) error {
    return c.Client.Ping(ctx).Err()
}

checker := srvmon.NewRedisChecker("redis", &redisClient{rdb}, 5*time.Second)
monitor.Register(checker)
```

### CompositeChecker

Aggregates multiple checkers with a strategy:

```go
// Require majority of Redis nodes to be healthy
checker := srvmon.NewCompositeChecker(
    "redis-cluster",
    srvmon.StrategyMajority,
    srvmon.NewPingChecker("redis-1", "redis-1:6379", 5*time.Second),
    srvmon.NewPingChecker("redis-2", "redis-2:6379", 5*time.Second),
    srvmon.NewPingChecker("redis-3", "redis-3:6379", 5*time.Second),
)
monitor.Register(checker)
```

### Custom Checker

Implement the `Checker` interface:

```go
type Checker interface {
    Name() string
    Check(ctx context.Context) CheckResult
}
```

Or use `CheckerFunc` for simple checks:

```go
checker := srvmon.NewCheckerFunc("custom", func(ctx context.Context) srvmon.CheckResult {
    start := time.Now()
    // ... your check logic ...
    return srvmon.CheckResult{
        Name:      "custom",
        Status:    srvmon.StatusUp,
        Message:   "check passed",
        Duration:  time.Since(start),
        Timestamp: time.Now(),
    }
})
monitor.Register(checker)
```

## Configuration Options

```go
monitor, _ := srvmon.New(
    // Service identification
    srvmon.WithServiceName("my-service"),
    srvmon.WithVersion("1.0.0"),
    srvmon.WithHostname("node-1"),

    // Server addresses
    srvmon.WithGRPCAddress(":50051"),
    srvmon.WithHTTPAddress(":8080"),

    // Check configuration
    srvmon.WithCheckTimeout(10*time.Second),
    srvmon.WithCheckInterval(30*time.Second),
    srvmon.WithCacheTTL(5*time.Second),

    // Parallel execution
    srvmon.WithParallelChecks(true),
    srvmon.WithMaxParallelChecks(10),

    // Shutdown
    srvmon.WithShutdownTimeout(30*time.Second),

    // Observability
    srvmon.WithLogger(logger),
    srvmon.WithMetrics("/metrics"),
    srvmon.WithPprof(),

    // Callbacks
    srvmon.WithOnHealthChange(func(old, new srvmon.Status) {
        log.Printf("health changed: %s -> %s", old, new)
    }),
)
```

## Checker Categories

Checkers can be registered with different categories:

```go
// Regular health check (aggregated into overall health)
monitor.Register(checker)

// Critical check (failure immediately marks service as DOWN)
monitor.RegisterCritical(checker)

// Readiness check (determines if service accepts traffic)
monitor.RegisterReadiness(checker)

// Liveness check (determines if service should be restarted)
monitor.RegisterLiveness(checker)

// Custom category
monitor.RegisterWithCategory(checker, srvmon.CategoryHealth, true /* critical */)
```

## Kubernetes Integration

### Deployment Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
spec:
  template:
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
            path: /livez
            port: http
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
        startupProbe:
          httpGet:
            path: /healthz
            port: http
          failureThreshold: 30
          periodSeconds: 10
```

### gRPC Health Checking

The server implements the [gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md):

```yaml
livenessProbe:
  grpc:
    port: 50051
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Response Examples

### Health Response

```json
{
  "status": "UP",
  "version": "1.0.0",
  "hostname": "web-server-1",
  "uptime": "24h30m15s",
  "checks": [
    {
      "name": "postgres",
      "status": "UP",
      "message": "database is healthy",
      "details": {
        "open_connections": 5,
        "in_use": 2,
        "idle": 3
      },
      "duration": "5.2ms",
      "timestamp": "2024-01-15T10:30:00Z"
    },
    {
      "name": "redis",
      "status": "UP",
      "message": "redis is healthy",
      "duration": "1.1ms",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Readiness Response

```json
{
  "ready": true,
  "checks": [],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Liveness Response

```json
{
  "alive": true,
  "uptime": "24h30m15s",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Development

### Prerequisites

- Go 1.21+
- protoc (Protocol Buffers compiler)
- protoc-gen-go & protoc-gen-go-grpc

### Setup

```bash
# Install dependencies
make deps

# Generate protobuf code
make proto

# Build
make build

# Run tests
make test

# Run linter
make lint
```

### Project Structure

```
srvmon/
├── srvmon.go           # Main Monitor implementation
├── checker.go          # Checker interface and built-in checkers
├── status.go           # Health status types and reports
├── options.go          # Functional options for configuration
├── aggregator.go       # Health check aggregation logic
├── server/
│   ├── grpc.go         # gRPC server implementation
│   └── http.go         # HTTP/REST server implementation
├── api/
│   ├── proto/v1/       # Protobuf definitions
│   └── swagger/        # OpenAPI specification
├── pkg/
│   └── grpc/           # Generated gRPC code
├── example/            # Example implementation
└── test/               # Test fixtures
```

## Best Practices

1. **Use Critical Checkers Wisely**: Only mark truly critical dependencies as critical
2. **Set Appropriate Timeouts**: Health checks should be fast; use short timeouts
3. **Cache Results**: Use caching to reduce load on dependencies during health checks
4. **Separate Concerns**: Use different checker categories for different probe types
5. **Graceful Degradation**: Return DEGRADED instead of DOWN when possible
6. **Log State Changes**: Use the `OnHealthChange` callback to log status transitions

## License

MIT License - see [LICENSE](LICENSE) for details.

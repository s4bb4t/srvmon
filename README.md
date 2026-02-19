# srvmon

Service health monitoring for Go microservices. gRPC + REST, Kubernetes-ready.

[![Go Reference](https://pkg.go.dev/badge/github.com/s4bb4t/srvmon.svg)](https://pkg.go.dev/github.com/s4bb4t/srvmon)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Install

```bash
# Library
go get github.com/s4bb4t/srvmon

# CLI
go install github.com/s4bb4t/srvmon/cmd/srvmon-cli@latest
```

## Quick Start

```go
cfg := srvmon.Config{
    Version:     "1.0.0",
    GRPCAddress: ":50051",
    HTTPAddress: ":8080",
}

monitor := srvmon.New(cfg, logger)
monitor.AddDependencies(
    NewPingChecker("redis", "localhost:6379", 5*time.Second, true),       // critical
    srvmon.NewConnChecker(grpcConn, "auth-svc", true),                    // gRPC health check
    NewPingChecker("metrics", "localhost:9090", 2*time.Second, false),    // non-critical
)
monitor.SetReady()
monitor.Run(ctx) // blocks until ctx is canceled
```

## How It Works

Implement the `Checker` interface and register dependencies:

```go
type Checker interface {
    MustOK(ctx context.Context) bool                       // true = critical dependency
    Check(ctx context.Context) (*pb.CheckResult, error)    // perform the check
}
```

**Status aggregation:**

| Dependency fails | `MustOK() = true` | `MustOK() = false` |
|---|---|---|
| Result | **DOWN** | **DEGRADED** |

If all checks pass, service status is **UP**.

## Built-in: ConnChecker

Verifies gRPC dependencies via the standard `grpc.health.v1.Health/Check` protocol — not just connection state, but actual service readiness.

```go
conn, _ := grpc.NewClient("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))

// Basic
srvmon.NewConnChecker(conn, "auth", true)

// With options
srvmon.NewConnChecker(conn, "auth", true,
    srvmon.WithTimeout(2*time.Second),
    srvmon.WithService("my.service.Name"),  // check specific service
)
```

The target server must register `grpc.health.v1.Health` (srvmon does this automatically for its own gRPC server).

## Endpoints

| HTTP | gRPC | Description |
|---|---|---|
| `GET /health` | `srvmon.v1.srvmon/Health` | Full health report |
| `GET /healthz` | — | Alias for `/health` |
| `GET /ready` | `srvmon.v1.srvmon/Ready` | Readiness probe |
| `GET /readyz` | — | Alias for `/ready` |

srvmon also registers `grpc.health.v1.Health` on its gRPC server, so `ConnChecker` from other services works out of the box.

## CLI

```
  srvmon — service health monitor
  ────────────────────────────────────────────────

  HEALTH  Health:  UP   v1.0.0

  ├── redis         ● UP      connection successful
  ├── auth-svc      ● UP      SERVING
  └── metrics       ● DOWN    connection failed
        dial tcp: connection refused

  READY  Readiness:  NOT READY   connection failed
```

```bash
srvmon-cli                              # localhost:8080
srvmon-cli -a localhost:9090            # custom address
srvmon-cli --watch                      # live updates (in-place, no spam)
srvmon-cli -w -i 1s -a localhost:8085   # watch with 1s interval
srvmon-cli health                       # health only
srvmon-cli ready                        # readiness only
```

| Flag | Short | Default | Description |
|---|---|---|---|
| `--addr` | `-a` | `localhost:8080` | HTTP address |
| `--timeout` | `-t` | `3s` | Request timeout |
| `--watch` | `-w` | `false` | Poll and update in-place |
| `--interval` | `-i` | `2s` | Poll interval |

## Kubernetes

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
```

Or native gRPC probes (k8s 1.24+):

```yaml
livenessProbe:
  grpc:
    port: 50051
```

## License

MIT

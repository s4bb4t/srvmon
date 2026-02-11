package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/s4bb4t/srvmon"
	pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PingChecker checks TCP connectivity to a host:port
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

// AlwaysUpChecker always returns healthy status
type AlwaysUpChecker struct {
	name string
}

func NewAlwaysUpChecker(name string) *AlwaysUpChecker {
	return &AlwaysUpChecker{name: name}
}

func (c *AlwaysUpChecker) MustOK(_ context.Context) bool {
	return false
}

func (c *AlwaysUpChecker) Check(_ context.Context) (*pb.CheckResult, error) {
	return &pb.CheckResult{
		Name:      c.name,
		Status:    pb.Status_STATUS_UP,
		Message:   "always healthy",
		Timestamp: timestamppb.Now(),
	}, nil
}

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create configuration
	cfg := srvmon.Config{
		Version:     "1.0.0",
		GRPCAddress: ":50051",
		HTTPAddress: ":8080",
	}

	// Create monitor with dependencies
	monitor := srvmon.New(cfg, logger)

	// Add health checkers
	monitor.AddDependencies(
		// Critical: if Redis is down, service is DOWN
		NewPingChecker("redis", "localhost:6379", 5*time.Second, true),
		// Non-critical: if external API is down, service is DEGRADED
		NewPingChecker("external-api", "api.example.com:443", 10*time.Second, false),
		// Always healthy checker for demonstration
		NewAlwaysUpChecker("internal-state"),
	)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutdown signal received")
		cancel()
	}()

	// Mark service as ready after initialization
	// In a real app, call this after all initialization is complete
	go func() {
		time.Sleep(2 * time.Second)
		logger.Info("service is ready")
		monitor.SetReady()
	}()

	logger.Info("starting srvmon example",
		zap.String("http", cfg.HTTPAddress),
		zap.String("grpc", cfg.GRPCAddress),
	)

	// Run blocks until context is canceled
	monitor.Run(ctx)

	logger.Info("srvmon example stopped")
}

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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PingChecker checks TCP connectivity to a host:port.
type PingChecker struct {
	name     string
	addr     string
	timeout  time.Duration
	critical bool
}

func NewPingChecker(name, addr string, timeout time.Duration, critical bool) *PingChecker {
	return &PingChecker{name: name, addr: addr, timeout: timeout, critical: critical}
}

func (c *PingChecker) MustOK(_ context.Context) bool { return c.critical }

func (c *PingChecker) Check(_ context.Context) (*pb.CheckResult, error) {
	result := &pb.CheckResult{Name: c.name, Timestamp: timestamppb.Now()}

	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		result.Status = pb.Status_STATUS_DOWN
		result.Message = "connection failed"
		result.Error = err.Error()
		return result, nil
	}
	_ = conn.Close()

	result.Status = pb.Status_STATUS_UP
	result.Message = "connection successful"
	return result, nil
}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	cfg := srvmon.Config{
		Version:     "1.0.0",
		GRPCAddress: ":50051",
		HTTPAddress: ":8080",
	}

	// Example: gRPC connection to another service that exposes grpc.health.v1
	otherSvcConn, err := grpc.NewClient("localhost:50052",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Fatal("dial other service", zap.Error(err))
	}
	defer func() {
		_ = otherSvcConn.Close()
	}()

	monitor := srvmon.New(cfg, logger)
	monitor.AddDependencies(
		// Critical: TCP check for Redis
		NewPingChecker("redis", "localhost:6379", 5*time.Second, true),
		// Critical: gRPC health check for another microservice
		srvmon.NewConnChecker(otherSvcConn, "other-service", true,
			srvmon.WithTimeout(2*time.Second),
		),
		// Non-critical: external API
		NewPingChecker("external-api", "api.example.com:443", 10*time.Second, false),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutdown signal received")
		cancel()
	}()

	go func() {
		time.Sleep(2 * time.Second)
		logger.Info("service is ready")
		monitor.SetReady()
	}()

	logger.Info("starting srvmon example",
		zap.String("http", cfg.HTTPAddress),
		zap.String("grpc", cfg.GRPCAddress),
	)

	monitor.Run(ctx)
	logger.Info("srvmon example stopped")
}

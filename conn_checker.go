package srvmon

import (
	"context"
	"time"

	pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ConnChecker struct {
	conn    *grpc.ClientConn
	name    string
	must    bool
	timeout time.Duration
	service string
}

type ConnCheckerOption func(*ConnChecker)

// WithTimeout sets the deadline for the health check RPC.
// Default: 3s.
func WithTimeout(d time.Duration) ConnCheckerOption {
	return func(c *ConnChecker) { c.timeout = d }
}

// WithService sets the service name passed to the gRPC Health check.
// Empty string checks the overall server health.
func WithService(name string) ConnCheckerOption {
	return func(c *ConnChecker) { c.service = name }
}

func NewConnChecker(conn *grpc.ClientConn, name string, mustOK bool, opts ...ConnCheckerOption) *ConnChecker {
	c := &ConnChecker{conn: conn, name: name, must: mustOK, timeout: 3 * time.Second}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *ConnChecker) Check(ctx context.Context) (*pb.CheckResult, error) {
	resp := &pb.CheckResult{
		Name:      c.name,
		Timestamp: timestamppb.New(time.Now()),
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Force the connection out of IDLE so it actually tries to connect.
	c.conn.Connect()

	client := grpc_health_v1.NewHealthClient(c.conn)
	hr, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: c.service,
	})
	if err != nil {
		resp.Status = pb.Status_STATUS_DOWN
		resp.Message = "health check failed"
		resp.Error = err.Error()
		return resp, nil
	}

	switch hr.GetStatus() {
	case grpc_health_v1.HealthCheckResponse_SERVING:
		resp.Status = pb.Status_STATUS_UP
		resp.Message = "SERVING"
	case grpc_health_v1.HealthCheckResponse_NOT_SERVING:
		resp.Status = pb.Status_STATUS_DOWN
		resp.Message = "NOT_SERVING"
	case grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN:
		resp.Status = pb.Status_STATUS_DEGRADED
		resp.Message = "SERVICE_UNKNOWN"
	default:
		resp.Status = pb.Status_STATUS_UNKNOWN
		resp.Message = hr.GetStatus().String()
	}

	return resp, nil
}

func (c *ConnChecker) MustOK(_ context.Context) bool {
	return c.must
}

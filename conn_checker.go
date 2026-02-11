package srvmon

import (
	"context"
	"time"

	pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ConnChecker struct {
	conn *grpc.ClientConn
	name string
	must bool
}

func NewConnChecker(conn *grpc.ClientConn, name string, mustOK bool) *ConnChecker {
	return &ConnChecker{conn: conn, name: name, must: mustOK}
}

func (c *ConnChecker) Check(_ context.Context) (*pb.CheckResult, error) {
	state := c.conn.GetState()
	resp := &pb.CheckResult{
		Name:      c.name,
		Timestamp: timestamppb.New(time.Now()),
	}

	if state == connectivity.Ready || state == connectivity.Idle {
		resp.Status = pb.Status_STATUS_UP
		resp.Message = state.String()
	} else {
		resp.Status = pb.Status_STATUS_DOWN
		resp.Message = state.String()
	}

	return resp, nil
}

func (c *ConnChecker) MustOK(_ context.Context) bool {
	return c.must
}

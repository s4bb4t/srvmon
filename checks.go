package srvmon

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (m *SrvMon) Health(ctx context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	resp := &pb.HealthResponse{
		Status:  pb.Status_STATUS_UP,
		Version: m.version,
	}

	var once sync.Once
	for _, dep := range m.dependencies {
		check, err := dep.Check(ctx)
		if err != nil {
			m.log.Error("dependency check", zap.Error(err))
			return nil, fmt.Errorf("dependency check: %w", err)
		}

		resp.Checks = append(resp.Checks, check)

		if check.Status != pb.Status_STATUS_DOWN {
			continue
		}

		if dep.MustOK(ctx) {
			once.Do(func() {
				resp.Status = pb.Status_STATUS_DOWN
			})
		} else {
			once.Do(func() {
				resp.Status = pb.Status_STATUS_DEGRADED
			})
		}
	}

	resp.Timestamp = timestamppb.New(time.Now())

	return resp, nil
}

func (m *SrvMon) Ready(ctx context.Context, _ *pb.ReadinessRequest) (resp *pb.ReadinessResponse, _ error) {
	resp = &pb.ReadinessResponse{
		Ready: false,
	}

	defer func() {
		resp.Timestamp = timestamppb.New(time.Now())
	}()

	if !m.ready.Load() {
		resp.Reason = "service is not ready"
		return resp, nil
	}

	resp.Ready = true

	var once sync.Once
	for _, dep := range m.dependencies {
		check, err := dep.Check(ctx)
		if err != nil {
			m.log.Error("dependency check", zap.Error(err))
			return nil, fmt.Errorf("dependency check: %w", err)
		}

		resp.Checks = append(resp.Checks, check)
		if check.Status != pb.Status_STATUS_DOWN {
			continue
		}

		if dep.MustOK(ctx) {
			once.Do(func() {
				resp.Ready = false
				resp.Reason = check.Message
			})
		}
	}

	return resp, nil
}

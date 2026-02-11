package srvmon

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	pb "github.com/s4bb4t/srvmon/pkg/grpc/srvmon/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/encoding/protojson"
)

const maxConcurrent = 10

var kaProps = keepalive.ServerParameters{
	MaxConnectionIdle:     time.Minute,
	MaxConnectionAge:      time.Minute,
	MaxConnectionAgeGrace: 5 * time.Second,
	Time:                  5 * time.Second,
	Timeout:               time.Second,
}

var kaPolicy = keepalive.EnforcementPolicy{
	MinTime:             5 * time.Second,
	PermitWithoutStream: true,
}

type (
	Checker interface {
		MustOK(ctx context.Context) bool
		Check(ctx context.Context) (*pb.CheckResult, error)
	}

	SrvMon struct {
		ctx          context.Context
		dependencies []Checker
		version      string
		grpcAddr     string
		httpAddr     string

		readyOnce sync.Once
		//ready     chan struct{}
		ready atomic.Bool

		log *zap.Logger
		pb.UnimplementedSrvmonServer
	}

	Config struct {
		Version     string `json:"version" yaml:"version" mapstructure:"version"`
		GRPCAddress string `json:"grpc_address" yaml:"grpc_address" mapstructure:"grpc_address"`
		HTTPAddress string `json:"http_address" yaml:"http_address" mapstructure:"http_address"`
	}
)

func New(cfg Config, log *zap.Logger, dependencies ...Checker) *SrvMon {
	m := &SrvMon{
		version:  cfg.Version,
		grpcAddr: cfg.GRPCAddress,
		httpAddr: cfg.HTTPAddress,
		log:      log,
	}

	if dependencies != nil {
		m.dependencies = dependencies
	}

	return m
}

func (m *SrvMon) AddDependencies(dependency ...Checker) *SrvMon {
	m.dependencies = append(m.dependencies, dependency...)
	return m
}

func (m *SrvMon) SetReady() {
	m.ready.CompareAndSwap(false, true)
}

func (m *SrvMon) Run(ctx context.Context) {
	shutdownGRPC := m.startGRPC()
	shutdownREST := m.startREST()

	<-ctx.Done()
	shutdownGRPC()
	if err := shutdownREST(context.Background()); err != nil {
		m.log.Error("shutdown rest server", zap.Error(err))
	}
}

func (m *SrvMon) startREST() func(ctx context.Context) error {
	router := mux.NewRouter()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		resp, err := m.Health(r.Context(), &pb.HealthRequest{})
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(err.Error()))
		}

		json, err := protojson.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}

		w.WriteHeader(http.StatusOK)
		w.Write(json)
	}

	readyHandler := func(w http.ResponseWriter, r *http.Request) {
		resp, err := m.Ready(r.Context(), &pb.ReadinessRequest{})
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(err.Error()))
		}

		json, err := protojson.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}

		w.WriteHeader(http.StatusOK)
		w.Write(json)
	}

	router.HandleFunc("/health", healthHandler)
	router.HandleFunc("/healthz", healthHandler)
	router.HandleFunc("/ready", readyHandler)
	router.HandleFunc("/readyz", readyHandler)

	srv := &http.Server{
		Addr:              m.httpAddr,
		Handler:           router,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       10 * time.Second,
	}

	m.log.Info("starting srvmon rest", zap.String("address", m.httpAddr))

	go func() {

	}()
	if err := srv.ListenAndServe(); err != nil {
		m.log.Error("serve", zap.Error(err))
	}

	return srv.Shutdown
}

func (m *SrvMon) startGRPC() func() {
	opts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.KeepaliveParams(kaProps),
		grpc.KeepaliveEnforcementPolicy(kaPolicy),
		grpc.MaxConcurrentStreams(uint32(maxConcurrent)),
		grpc.MaxRecvMsgSize(4 * 1024 * 1024),
		grpc.MaxSendMsgSize(4 * 1024 * 1024),
	}

	s := grpc.NewServer(opts...)

	pb.RegisterSrvmonServer(s, m)

	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		m.log.Panic("listen:", zap.Error(err))
	}

	m.log.Info("starting srvmon grpc", zap.String("address", m.grpcAddr))

	go func() {
		if err := s.Serve(lis); err != nil {
			m.log.Error("serve:", zap.Error(err))
		}
	}()

	return s.Stop
}

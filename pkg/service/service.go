// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package service implements the high-level service entry point.
package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/jonboulle/clockwork"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/siderolabs/discovery-api/api/v1alpha1/server/pb"
	"github.com/siderolabs/go-debug"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/experimental"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/discovery-service/internal/landing"
	"github.com/siderolabs/discovery-service/internal/limiter"
	"github.com/siderolabs/discovery-service/internal/state"
	"github.com/siderolabs/discovery-service/internal/state/storage"
	"github.com/siderolabs/discovery-service/pkg/limits"
	"github.com/siderolabs/discovery-service/pkg/server"
)

// Options are the configuration options for the service.
type Options struct {
	MetricsRegisterer prom.Registerer

	LandingAddr      string
	MetricsAddr      string
	SnapshotPath     string
	DebugAddr        string
	RedirectEndpoint string
	ListenAddr       string

	GCInterval       time.Duration
	SnapshotInterval time.Duration

	LandingServerEnabled bool
	DebugServerEnabled   bool
	MetricsServerEnabled bool
	SnapshotsEnabled     bool
}

// Run starts the service with the given options.
func Run(ctx context.Context, options Options, logger *zap.Logger) error {
	logger.Info("service starting")

	defer logger.Info("service shut down")

	recoveryOpt := grpc_recovery.WithRecoveryHandler(recoveryHandler(logger))

	limiter := limiter.NewIPRateLimiter(limits.IPRateRequestsPerSecondMax, limits.IPRateBurstSizeMax)

	metrics := grpc_prometheus.NewServerMetrics(
		grpc_prometheus.WithServerHandlingTimeHistogram(grpc_prometheus.WithHistogramBuckets([]float64{0.01, 0.1, 0.25, 0.5, 1.0, 2.5})),
	)

	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
		logging.WithFieldsFromContext(logging.ExtractFields),
	}

	//nolint:contextcheck
	serverOptions := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			server.AddLoggingFieldsUnaryServerInterceptor(),
			logging.UnaryServerInterceptor(interceptorLogger(logger), loggingOpts...),
			server.RateLimitUnaryServerInterceptor(limiter),
			metrics.UnaryServerInterceptor(),
			grpc_recovery.UnaryServerInterceptor(recoveryOpt),
		),
		grpc.ChainStreamInterceptor(
			server.AddLoggingFieldsStreamServerInterceptor(),
			server.RateLimitStreamServerInterceptor(limiter),
			logging.StreamServerInterceptor(interceptorLogger(logger), loggingOpts...),
			metrics.StreamServerInterceptor(),
			grpc_recovery.StreamServerInterceptor(recoveryOpt),
		),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime: 10 * time.Second,
		}),
		grpc.SharedWriteBuffer(true),
		experimental.RecvBufferPool(grpc.NewSharedBufferPool()),
		grpc.ReadBufferSize(16 * 1024),
		grpc.WriteBufferSize(16 * 1024),
	}

	state := state.NewState(logger)

	var stateStorage *storage.Storage

	if options.SnapshotsEnabled {
		stateStorage = storage.New(options.SnapshotPath, state, logger)
		if err := stateStorage.Load(); err != nil {
			logger.Warn("failed to load state from storage", zap.Error(err))
		}
	} else {
		logger.Info("snapshots are disabled")
	}

	srv := server.NewClusterServer(state, ctx.Done(), options.RedirectEndpoint)

	lis, err := net.Listen("tcp", options.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	landingLis, err := net.Listen("tcp", options.LandingAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s := grpc.NewServer(serverOptions...)
	pb.RegisterClusterServer(s, srv)

	metrics.InitializeMetrics(s)

	var (
		metricsServer http.Server
		landingServer http.Server
	)

	if options.MetricsServerEnabled {
		var metricsMux http.ServeMux

		metricsMux.Handle("/metrics", promhttp.Handler())

		metricsServer = http.Server{
			Addr:    options.MetricsAddr,
			Handler: &metricsMux,
		}
	}

	if options.LandingServerEnabled {
		landingServer = http.Server{
			Handler: landing.Handler(state, logger),
		}
	}

	eg, ctx := errgroup.WithContext(ctx)

	if options.SnapshotsEnabled {
		eg.Go(func() error {
			return stateStorage.Start(ctx, clockwork.NewRealClock(), options.SnapshotInterval)
		})
	}

	eg.Go(func() error {
		logger.Info("gRPC server starting", zap.Stringer("address", lis.Addr()))

		if serveErr := s.Serve(lis); serveErr != nil {
			return fmt.Errorf("failed to serve: %w", serveErr)
		}

		return nil
	})

	if options.LandingServerEnabled {
		eg.Go(func() error {
			logger.Info("landing server starting", zap.Stringer("address", landingLis.Addr()))

			if serveErr := landingServer.Serve(landingLis); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				return fmt.Errorf("failed to serve: %w", serveErr)
			}

			return nil
		})
	}

	if options.MetricsServerEnabled {
		eg.Go(func() error {
			logger.Info("metrics starting", zap.String("address", metricsServer.Addr))

			if serveErr := metricsServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				return serveErr
			}

			return nil
		})
	}

	eg.Go(func() error {
		<-ctx.Done()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		s.GracefulStop()

		if options.LandingServerEnabled {
			landingServer.Shutdown(ctx) //nolint:errcheck
		}

		if options.MetricsServerEnabled {
			metricsServer.Shutdown(shutdownCtx) //nolint:errcheck,contextcheck
		}

		return nil
	})

	eg.Go(func() error {
		state.RunGC(ctx, logger, options.GCInterval)

		return nil
	})

	eg.Go(func() error {
		limiter.RunGC(ctx)

		return nil
	})

	if options.DebugServerEnabled {
		eg.Go(func() error {
			return debug.ListenAndServe(ctx, options.DebugAddr, func(msg string) { logger.Info(msg) })
		})
	}

	if options.MetricsRegisterer != nil {
		collectors := []prom.Collector{state, srv, metrics, stateStorage}

		defer unregisterCollectors(options.MetricsRegisterer, collectors...)

		if err = registerCollectors(options.MetricsRegisterer, collectors...); err != nil {
			return fmt.Errorf("failed to register collectors: %w", err)
		}
	}

	return eg.Wait()
}

func recoveryHandler(logger *zap.Logger) grpc_recovery.RecoveryHandlerFunc {
	return func(p interface{}) error {
		if logger != nil {
			logger.Error("grpc panic", zap.Any("panic", p), zap.Stack("stack"))
		}

		return status.Errorf(codes.Internal, "%v", p)
	}
}

func interceptorLogger(l *zap.Logger) logging.Logger {
	return logging.LoggerFunc(func(_ context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i].(string) //nolint:forcetypeassert,errcheck
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key, v))
			case int:
				f = append(f, zap.Int(key, v))
			case bool:
				f = append(f, zap.Bool(key, v))
			default:
				f = append(f, zap.Any(key, v))
			}
		}

		logger := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			logger.Debug(msg)
		case logging.LevelInfo:
			logger.Info(msg)
		case logging.LevelWarn:
			logger.Warn(msg)
		case logging.LevelError:
			logger.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func unregisterCollectors(registerer prom.Registerer, collectors ...prom.Collector) {
	for _, collector := range collectors {
		if collector == nil {
			continue
		}

		registerer.Unregister(collector)
	}
}

func registerCollectors(registerer prom.Registerer, collectors ...prom.Collector) (err error) {
	for _, collector := range collectors {
		if collector == nil {
			continue
		}

		if err = registerer.Register(collector); err != nil {
			return fmt.Errorf("failed to register collector: %w", err)
		}
	}

	return nil
}

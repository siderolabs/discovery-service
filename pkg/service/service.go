// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package service implements the high-level service entry point.
package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/jonboulle/clockwork"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/siderolabs/discovery-api/api/v1alpha1/server/pb"
	"github.com/siderolabs/go-debug"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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

	ListenAddr   string
	LandingAddr  string
	MetricsAddr  string
	SnapshotPath string
	DebugAddr    string

	CertificatePath, KeyPath string

	RedirectEndpoint string

	GCInterval       time.Duration
	SnapshotInterval time.Duration

	LandingServerEnabled bool
	DebugServerEnabled   bool
	MetricsServerEnabled bool
	SnapshotsEnabled     bool
	TrustXRealIP         bool
}

func newGRPCServer(ctx context.Context, state *state.State, options Options, logger *zap.Logger) (*grpc.Server, *server.ClusterServer, *limiter.IPRateLimiter, *grpc_prometheus.ServerMetrics) {
	recoveryOpt := grpc_recovery.WithRecoveryHandler(recoveryHandler(logger))

	limiter := limiter.NewIPRateLimiter(limits.IPRateRequestsPerSecondMax, limits.IPRateBurstSizeMax)

	metrics := grpc_prometheus.NewServerMetrics(
		grpc_prometheus.WithServerHandlingTimeHistogram(grpc_prometheus.WithHistogramBuckets([]float64{0.01, 0.1, 0.25, 0.5, 1.0, 2.5})),
	)

	//nolint:contextcheck
	serverOptions := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			server.UnaryRequestLogger(logger),
			server.RateLimitUnaryServerInterceptor(limiter),
			metrics.UnaryServerInterceptor(),
			grpc_recovery.UnaryServerInterceptor(recoveryOpt),
		),
		grpc.ChainStreamInterceptor(
			server.StreamRequestLogger(logger),
			server.RateLimitStreamServerInterceptor(limiter),
			metrics.StreamServerInterceptor(),
			grpc_recovery.StreamServerInterceptor(recoveryOpt),
		),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime: 10 * time.Second,
		}),
		grpc.SharedWriteBuffer(true),
		grpc.ReadBufferSize(16 * 1024),
		grpc.WriteBufferSize(16 * 1024),
	}

	srv := server.NewClusterServer(state, ctx.Done(), options.RedirectEndpoint)

	s := grpc.NewServer(serverOptions...)
	pb.RegisterClusterServer(s, srv)

	metrics.InitializeMetrics(s)

	return s, srv, limiter, metrics
}

// Run starts the service with the given options.
//
//nolint:gocognit,gocyclo,cyclop
func Run(ctx context.Context, options Options, logger *zap.Logger) error {
	logger.Info("service starting")
	defer logger.Info("service shut down")

	server.TrustXRealIP(options.TrustXRealIP)

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

	s, srv, limiter, metrics := newGRPCServer(ctx, state, options, logger)

	lis, err := net.Listen("tcp", options.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	landingHandler := landing.Handler(state, logger)

	eg, ctx := errgroup.WithContext(ctx)

	var rootHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			s.ServeHTTP(w, r)
		} else {
			landingHandler.ServeHTTP(w, r)
		}
	})

	insecure := options.CertificatePath == "" && options.KeyPath == ""

	if insecure {
		rootHandler = h2c.NewHandler(rootHandler, &http2.Server{})
	}

	var tlsConfig *tls.Config

	if !insecure {
		certLoader := NewDynamicCertificate(options.CertificatePath, options.KeyPath)
		if err = certLoader.Load(); err != nil {
			return fmt.Errorf("failed to load certificate: %w", err)
		}

		eg.Go(func() error {
			return certLoader.WatchWithRestarts(ctx, logger)
		})

		tlsConfig = &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certLoader.GetCertificate,
		}
	}

	mainServer := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           rootHandler,
		TLSConfig:         tlsConfig,
		ErrorLog:          zap.NewStdLog(logger.With(zap.String("server", "http"))),
	}

	if err = http2.ConfigureServer(mainServer, nil); err != nil {
		return fmt.Errorf("failed to configure server: %w", err)
	}

	if stateStorage != nil {
		eg.Go(func() error {
			return stateStorage.Start(ctx, clockwork.NewRealClock(), options.SnapshotInterval)
		})
	}

	eg.Go(func() error {
		logger.Info("API server starting", zap.Stringer("address", lis.Addr()))

		var serveErr error

		if insecure {
			serveErr = mainServer.Serve(lis)
		} else {
			serveErr = mainServer.ServeTLS(lis, "", "")
		}

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("failed to serve: %w", serveErr)
		}

		return nil
	})

	if options.LandingServerEnabled {
		var landingLis net.Listener

		landingLis, err = net.Listen("tcp", options.LandingAddr)
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		landingServer := http.Server{
			Handler: landingHandler,
		}

		eg.Go(func() error {
			logger.Info("landing server starting", zap.Stringer("address", landingLis.Addr()))

			if serveErr := landingServer.Serve(landingLis); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				return fmt.Errorf("failed to serve: %w", serveErr)
			}

			return nil
		})

		eg.Go(func() error {
			<-ctx.Done()

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			return landingServer.Shutdown(shutdownCtx) //nolint:contextcheck
		})
	}

	if options.MetricsServerEnabled {
		var metricsMux http.ServeMux

		metricsMux.Handle("/metrics", promhttp.Handler())

		metricsServer := http.Server{
			Addr:    options.MetricsAddr,
			Handler: &metricsMux,
		}

		eg.Go(func() error {
			logger.Info("metrics starting", zap.String("address", metricsServer.Addr))

			if serveErr := metricsServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				return serveErr
			}

			return nil
		})

		eg.Go(func() error {
			<-ctx.Done()

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			return metricsServer.Shutdown(shutdownCtx) //nolint:contextcheck
		})
	}

	eg.Go(func() error {
		<-ctx.Done()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		return mainServer.Shutdown(shutdownCtx) //nolint:contextcheck
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
		collectors := []prom.Collector{state, srv, metrics}

		if stateStorage != nil {
			collectors = append(collectors, stateStorage)
		}

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

func unregisterCollectors(registerer prom.Registerer, collectors ...prom.Collector) {
	for _, collector := range collectors {
		registerer.Unregister(collector)
	}
}

func registerCollectors(registerer prom.Registerer, collectors ...prom.Collector) (err error) {
	for _, collector := range collectors {
		if err = registerer.Register(collector); err != nil {
			return fmt.Errorf("failed to register collector: %w", err)
		}
	}

	return nil
}

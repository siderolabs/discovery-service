// Copyright (c) 2026 Sidero Labs, Inc.
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
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/jonboulle/clockwork"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	cryptotls "github.com/siderolabs/crypto/tls"
	"github.com/siderolabs/discovery-api/api/v1alpha1/server/pb"
	"github.com/siderolabs/go-debug"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/discovery-service/internal/landing"
	"github.com/siderolabs/discovery-service/internal/limiter"
	"github.com/siderolabs/discovery-service/internal/protomux"
	"github.com/siderolabs/discovery-service/internal/state"
	storageinternal "github.com/siderolabs/discovery-service/internal/state/storage"
	"github.com/siderolabs/discovery-service/internal/stats"
	"github.com/siderolabs/discovery-service/pkg/limits"
	"github.com/siderolabs/discovery-service/pkg/server"
	"github.com/siderolabs/discovery-service/pkg/storage"
)

// Options are the configuration options for the service.
type Options struct {
	// MetricsRegisterer is where the service collectors are registered.
	//
	// When unset, the collectors are not registered anywhere.
	MetricsRegisterer prom.Registerer

	// MetricsGatherer is what the metrics server serves.
	//
	// Defaults to MetricsRegisterer when it is a registry (which can both register and gather),
	// so a custom registry only exposes what was registered with it (the Go runtime and process
	// collectors come with prom.DefaultRegisterer). It must be set explicitly when
	// MetricsRegisterer can't gather (e.g. prom.WrapRegistererWith), and it defaults to
	// prom.DefaultGatherer when MetricsRegisterer is unset.
	MetricsGatherer prom.Gatherer

	ListenAddr   string
	LandingAddr  string
	MetricsAddr  string
	SnapshotPath string

	// SnapshotStore is an optional store for snapshots.
	//
	// When set, SnapshotPath is ignored, and the snapshots are read from and written to the provided store.
	SnapshotStore storage.SnapshotStore

	DebugAddr string

	CertificatePath, KeyPath string

	RedirectEndpoint string

	GCInterval       time.Duration
	SnapshotInterval time.Duration

	LandingServerEnabled     bool
	DebugServerEnabled       bool
	MetricsServerEnabled     bool
	SnapshotsEnabled         bool
	TrustXRealIP             bool
	TrustFirstXForwadedFor   bool
	DisableClientIPReporting bool
}

func newGRPCServer(
	ctx context.Context,
	state *state.State,
	options Options,
	logger *zap.Logger,
) (*grpc.Server, *server.ClusterServer, *limiter.IPRateLimiter, *grpc_prometheus.ServerMetrics) {
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
		// The service holds a very large number of mostly idle streams, so per-connection
		// buffers dominate the heap. The write buffer is pooled by gRPC and released on
		// every flush, while the read buffer is not (it is only pooled for raw TCP
		// connections, and TLS is terminated here), so reading straight from the
		// connection is cheaper: crypto/tls does its own record buffering anyway.
		grpc.ReadBufferSize(0),
		grpc.WriteBufferSize(16 * 1024),
	}

	srv := server.NewClusterServerWithOptions(state, ctx.Done(), server.ClusterServerOptions{
		RedirectEndpoint:         options.RedirectEndpoint,
		DisableClientIPReporting: options.DisableClientIPReporting,
	})

	s := grpc.NewServer(serverOptions...)
	pb.RegisterClusterServer(s, srv)

	metrics.InitializeMetrics(s)

	return s, srv, limiter, metrics
}

// Run starts the service with the given options.
//
//nolint:gocognit,gocyclo,cyclop,maintidx
func Run(ctx context.Context, options Options, logger *zap.Logger) error {
	logger.Info("service starting")
	defer logger.Info("service shut down")

	server.TrustXRealIP(options.TrustXRealIP)
	server.TrustFirstXForwardedFor(options.TrustFirstXForwadedFor)

	state := state.NewState(logger)

	var stateStorage *storageinternal.Storage

	var err error

	if options.SnapshotsEnabled {
		if options.SnapshotStore == nil {
			options.SnapshotStore = &storageinternal.FileStore{Path: options.SnapshotPath}
		}

		stateStorage = storageinternal.New(options.SnapshotStore, state, logger)
		if err = stateStorage.Load(ctx); err != nil {
			logger.Warn("failed to load state from storage", zap.Error(err))
		}
	} else {
		logger.Info("snapshots are disabled")
	}

	var metricsGatherer prom.Gatherer

	if options.MetricsServerEnabled {
		metricsGatherer, err = resolveGatherer(options)
		if err != nil {
			return err
		}
	}

	// the gRPC handlers are unblocked by this context on shutdown, so it has to be the errgroup
	// one: a failure of any errgroup member should still allow a graceful stop to complete
	eg, ctx := errgroup.WithContext(ctx)

	connMetrics := protomux.NewMetrics()

	s, srv, limiter, metrics := newGRPCServer(ctx, state, options, logger)

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", options.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	siteMux := http.NewServeMux()
	siteMux.Handle("/stats", stats.Handler(state, logger))
	siteMux.Handle("/", landing.Handler(state, logger))

	insecure := options.CertificatePath == "" && options.KeyPath == ""

	var tlsConfig *tls.Config

	if !insecure {
		certLoader := cryptotls.NewDynamicCertificate(options.CertificatePath, options.KeyPath)
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

	// gRPC and the static assets share the listener, but they are served by two different
	// servers: gRPC clients speak HTTP/2 and are handed to the native gRPC server, everything
	// else is served over HTTP/1.1.
	mux := protomux.New(lis, tlsConfig, connMetrics, logger)

	// only HTTP/1.1 connections reach this server, so HTTP/2 support is not needed
	var protocols http.Protocols

	protocols.SetHTTP1(true)

	mainServer := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           siteMux,
		Protocols:         &protocols,
		ConnState:         protomux.HTTPConnState(connMetrics),
		ErrorLog:          zap.NewStdLog(logger.With(zap.String("server", "http"))),
	}

	if stateStorage != nil {
		eg.Go(func() error {
			return stateStorage.Start(ctx, clockwork.NewRealClock(), options.SnapshotInterval)
		})
	}

	eg.Go(func() error {
		logger.Info("API server starting", zap.Stringer("address", lis.Addr()))

		return mux.Run(ctx)
	})

	// on shutdown the mux closes both listeners, which surfaces as net.ErrClosed in the servers
	eg.Go(func() error {
		if serveErr := s.Serve(mux.H2Listener()); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) && !errors.Is(serveErr, net.ErrClosed) {
			return fmt.Errorf("failed to serve gRPC: %w", serveErr)
		}

		return nil
	})

	eg.Go(func() error {
		if serveErr := mainServer.Serve(mux.HTTPListener()); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
			return fmt.Errorf("failed to serve: %w", serveErr)
		}

		return nil
	})

	if options.LandingServerEnabled {
		var landingLis net.Listener

		landingLis, err = (&net.ListenConfig{}).Listen(ctx, "tcp", options.LandingAddr)
		if err != nil {
			return fmt.Errorf("failed to listen: %w", err)
		}

		landingServer := http.Server{
			Handler: siteMux,
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

		metricsMux.Handle("/metrics", promhttp.HandlerFor(metricsGatherer, promhttp.HandlerOpts{}))

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

		// the Watch handlers are unblocked by the same (errgroup) context, so a graceful
		// stop completes quickly; Stop is a safety net for a stuck connection
		stopped := make(chan struct{})

		go func() {
			defer close(stopped)

			s.GracefulStop()
		}()

		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			s.Stop()
		}

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
		collectors := []prom.Collector{state, srv, metrics, connMetrics}

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

// resolveGatherer picks the gatherer for the metrics server, see Options.MetricsGatherer.
func resolveGatherer(options Options) (prom.Gatherer, error) {
	if options.MetricsGatherer != nil {
		return options.MetricsGatherer, nil
	}

	if options.MetricsRegisterer == nil {
		return prom.DefaultGatherer, nil
	}

	// registries implement both interfaces, so this is the very registry the collectors are
	// registered with
	if gatherer, ok := options.MetricsRegisterer.(prom.Gatherer); ok {
		return gatherer, nil
	}

	// silently serving another registry would leave the service metrics out of /metrics
	return nil, errors.New("MetricsRegisterer can't gather metrics, MetricsGatherer must be set")
}

func recoveryHandler(logger *zap.Logger) grpc_recovery.RecoveryHandlerFunc {
	return func(p any) error {
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

// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/talos-systems/discovery-api/api/v1alpha1/server/pb"
	"github.com/talos-systems/go-debug"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/talos-systems/discovery-service/internal/landing"
	_ "github.com/talos-systems/discovery-service/internal/proto"
	"github.com/talos-systems/discovery-service/internal/state"
	"github.com/talos-systems/discovery-service/pkg/server"
)

var (
	listenAddr  = ":3000"
	landingAddr = ":3001"
	metricsAddr = ":2122"
	debugAddr   = ":2123"
	devMode     = false
	gcInterval  = time.Minute
)

func init() {
	flag.StringVar(&listenAddr, "addr", listenAddr, "addr on which to listen")
	flag.StringVar(&landingAddr, "landing-addr", landingAddr, "addr on which to listen for landing page")
	flag.StringVar(&metricsAddr, "metrics-addr", metricsAddr, "prometheus metrics listen addr")
	flag.BoolVar(&devMode, "debug", devMode, "enable debug mode")
	flag.DurationVar(&gcInterval, "gc-interval", gcInterval, "garbage collection interval")

	if debug.Enabled {
		flag.StringVar(&debugAddr, "debug-addr", debugAddr, "debug (pprof, trace, expvar) listen addr")
	}
}

func main() {
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalln("failed to initialize logger:", err)
	}

	if os.Getenv("MODE") == "dev" {
		devMode = true
	}

	if devMode {
		logger, err = zap.NewDevelopment()

		if err != nil {
			log.Fatalln("failed to initialize development logger:", err)
		}
	}

	zap.ReplaceGlobals(logger)
	zap.RedirectStdLog(logger)

	if err = signalHandler(context.Background(), logger, run); err != nil {
		logger.Error("service failed", zap.Error(err))

		os.Exit(1)
	}
}

func signalHandler(ctx context.Context, logger *zap.Logger, f func(ctx context.Context, logger *zap.Logger) error) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return f(ctx, logger)
}

func recoveryHandler(logger *zap.Logger) grpc_recovery.RecoveryHandlerFunc {
	return func(p interface{}) error {
		if logger != nil {
			logger.Error("grpc panic", zap.Any("panic", p), zap.Stack("stack"))
		}

		return status.Errorf(codes.Internal, "%v", p)
	}
}

func run(ctx context.Context, logger *zap.Logger) error {
	logger.Info("service starting")

	defer logger.Info("service shut down")

	// Recovery is installed as the the first middleware in the chain to handle panics (via defer and recover()) in all subsequent middlewares.

	// Logging is installed as the first middleware (even before recovery middleware) in the chain
	// so that request in the form it was received and status sent on the wire is logged (error/success).
	// It also tracks the whole duration of the request, including other middleware overhead.
	grpc_zap.ReplaceGrpcLoggerV2(logger)

	recoveryOpt := grpc_recovery.WithRecoveryHandler(recoveryHandler(logger))

	serverOptions := []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(server.FieldExtractor)),
			server.AddPeerAddressUnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(logger),
			grpc_prometheus.UnaryServerInterceptor,
			grpc_recovery.UnaryServerInterceptor(recoveryOpt),
		),
		grpc_middleware.WithStreamServerChain(
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(server.FieldExtractor)),
			server.AddPeerAddressStreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(logger),
			grpc_prometheus.StreamServerInterceptor,
			grpc_recovery.StreamServerInterceptor(recoveryOpt),
		),
	}

	state := state.NewState(logger)
	prom.MustRegister(state)

	srv := server.NewClusterServer(state, ctx.Done())
	prom.MustRegister(srv)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	landingLis, err := net.Listen("tcp", landingAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s := grpc.NewServer(serverOptions...)
	pb.RegisterClusterServer(s, srv)

	// TODO(aleksi): tweak buckets once we know the actual distribution
	buckets := []float64{0.01, 0.1, 0.25, 0.5, 1.0, 2.5}
	grpc_prometheus.EnableHandlingTimeHistogram(grpc_prometheus.WithHistogramBuckets(buckets))
	grpc_prometheus.Register(s)

	var metricsMux http.ServeMux

	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsServer := http.Server{
		Addr:    metricsAddr,
		Handler: &metricsMux,
	}

	landingServer := http.Server{
		Handler: landing.Handler(),
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		logger.Info("gRPC server starting", zap.Stringer("address", lis.Addr()))

		if err := s.Serve(lis); err != nil {
			return fmt.Errorf("failed to serve: %w", err)
		}

		return nil
	})

	eg.Go(func() error {
		logger.Info("landing server starting", zap.Stringer("address", landingLis.Addr()))

		if err := landingServer.Serve(landingLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to serve: %w", err)
		}

		return nil
	})

	eg.Go(func() error {
		logger.Info("metrics starting", zap.String("address", metricsServer.Addr))

		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	})

	eg.Go(func() error {
		<-ctx.Done()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		s.GracefulStop()
		landingServer.Shutdown(ctx)         //nolint:errcheck
		metricsServer.Shutdown(shutdownCtx) //nolint:errcheck

		return nil
	})

	eg.Go(func() error {
		state.RunGC(ctx, logger, gcInterval)

		return nil
	})

	eg.Go(func() error {
		return debug.ListenAndServe(ctx, debugAddr, func(msg string) { logger.Info(msg) })
	})

	return eg.Wait()
}

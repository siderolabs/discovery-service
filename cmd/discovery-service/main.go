// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package main implements the discovery service entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/siderolabs/go-debug"
	_ "github.com/siderolabs/proto-codec/codec" // enable vtproto codecV2
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/siderolabs/discovery-service/pkg/service"
)

const defaultLogLevel = zap.InfoLevel

var (
	listenAddr              = ":3000"
	landingAddr             = ":3001"
	metricsAddr             = ":2122"
	debugAddr               = ":2123"
	debugMode               = false
	gcInterval              = time.Minute
	redirectEndpoint        = ""
	snapshotsEnabled        = true
	snapshotPath            = "/var/discovery-service/state.binpb"
	snapshotInterval        = 10 * time.Minute
	certificatePath         = ""
	keyPath                 = ""
	trustXRealIP            = true
	trustFirstXForwardedFor = true
	logLevelStr             = defaultLogLevel.String()
)

func init() {
	flag.StringVar(&listenAddr, "addr", listenAddr, "addr on which to listen")
	flag.StringVar(&certificatePath, "certificate-path", certificatePath, "path to the certificate file")
	flag.StringVar(&keyPath, "key-path", keyPath, "path to the key file")
	flag.StringVar(&landingAddr, "landing-addr", landingAddr, "addr on which to listen for landing page (set to empty to disable)")
	flag.StringVar(&metricsAddr, "metrics-addr", metricsAddr, "prometheus metrics listen addr (set to empty to disable)")
	flag.BoolVar(&debugMode, "debug", debugMode, "enable debug mode")
	flag.DurationVar(&gcInterval, "gc-interval", gcInterval, "garbage collection interval")
	flag.StringVar(&redirectEndpoint, "redirect-endpoint", redirectEndpoint, "redirect all clients to a new endpoint (gRPC endpoint, e.g. 'example.com:443'")
	flag.BoolVar(&snapshotsEnabled, "snapshots-enabled", snapshotsEnabled, "enable snapshots")
	flag.StringVar(&snapshotPath, "snapshot-path", snapshotPath, "path to the snapshot file")
	flag.DurationVar(&snapshotInterval, "snapshot-interval", snapshotInterval, "interval to save the snapshot")
	flag.BoolVar(&trustXRealIP, "trust-x-real-ip", trustXRealIP, "trust X-Real-IP header")
	flag.BoolVar(&trustFirstXForwardedFor, "trust-first-x-forwarded-for", trustFirstXForwardedFor, "trust the first entry in the X-Forwarded-For header")
	flag.StringVar(
		&logLevelStr,
		"log-level",
		defaultLogLevel.String(),
		fmt.Sprintf(
			"set log level: debug, info, warn, error, dpanic, panic, fatal (default: %s)",
			defaultLogLevel.String(),
		),
	)

	if debug.Enabled {
		flag.StringVar(&debugAddr, "debug-addr", debugAddr, "debug (pprof, trace, expvar) listen addr")
	}
}

func main() {
	flag.Parse()

	logLevel, err := resolveLogLevel(logLevelStr)
	if err != nil {
		log.Fatalln("invalid log level:", err)
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(logLevel)

	logger, err := cfg.Build()
	if err != nil {
		log.Fatalln("failed to initialize logger:", err)
	}

	if os.Getenv("MODE") == "dev" {
		debugMode = true
	}

	if debugMode {
		logger, err = zap.NewDevelopment()
		if err != nil {
			log.Fatalln("failed to initialize development logger:", err)
		}
	}

	logger.Info(fmt.Sprintf("log level set to %q", logger.Level().String()))

	zap.ReplaceGlobals(logger)
	zap.RedirectStdLog(logger)

	if err = signalHandler(context.Background(), logger, func(ctx context.Context, logger *zap.Logger) error {
		return service.Run(ctx, service.Options{
			SnapshotsEnabled: snapshotsEnabled,
			SnapshotPath:     snapshotPath,
			SnapshotInterval: snapshotInterval,

			RedirectEndpoint: redirectEndpoint,
			ListenAddr:       listenAddr,
			GCInterval:       gcInterval,

			CertificatePath: certificatePath,
			KeyPath:         keyPath,

			LandingServerEnabled: landingAddr != "",
			LandingAddr:          landingAddr,

			DebugServerEnabled: debugAddr != "",
			DebugAddr:          debugAddr,

			MetricsServerEnabled: metricsAddr != "",
			MetricsAddr:          metricsAddr,

			MetricsRegisterer: prometheus.DefaultRegisterer,

			TrustXRealIP:           trustXRealIP,
			TrustFirstXForwadedFor: trustFirstXForwardedFor,
		}, logger)
	}); err != nil {
		logger.Error("service failed", zap.Error(err))

		os.Exit(1)
	}
}

func signalHandler(ctx context.Context, logger *zap.Logger, f func(ctx context.Context, logger *zap.Logger) error) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return f(ctx, logger)
}

func resolveLogLevel(s string) (zapcore.Level, error) {
	if s == "" {
		return defaultLogLevel, nil
	}

	return zapcore.ParseLevel(s)
}

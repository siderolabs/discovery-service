// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func extractFields(req any) []zapcore.Field {
	var result []zapcore.Field

	if msg, ok := req.(interface {
		GetClusterId() string
	}); ok {
		result = append(result, zap.String("cluster_id", msg.GetClusterId()))
	}

	if msg, ok := req.(interface {
		GetAffiliateId() string
	}); ok {
		result = append(result, zap.String("affiliate_id", msg.GetAffiliateId()))
	}

	if msg, ok := req.(interface {
		GetClientVersion() string
	}); ok {
		result = append(result, zap.String("client_version", msg.GetClientVersion()))
	}

	return result
}

// UnaryRequestLogger returns a new unary server interceptor that logs the requests.
func UnaryRequestLogger(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		startTime := time.Now()

		resp, err := handler(ctx, req)

		level := zapcore.InfoLevel

		if err != nil {
			level = zapcore.ErrorLevel
		}

		// building the fields is not free (peer address extraction, allocations), so skip it
		// entirely when the entry is not going to be written
		if entry := logger.Check(level, info.FullMethod); entry != nil {
			entry.Write(
				append(
					[]zapcore.Field{
						zap.Duration("duration", time.Since(startTime)),
						zap.Stringer("code", status.Code(err)),
						zap.Error(err),
						zap.Stringer("peer.address", PeerAddress(ctx)),
					},
					extractFields(req)...,
				)...,
			)
		}

		return resp, err
	}
}

// StreamRequestLogger returns a new stream server interceptor that logs the requests.
func StreamRequestLogger(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		startTime := time.Now()

		err := handler(srv, ss)

		level := zapcore.InfoLevel

		if err != nil {
			level = zapcore.ErrorLevel
		}

		if entry := logger.Check(level, info.FullMethod); entry != nil {
			entry.Write(
				zap.Duration("duration", time.Since(startTime)),
				zap.Stringer("code", status.Code(err)),
				zap.Error(err),
				zap.Stringer("peer.address", PeerAddress(ctx)),
			)
		}

		return err
	}
}

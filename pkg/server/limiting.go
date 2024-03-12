// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/discovery-service/internal/limiter"
)

func pause(ctx context.Context, limiter *limiter.IPRateLimiter) error {
	iPAddr := PeerAddress(ctx)
	if !IsZero(iPAddr) {
		limit := limiter.Get(iPAddr)

		err := limit.Wait(ctx)
		if err != nil {
			return status.Error(codes.ResourceExhausted, "rate limit exceeds request timeout")
		}
	}

	return nil
}

// RateLimitUnaryServerInterceptor limits Unary PRCs from an IPAdress.
func RateLimitUnaryServerInterceptor(limiter *limiter.IPRateLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		err = pause(ctx, limiter)
		if err != nil {
			return resp, err
		}

		return handler(ctx, req)
	}
}

// RateLimitStreamServerInterceptor limits Stream PRCs from an IPAdress.
func RateLimitStreamServerInterceptor(limiter *limiter.IPRateLimiter) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		err := pause(ctx, limiter)
		if err != nil {
			return err
		}

		return handler(srv, ss)
	}
}

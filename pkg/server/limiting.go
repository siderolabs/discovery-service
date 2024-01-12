// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"
	"net/netip"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/discovery-service/internal/limiter"
)

func pause(ctx context.Context, limiter *limiter.IPRateLimiter) error {
	iPAddr := extractIPAddressFromTags(ctx)
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
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		err = pause(ctx, limiter)
		if err != nil {
			return resp, err
		}

		return handler(ctx, req)
	}
}

// RateLimitStreamServerInterceptor limits Stream PRCs from an IPAdress.
func RateLimitStreamServerInterceptor(limiter *limiter.IPRateLimiter) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		err := pause(ctx, limiter)
		if err != nil {
			return err
		}

		return handler(srv, ss)
	}
}

func extractIPAddressFromTags(ctx context.Context) netip.Addr {
	if tags := grpc_ctxtags.Extract(ctx); tags != nil {
		values := tags.Values()
		if addrV, ok := values["peer.address"]; ok {
			if addr, ok := addrV.(netip.Addr); ok {
				return addr
			}
		}
	}

	return netip.Addr{}
}

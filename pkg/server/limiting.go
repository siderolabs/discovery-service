package server

import (
	"context"
	"time"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"google.golang.org/grpc"

	"github.com/talos-systems/discovery-service/pkg/limits"
)

// RateLimitUnaryServerInterceptor limits Unary PRCs from an IPAdress.
func RateLimitUnaryServerInterceptor(limiter *limits.IPRateLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		limiter := limits.NewIPRateLimiter(1, limits.RequestsPerSecondMax)

		iPAddr := extractIPAddressFromTags(ctx)
		if iPAddr != "" {
			limit := limiter.GetLimiter(iPAddr)
			if !limit.Allow() {
				time.Sleep(100 * time.Millisecond)
			}
		}

		return handler(ctx, req)
	}
}

// RateLimitStreamServerInterceptor limits Stream PRCs from an IPAdress.
func RateLimitStreamServerInterceptor(limiter *limits.IPRateLimiter) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		iPAddr := extractIPAddressFromTags(ctx)
		if iPAddr != "" {
			limit := limiter.GetLimiter(iPAddr)
			if !limit.Allow() {
				time.Sleep(100 * time.Millisecond)
			}
		}

		return handler(srv, ss)
	}
}

func extractIPAddressFromTags(ctx context.Context) string {
	if tags := grpc_ctxtags.Extract(ctx); tags != nil {
		values := tags.Values()
		if stringValue, ok := values["peer.address"]; ok {
			if iPString, ok := stringValue.(string); ok {
				return iPString
			}
		}
	}

	return ""
}

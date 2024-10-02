// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// fieldExtractor prepares tags for logging and tracing out of the request.
func fieldExtractor(ctx context.Context, req interface{}) logging.Fields {
	var ret logging.Fields

	if msg, ok := req.(proto.Message); ok {
		r := msg.ProtoReflect()
		fields := r.Descriptor().Fields()

		for _, name := range []string{"cluster_id", "affiliate_id", "client_version"} {
			if field := fields.ByName(protoreflect.Name(name)); field != nil {
				ret = append(ret, name, r.Get(field).String())
			}
		}
	}

	if peerAddress := PeerAddress(ctx); !IsZero(peerAddress) {
		ret = append(ret, "peer.address", peerAddress.String())
	}

	return ret
}

// AddLoggingFieldsUnaryServerInterceptor sets peer.address for logging.
func AddLoggingFieldsUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		return handler(logging.InjectFields(ctx, fieldExtractor(ctx, req)), req)
	}
}

// AddLoggingFieldsStreamServerInterceptor sets peer.address for logging.
func AddLoggingFieldsStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		wrapped := grpc_middleware.WrapServerStream(ss)
		wrapped.WrappedContext = logging.InjectFields(ctx, fieldExtractor(ctx, nil)) //nolint:fatcontext

		return handler(srv, wrapped)
	}
}

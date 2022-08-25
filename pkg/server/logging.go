// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldExtractor prepares tags for logging and tracing out of the request.
func FieldExtractor(fullMethod string, req interface{}) map[string]interface{} {
	if msg, ok := req.(proto.Message); ok {
		r := msg.ProtoReflect()
		fields := r.Descriptor().Fields()

		ret := map[string]interface{}{}

		for _, name := range []string{"cluster_id", "affiliate_id", "client_version"} {
			if field := fields.ByName(protoreflect.Name(name)); field != nil {
				ret[name] = r.Get(field).String()
			}
		}

		if len(ret) > 0 {
			return ret
		}
	}

	return nil
}

// AddPeerAddressUnaryServerInterceptor sets peer.address for logging.
func AddPeerAddressUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		extractPeerAddress(ctx)

		return handler(ctx, req)
	}
}

// AddPeerAddressStreamServerInterceptor sets peer.address for logging.
func AddPeerAddressStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		extractPeerAddress(ctx)

		return handler(srv, ss)
	}
}

func extractPeerAddress(ctx context.Context) {
	if peerAddress := PeerAddress(ctx); !IsZero(peerAddress) {
		if tags := grpc_ctxtags.Extract(ctx); tags != nil {
			tags.Set("peer.address", peerAddress.String())
		}
	}
}

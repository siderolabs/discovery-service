// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"
	"net/netip"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

var trustXRealIP bool

// TrustXRealIP enables X-Real-IP header support.
func TrustXRealIP(enabled bool) {
	trustXRealIP = enabled
}

// PeerAddress is used to extract peer address from the client.
// it will try to extract the actual client's IP when called via
// Nginx ingress first if not it will get the nginx or the machine
// which calls the server, if everything fails returns an empty address.
func PeerAddress(ctx context.Context) netip.Addr {
	if trustXRealIP {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("X-Real-IP"); vals != nil {
				if ip, err := netip.ParseAddr(vals[0]); err == nil {
					return ip
				}
			}
		}
	}

	if peer, ok := peer.FromContext(ctx); ok {
		if addrPort, err := netip.ParseAddrPort(peer.Addr.String()); err == nil {
			return addrPort.Addr()
		}
	}

	return netip.Addr{}
}

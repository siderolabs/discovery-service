// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"
	"net/netip"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

var (
	trustXRealIP            bool
	trustFirstXForwardedFor bool
)

// TrustXRealIP enables X-Real-IP header support.
func TrustXRealIP(enabled bool) {
	trustXRealIP = enabled
}

// TrustFirstXForwardedFor enables X-Forwarded-For header support.
func TrustFirstXForwardedFor(enabled bool) {
	trustFirstXForwardedFor = enabled
}

// PeerAddress is used to extract peer address from the client.
// it will try to extract the actual client's IP when called via
// Nginx ingress first if not it will get the nginx or the machine
// which calls the server, if everything fails returns an empty address.
func PeerAddress(ctx context.Context) netip.Addr {
	if trustXRealIP {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("X-Real-IP"); len(vals) > 0 {
				if ip, err := netip.ParseAddr(vals[0]); err == nil {
					return ip
				}
			}
		}
	}

	if trustFirstXForwardedFor {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("X-Forwarded-For"); len(vals) > 0 {
				first, _, _ := strings.Cut(vals[0], ",")

				if ip, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
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

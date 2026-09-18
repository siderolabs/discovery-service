// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// These are process-wide settings, but they are written by service.Run while requests of a
// previously started service may still be in flight, so they are accessed atomically.
var (
	trustXRealIP            atomic.Bool
	trustFirstXForwardedFor atomic.Bool
)

// TrustXRealIP enables X-Real-IP header support.
func TrustXRealIP(enabled bool) {
	trustXRealIP.Store(enabled)
}

// TrustFirstXForwardedFor enables X-Forwarded-For header support.
func TrustFirstXForwardedFor(enabled bool) {
	trustFirstXForwardedFor.Store(enabled)
}

// PeerAddress is used to extract peer address from the client.
// it will try to extract the actual client's IP when called via
// Nginx ingress first if not it will get the nginx or the machine
// which calls the server, if everything fails returns an empty address.
func PeerAddress(ctx context.Context) netip.Addr {
	// PeerAddress is on the hot path (it is called for every request by the rate limiter and
	// the request logger), so the metadata is looked up by key: metadata.FromIncomingContext
	// copies the whole metadata map on every call.
	if trustXRealIP.Load() {
		if vals := metadata.ValueFromIncomingContext(ctx, "x-real-ip"); len(vals) > 0 {
			if ip, err := netip.ParseAddr(vals[0]); err == nil {
				return ip
			}
		}
	}

	if trustFirstXForwardedFor.Load() {
		if vals := metadata.ValueFromIncomingContext(ctx, "x-forwarded-for"); len(vals) > 0 {
			first, _, _ := strings.Cut(vals[0], ",")

			if ip, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
				return ip
			}
		}
	}

	if peer, ok := peer.FromContext(ctx); ok {
		// fast path: avoid formatting the address into a string just to parse it back
		if tcpAddr, ok := peer.Addr.(*net.TCPAddr); ok {
			return tcpAddr.AddrPort().Addr().Unmap()
		}

		if addrPort, err := netip.ParseAddrPort(peer.Addr.String()); err == nil {
			return addrPort.Addr()
		}
	}

	return netip.Addr{}
}

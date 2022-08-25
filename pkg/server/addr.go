// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"context"
	"net"
	"net/netip"

	"go4.org/netipx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// PeerAddress is used to extract peer address from the client.
// it will try to extract the actual client's IP when called via
// Nginx ingress first if not it will get the nginx or the machine
// which calls the server, if everything fails returns an empty address.
func PeerAddress(ctx context.Context) netip.Addr {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("X-Real-IP"); vals != nil {
			if ip, err := netip.ParseAddr(vals[0]); err == nil {
				return ip
			}
		}
	}

	if peer, ok := peer.FromContext(ctx); ok {
		if addr, ok := peer.Addr.(*net.TCPAddr); ok {
			if ip, ok := netipx.FromStdIP(addr.IP); ok {
				return ip
			}
		}
	}

	return netip.Addr{}
}

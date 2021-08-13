// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package types contains all manager data types.
package types

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"inet.af/netaddr"
)

// Address describes an IP or DNS address with optional Port.
type Address struct {
	// LastReported indicates the time at which this address was last reported.
	LastReported time.Time `json:"lastReported"`
	// IP is the IP address of this NodeAddress, if known.
	IP netaddr.IP `json:"ip,omitempty"`
	// Name is the DNS name of this NodeAddress, if known.
	Name string `json:"name,omitempty"`
	// Port is the port number for this NodeAddress, if known.
	Port uint16 `json:"port,omitempty"`
}

// EqualHost indicates whether two addresses have the same host portion, ignoring the ports.
func (a *Address) EqualHost(other *Address) bool {
	if !a.IP.IsZero() || !other.IP.IsZero() {
		return a.IP == other.IP
	}

	return a.Name == other.Name
}

// Equal indicates whether two addresses are equal.
func (a *Address) Equal(other *Address) bool {
	if !a.EqualHost(other) {
		return false
	}

	return a.Port == other.Port
}

// Endpoint returns a UDP endpoint address for the Address, using the defaultPort if none is known.
func (a *Address) Endpoint(defaultPort uint16) (*net.UDPAddr, error) {
	proto := "udp"
	addr := a.Name
	port := a.Port

	if !a.IP.IsZero() {
		addr = a.IP.String()

		if a.IP.Is6() {
			proto = "udp6"
			addr = "[" + addr + "]"
		} else {
			proto = "udp4"
		}
	}

	if port == 0 {
		port = defaultPort
	}

	return net.ResolveUDPAddr(proto, fmt.Sprintf("%s:%d", addr, port))
}

// Node describes a Wireguard Peer.
type Node struct {
	// Name is the human-readable identifier of this Node.
	// Usually, this is the kubernetes Node name.
	// It *should* generally be unique, but it is not required to be so.
	Name string `json:"name,omitempty"`

	// ID is the unique identifier for this Node.
	// Usually, this is the Wireguard Public Key of the Node.
	ID string `json:"id,omitempty"`

	// IP is the IP address of the Wireguard interface on this Node.
	IP netaddr.IP `json:"ip,omitempty"`

	// Addresses is a list of addresses for the Node.
	Addresses []*Address `json:"selfIPs,omitempty"`

	mu sync.Mutex
}

// AddAddresses adds a set of addresses to a Node.
func (n *Node) AddAddresses(addresses ...*Address) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, a := range addresses {
		var found bool

		if a.LastReported.IsZero() {
			a.LastReported = time.Now()
		}

		for _, existing := range n.Addresses {
			if a.EqualHost(existing) {
				found = true

				if a.Port > 0 {
					existing.Port = a.Port
				}

				existing.LastReported = a.LastReported

				break
			}
		}

		if !found {
			n.Addresses = append(n.Addresses, a)
		}
	}
}

// ExpireAddressesOlderThan removes addresses from the Node which have not been reported within the given timeframe.
func (n *Node) ExpireAddressesOlderThan(maxAge time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	i := 0

	for _, a := range n.Addresses {
		if time.Since(a.LastReported) < maxAge {
			n.Addresses[i] = a

			i++

			continue
		}
	}

	n.Addresses = n.Addresses[:i]
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (n *Node) MarshalBinary() ([]byte, error) {
	return json.Marshal(n)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (n *Node) UnmarshalBinary(data []byte) error {
	*n = Node{}

	return json.Unmarshal(data, n)
}

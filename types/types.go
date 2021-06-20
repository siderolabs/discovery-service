package types

import (
	"time"

	"inet.af/netaddr"
)

// Node describes a Wireguard Peer
type Node struct {
	// Name is the human-readable identifier of this Node.
	// Usually, this is the kubernetes Node name.
	// It *should* generally be unique, but it is not required to be so.
	Name string `json:"name,omitempty"`

	// ID is the unique identifier for this Node.
	// Usually, this is the Wireguard Public Key of the Node.
	ID string `json:"id,omitempty"`

	// IP is the Wireguard interface IP of this Node.
	IP netaddr.IP `json:"ip,omitempty"`

	// KnownEndpoints is a list of known endpoints (host:port) for this Node.
	KnownEndpoints []*KnownEndpoint `json:"knownEndpoints,omitempty"`

	// SelfAddresses is a list of addresses assigned to the Node itself, either directly or via NAT.
	SelfIPs []string `json:"selfIPs,omitempty"`
}

type KnownEndpoint struct {
	// Endpoint describes the IP:Port of the known-good connection
	Endpoint netaddr.IPPort `json:"endpoint"`

	// LastConnected records the time at which the endpoint was last reported to be good.
	LastConnected time.Time `json:"lastConnected"`
}

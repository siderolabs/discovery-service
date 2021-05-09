package types

// Node describes a Wireguard Peer
type Node struct {
	// Name is the human-readable identifier of this Node.
	// Usually, this is the kubernetes Node name.
	// It *should* generally be unique, but it is not required to be so.
	Name string

	// ID is the unique identifier for this Node.
	// Usually, this is the Wireguard Public Key of the Node.
   ID string

	// IP is the Wireguard interface IP of this Node.
	IP string

	// KnownEndpoints is a list of known endpoints (host:port) for this Node.
   KnownEndpoints []string
}

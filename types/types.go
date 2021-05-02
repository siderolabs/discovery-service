package types

// Node describes a Wireguard Peer
type Node struct {
   ID string

   KnownEndpoints []string
}

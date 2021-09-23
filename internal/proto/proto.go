// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package proto contains wrappers to inject vtprotobuf marshaling.
package proto

import (
	"fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	protov2 "google.golang.org/protobuf/proto" //nolint:gci
)

// Codec provides protobuf encoding.Codec.
type Codec struct{}

// Marshal implements encoding.Codec.
func (Codec) Marshal(v interface{}) ([]byte, error) {
	// our types implement Message (with or without vtproto additions depending on build configuration)
	if m, ok := v.(Message); ok {
		return Marshal(m)
	}

	// no types implement protobuf API v1 only, so don't check for it

	return nil, fmt.Errorf("failed to marshal %T", v)
}

// Unmarshal implements encoding.Codec.
func (Codec) Unmarshal(data []byte, v interface{}) error {
	// our types implement Message (with or without vtproto additions depending on build configuration)
	if m, ok := v.(Message); ok {
		return Unmarshal(data, m)
	}

	// no types implement protobuf API v1 only, so don't check for it

	return fmt.Errorf("failed to unmarshal %T", v)
}

// Name implements encoding.Codec.
func (Codec) Name() string {
	return proto.Name // overrides google.golang.org/grpc/encoding/proto codec
}

// Message is the main interface for protobuf API v2 messages.
type Message = protov2.Message

// vtprotoMessage is the interface for vtproto additions.
//
// We use only a subset of that interface but include additional methods
// to prevent accidental successful type assertion for unrelated types.
type vtprotoMessage interface {
	MarshalVT() ([]byte, error)
	MarshalToVT([]byte) (int, error)
	MarshalToSizedBufferVT([]byte) (int, error)
	UnmarshalVT([]byte) error
}

// Marshal returns the wire-format encoding of m.
func Marshal(m Message) ([]byte, error) {
	if vm, ok := m.(vtprotoMessage); ok {
		return vm.MarshalVT()
	}

	return protov2.Marshal(m)
}

// Unmarshal parses the wire-format message in b and places the result in m.
// The provided message must be mutable (e.g., a non-nil pointer to a message).
func Unmarshal(b []byte, m Message) error {
	if vm, ok := m.(vtprotoMessage); ok {
		return vm.UnmarshalVT(b)
	}

	return protov2.Unmarshal(b, m)
}

func init() {
	encoding.RegisterCodec(Codec{})
}

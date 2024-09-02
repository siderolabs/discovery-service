// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package proto contains wrappers to inject vtprotobuf marshaling.
package proto

import (
	"fmt"

	"google.golang.org/grpc/encoding"
	protoenc "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/proto"
)

// Codec provides protobuf [encoding.CodecV2].
type Codec struct{}

// Marshal implements [encoding.CodecV2].
func (Codec) Marshal(v any) (mem.BufferSlice, error) {
	size, err := getSize(v)
	if err != nil {
		return nil, err
	}

	if mem.IsBelowBufferPoolingThreshold(size) {
		buf, err := marshal(v)
		if err != nil {
			return nil, err
		}

		return mem.BufferSlice{mem.SliceBuffer(buf)}, nil
	}

	pool := mem.DefaultBufferPool()

	buf := pool.Get(size)
	if err := marshalAppend((*buf)[:size], v); err != nil {
		pool.Put(buf)

		return nil, err
	}

	return mem.BufferSlice{mem.NewBuffer(buf, pool)}, nil
}

func getSize(v any) (int, error) {
	// our types implement Message (with or without vtproto additions depending on build configuration)
	// no types implement protobuf API v1 only, so don't check for it
	switch v := v.(type) {
	case vtprotoMessage:
		return v.SizeVT(), nil
	case proto.Message:
		return proto.Size(v), nil
	default:
		return -1, fmt.Errorf("failed to get size, message is %T, must satisfy the vtprotoMessage, proto.Message or protoadapt.MessageV1 ", v)
	}
}

func marshal(v any) ([]byte, error) {
	// our types implement Message (with or without vtproto additions depending on build configuration)
	// no types implement protobuf API v1 only, so don't check for it
	switch v := v.(type) {
	case vtprotoMessage:
		return v.MarshalVT()
	case proto.Message:
		return proto.Marshal(v)
	default:
		return nil, fmt.Errorf("failed to marshal, message is %T, must satisfy the vtprotoMessage, proto.Message or protoadapt.MessageV1 ", v)
	}
}

func marshalAppend(dst []byte, v any) error {
	takeErr := func(_ any, e error) error { return e }

	// our types implement Message (with or without vtproto additions depending on build configuration)
	// no types implement protobuf API v1 only, so don't check for it
	switch v := v.(type) {
	case vtprotoMessage:
		return takeErr(v.MarshalToSizedBufferVT(dst))
	case proto.Message:
		return takeErr((proto.MarshalOptions{}).MarshalAppend(dst[:0], v))
	default:
		return fmt.Errorf("failed to marshal-append, message is %T, must satisfy the vtprotoMessage, proto.Message or protoadapt.MessageV1 ", v)
	}
}

// Unmarshal implements [encoding.CodecV2].
func (Codec) Unmarshal(data mem.BufferSlice, v any) error {
	buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
	defer buf.Free()

	// our types implement Message (with or without vtproto additions depending on build configuration)
	// no types implement protobuf API v1 only, so don't check for it
	switch v := v.(type) {
	case vtprotoMessage:
		return v.UnmarshalVT(buf.ReadOnlyData())
	case Message:
		return proto.Unmarshal(buf.ReadOnlyData(), v)
	default:
		return fmt.Errorf("failed to unmarshal, message is %T, must satisfy the vtprotoMessage, proto.Message or protoadapt.MessageV1", v)
	}
}

// Name implements encoding.Codec.
func (Codec) Name() string { return protoenc.Name } // overrides google.golang.org/grpc/encoding/proto codec

// Message is the main interface for protobuf API v2 messages.
type Message = proto.Message

// vtprotoMessage is the interface for vtproto additions.
//
// We use only a subset of that interface but include additional methods
// to prevent accidental successful type assertion for unrelated types.
type vtprotoMessage interface {
	MarshalVT() ([]byte, error)
	MarshalToSizedBufferVT([]byte) (int, error)
	UnmarshalVT([]byte) error
	SizeVT() int
}

// Marshal returns the wire-format encoding of m.
func Marshal(m Message) ([]byte, error) {
	// our types implement Message (with or without vtproto additions depending on build configuration)
	if vm, ok := m.(vtprotoMessage); ok {
		return vm.MarshalVT()
	}

	// no types implement protobuf API v1 only, so don't check for it

	return proto.Marshal(m)
}

// Unmarshal parses the wire-format message in b and places the result in m.
// The provided message must be mutable (e.g., a non-nil pointer to a message).
func Unmarshal(b []byte, m Message) error {
	// our types implement Message (with or without vtproto additions depending on build configuration)
	if vm, ok := m.(vtprotoMessage); ok {
		return vm.UnmarshalVT(b)
	}

	// no types implement protobuf API v1 only, so don't check for it

	return proto.Unmarshal(b, m)
}

func init() {
	encoding.RegisterCodecV2(Codec{})
}

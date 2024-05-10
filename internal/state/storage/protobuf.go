// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package storage

import (
	"errors"
	"fmt"
	"io"
	"slices"

	"google.golang.org/protobuf/encoding/protowire"

	storagepb "github.com/siderolabs/discovery-service/api/storage"
)

const (
	clustersFieldNum  = 1
	clustersFieldType = protowire.BytesType

	// MaxClusterSize is the maximum allowed size of a cluster snapshot.
	MaxClusterSize = 138578179
)

// ErrClusterSnapshotTooLarge is returned when a cluster snapshot is above the maximum size of MaxClusterSize.
var ErrClusterSnapshotTooLarge = fmt.Errorf("cluster snapshot is above the maximum size of %qB", MaxClusterSize)

// encodeClusterSnapshot encodes a ClusterSnapshot into the given buffer, resizing it as needed.
//
// It returns the buffer with the encoded ClusterSnapshot and an error if the encoding fails.
func encodeClusterSnapshot(buffer []byte, snapshot *storagepb.ClusterSnapshot) ([]byte, error) {
	buffer = protowire.AppendTag(buffer, clustersFieldNum, clustersFieldType)
	size := snapshot.SizeVT()
	buffer = protowire.AppendVarint(buffer, uint64(size))

	startIdx := len(buffer)
	clusterSize := size

	buffer = slices.Grow(buffer, clusterSize)
	buffer = buffer[:startIdx+clusterSize]

	if _, err := snapshot.MarshalToSizedBufferVT(buffer[startIdx:]); err != nil {
		return nil, fmt.Errorf("failed to marshal cluster: %w", err)
	}

	return buffer, nil
}

// decodeClusterSnapshot decodes a ClusterSnapshot from the given buffer.
func decodeClusterSnapshot(buffer []byte) (*storagepb.ClusterSnapshot, error) {
	var clusterSnapshot storagepb.ClusterSnapshot

	if err := clusterSnapshot.UnmarshalVT(buffer); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster snapshot: %w", err)
	}

	return &clusterSnapshot, nil
}

// decodeClusterSnapshotHeader reads and decodes the header of a ClusterSnapshot from the given io.ByteReader.
//
// It returns the size of the header and the size of the snapshot.
func decodeClusterSnapshotHeader(r io.ByteReader) (headerSize, clusterSize int, err error) {
	tagNum, tagType, tagEncodedLen, err := consumeTag(r)
	if err != nil {
		return 0, 0, err
	}

	if tagNum != clustersFieldNum {
		return 0, 0, fmt.Errorf("unexpected number: %v", tagNum)
	}

	if tagType != clustersFieldType {
		return 0, 0, fmt.Errorf("unexpected type: %v", tagType)
	}

	clusterSizeVal, clusterSizeEncodedLen, err := consumeVarint(r)
	if clusterSizeEncodedLen < 0 {
		return 0, 0, err
	}

	if clusterSizeVal > MaxClusterSize {
		return 0, 0, fmt.Errorf("%w: %v", ErrClusterSnapshotTooLarge, clusterSizeVal)
	}

	return tagEncodedLen + clusterSizeEncodedLen, int(clusterSizeVal), nil
}

// consumeTag reads a varint-encoded tag from the given io.ByteReader, reporting its length.
//
// It is an adaptation of protowire.ConsumeTag to work with io.ByteReader.
//
// It returns the tag number, the tag type, the length of the tag, and an error if the tag is invalid.
func consumeTag(r io.ByteReader) (protowire.Number, protowire.Type, int, error) {
	v, n, err := consumeVarint(r)
	if err != nil {
		return 0, 0, 0, err
	}

	num, typ := protowire.DecodeTag(v)
	if num < protowire.MinValidNumber {
		return 0, 0, 0, errors.New("invalid field number")
	}

	return num, typ, n, nil
}

// consumeVarint parses a varint-encoded uint64 from the given io.ByteReader, reporting its length.
//
// It is an adaptation of protowire.ConsumeVarint to work with io.ByteReader.
//
// It returns the parsed value, the length of the varint, and an error if the varint is invalid.
func consumeVarint(r io.ByteReader) (uint64, int, error) {
	var v uint64

	for i := range 10 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, 0, err
		}

		y := uint64(b)
		v += y << uint(i*7)

		if y < 0x80 {
			return v, i + 1, nil
		}

		v -= 0x80 << uint(i*7)
	}

	return 0, 0, errors.New("variable length integer overflow")
}

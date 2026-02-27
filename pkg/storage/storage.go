// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package storage provides an interface for reading and writing discovery service snapshots.
package storage

import (
	"context"
	"io"
)

// SnapshotStore is an interface for reading and writing discovery service snapshots.
type SnapshotStore interface {
	// Reader returns a reader for the snapshot.
	Reader(context.Context) (io.ReadCloser, error)
	// Writer returns a writer for the snapshot.
	Writer(context.Context) (io.WriteCloser, error)
}

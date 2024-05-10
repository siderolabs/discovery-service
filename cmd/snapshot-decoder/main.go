// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package main implements a simple tool to decode a snapshot file.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	storagepb "github.com/siderolabs/discovery-service/api/storage"
)

var snapshotPath = "/var/discovery-service/state.binpb"

func init() {
	flag.StringVar(&snapshotPath, "snapshot-path", snapshotPath, "path to the snapshot file")
}

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to read snapshot: %w", err)
	}

	snapshot := &storagepb.StateSnapshot{}

	if err = snapshot.UnmarshalVT(data); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	opts := protojson.MarshalOptions{
		Indent: "  ",
	}

	json, err := opts.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if _, err = io.Copy(os.Stdout, bytes.NewReader(json)); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	fmt.Println()

	return nil
}

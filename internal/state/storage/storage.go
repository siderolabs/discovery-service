// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package storage implements persistent storage for the state of the discovery service.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"

	"github.com/siderolabs/discovery-service/internal/state"
)

const bucketName = "state"

// Storage is a persistent storage for the state of the discovery service.
type Storage struct {
	state  ClusterImportExporter
	logger *zap.Logger
	dbPath string
}

// ClusterImportExporter is an interface for exporting and importing cluster state.
type ClusterImportExporter interface {
	ExportClusters() []*state.ClusterExport
	ImportClusters(clusters []*state.ClusterExport)
}

// New creates a new instance of Storage.
func New(dbPath string, state ClusterImportExporter, logger *zap.Logger) *Storage {
	return &Storage{
		dbPath: dbPath,
		state:  state,
		logger: logger,
	}
}

// SaveAll saves all clusters' states into the persistent storage.
func (s *Storage) SaveAll() error {
	db, err := bolt.Open(s.dbPath, 0o600, nil)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			s.logger.Error("failed to close db", zap.Error(closeErr))
		}
	}()

	clusters := s.state.ExportClusters()

	if err = db.Update(func(tx *bolt.Tx) error {
		return s.update(tx, clusters)
	}); err != nil {
		return fmt.Errorf("failed to update db: %w", err)
	}

	s.logger.Info("saved all clusters into the persistent storage", zap.Int("count", len(clusters)))

	return nil
}

// LoadAll loads all clusters' states from the persistent storage.
func (s *Storage) LoadAll() error {
	db, err := bolt.Open(s.dbPath, 0o600, nil)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			s.logger.Error("failed to close db", zap.Error(closeErr))
		}
	}()

	var clusters []*state.ClusterExport

	if err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(_, val []byte) error {
			var cluster state.ClusterExport

			if unmarshalErr := json.Unmarshal(val, &cluster); unmarshalErr != nil {
				return fmt.Errorf("failed to unmarshal cluster: %w", unmarshalErr)
			}

			clusters = append(clusters, &cluster)

			return nil
		})
	}); err != nil {
		return fmt.Errorf("failed to load clusters: %w", err)
	}

	s.state.ImportClusters(clusters)

	s.logger.Info("loaded all clusters from the persistent storage", zap.Int("count", len(clusters)))

	return nil
}

func (s *Storage) update(tx *bolt.Tx, clusters []*state.ClusterExport) error {
	err := tx.DeleteBucket([]byte(bucketName))
	if err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	bucket, err := tx.CreateBucket([]byte(bucketName))
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	for _, cluster := range clusters {
		clusterJSON, marshalErr := json.Marshal(cluster)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal cluster: %w", marshalErr)
		}

		if err = bucket.Put([]byte(cluster.ID), clusterJSON); err != nil {
			return fmt.Errorf("failed to put cluster: %w", err)
		}
	}

	return nil
}

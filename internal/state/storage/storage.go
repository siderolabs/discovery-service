// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package storage implements persistent storage for the state of the discovery service.
package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	prom "github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	storagepb "github.com/siderolabs/discovery-service/api/storage"
)

const (
	labelOperation = "operation"
	labelStatus    = "status"

	operationSave = "save"
	operationLoad = "load"

	statusSuccess = "success"
	statusError   = "error"
)

// Storage is a persistent storage for the state of the discovery service.
type Storage struct {
	state  Snapshotter
	logger *zap.Logger

	operationsMetric              *prom.CounterVec
	lastSnapshotSizeMetric        *prom.GaugeVec
	lastOperationClustersMetric   *prom.GaugeVec
	lastOperationAffiliatesMetric *prom.GaugeVec
	lastOperationEndpointsMetric  *prom.GaugeVec
	lastOperationDurationMetric   *prom.GaugeVec

	path string
}

// Describe implements prometheus.Collector interface.
func (storage *Storage) Describe(descs chan<- *prom.Desc) {
	prom.DescribeByCollect(storage, descs)
}

// Collect implements prometheus.Collector interface.
func (storage *Storage) Collect(metrics chan<- prom.Metric) {
	storage.operationsMetric.Collect(metrics)
	storage.lastSnapshotSizeMetric.Collect(metrics)
	storage.lastOperationClustersMetric.Collect(metrics)
	storage.lastOperationAffiliatesMetric.Collect(metrics)
	storage.lastOperationEndpointsMetric.Collect(metrics)
	storage.lastOperationDurationMetric.Collect(metrics)
}

// Snapshotter is an interface for exporting and importing cluster state.
type Snapshotter interface {
	// ExportClusterSnapshots exports cluster snapshots to the given function.
	ExportClusterSnapshots(f func(*storagepb.ClusterSnapshot) error) error

	// ImportClusterSnapshots imports cluster snapshots from the given function.
	ImportClusterSnapshots(f func() (*storagepb.ClusterSnapshot, bool, error)) error
}

// New creates a new instance of Storage.
func New(path string, state Snapshotter, logger *zap.Logger) *Storage {
	return &Storage{
		state:  state,
		logger: logger.With(zap.String("component", "storage"), zap.String("path", path)),
		path:   path,

		operationsMetric: prom.NewCounterVec(prom.CounterOpts{
			Name: "discovery_storage_operations_total",
			Help: "The total number of storage operations.",
		}, []string{labelOperation, labelStatus}),
		lastSnapshotSizeMetric: prom.NewGaugeVec(prom.GaugeOpts{
			Name: "discovery_storage_last_snapshot_size_bytes",
			Help: "The size of the last processed snapshot in bytes.",
		}, []string{labelOperation}),
		lastOperationClustersMetric: prom.NewGaugeVec(prom.GaugeOpts{
			Name: "discovery_storage_last_operation_clusters",
			Help: "The number of clusters in the snapshot of the last operation.",
		}, []string{labelOperation}),
		lastOperationAffiliatesMetric: prom.NewGaugeVec(prom.GaugeOpts{
			Name: "discovery_storage_last_operation_affiliates",
			Help: "The number of affiliates in the snapshot of the last operation.",
		}, []string{labelOperation}),
		lastOperationEndpointsMetric: prom.NewGaugeVec(prom.GaugeOpts{
			Name: "discovery_storage_last_operation_endpoints",
			Help: "The number of endpoints in the snapshot of the last operation.",
		}, []string{labelOperation}),
		lastOperationDurationMetric: prom.NewGaugeVec(prom.GaugeOpts{
			Name: "discovery_storage_last_operation_duration_seconds",
			Help: "The duration of the last operation in seconds.",
		}, []string{labelOperation}),
	}
}

// Start starts the storage loop that periodically saves the state.
func (storage *Storage) Start(ctx context.Context, clock clockwork.Clock, interval time.Duration) error {
	storage.logger.Info("start storage loop", zap.Duration("interval", interval))

	ticker := clock.NewTicker(interval)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			storage.logger.Info("received shutdown signal")

			if err := storage.Save(); err != nil {
				return fmt.Errorf("failed to save state on shutdown: %w", err)
			}

			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}

			return ctx.Err()
		case <-ticker.Chan():
			if err := storage.Save(); err != nil {
				storage.logger.Error("failed to save state", zap.Error(err))
			}
		}
	}
}

// Save saves all clusters' states into the persistent storage.
func (storage *Storage) Save() (err error) {
	start := time.Now()

	defer func() {
		if err != nil {
			storage.operationsMetric.WithLabelValues(operationSave, statusError).Inc()
		}
	}()

	if err = os.MkdirAll(filepath.Dir(storage.path), 0o755); err != nil {
		return fmt.Errorf("failed to create directory path: %w", err)
	}

	tmpFile, err := getTempFile(storage.path)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	defer func() {
		tmpFile.Close()           //nolint:errcheck
		os.Remove(tmpFile.Name()) //nolint:errcheck
	}()

	stats, err := storage.Export(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	if err = commitTempFile(tmpFile, storage.path); err != nil {
		return fmt.Errorf("failed to commit temporary file: %w", err)
	}

	duration := time.Since(start)

	storage.logger.Info("state saved", zap.Int("clusters", stats.NumClusters), zap.Int("affiliates", stats.NumAffiliates),
		zap.Int("endpoints", stats.NumEndpoints), zap.Duration("duration", duration))

	storage.operationsMetric.WithLabelValues(operationSave, statusSuccess).Inc()
	storage.lastSnapshotSizeMetric.WithLabelValues(operationSave).Set(float64(stats.Size))
	storage.lastOperationClustersMetric.WithLabelValues(operationSave).Set(float64(stats.NumClusters))
	storage.lastOperationAffiliatesMetric.WithLabelValues(operationSave).Set(float64(stats.NumAffiliates))
	storage.lastOperationEndpointsMetric.WithLabelValues(operationSave).Set(float64(stats.NumEndpoints))
	storage.lastOperationDurationMetric.WithLabelValues(operationSave).Set(duration.Seconds())

	return nil
}

// Load loads all clusters' states from the persistent storage.
func (storage *Storage) Load() (err error) {
	defer func() {
		if err != nil {
			storage.operationsMetric.WithLabelValues(operationLoad, statusError).Inc()
		}
	}()

	start := time.Now()

	// open file for reading
	file, err := os.Open(storage.path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	defer file.Close() //nolint:errcheck

	stats, err := storage.Import(file)
	if err != nil {
		return fmt.Errorf("failed to read snapshot: %w", err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	duration := time.Since(start)

	storage.logger.Info("state loaded", zap.Int("clusters", stats.NumClusters), zap.Int("affiliates", stats.NumAffiliates),
		zap.Int("endpoints", stats.NumEndpoints), zap.Duration("duration", duration))

	storage.operationsMetric.WithLabelValues(operationLoad, statusSuccess).Inc()
	storage.lastSnapshotSizeMetric.WithLabelValues(operationLoad).Set(float64(stats.Size))
	storage.lastOperationClustersMetric.WithLabelValues(operationLoad).Set(float64(stats.NumClusters))
	storage.lastOperationAffiliatesMetric.WithLabelValues(operationLoad).Set(float64(stats.NumAffiliates))
	storage.lastOperationEndpointsMetric.WithLabelValues(operationLoad).Set(float64(stats.NumEndpoints))
	storage.lastOperationDurationMetric.WithLabelValues(operationLoad).Set(duration.Seconds())

	return nil
}

// Import imports all clusters' states from the given reader.
//
// When importing, we avoid unmarshalling to the storagepb.StateSnapshot type directly, as it causes an allocation of all the cluster snapshots at once.
// Instead, we process clusters in a streaming manner, unmarshaling them one by one and importing them into the state.
func (storage *Storage) Import(reader io.Reader) (SnapshotStats, error) {
	size := 0
	numClusters := 0
	numAffiliates := 0
	numEndpoints := 0

	buffer := make([]byte, 256)
	bufferedReader := bufio.NewReader(reader)

	// unmarshal the clusters in a streaming manner and import them into the state
	if err := storage.state.ImportClusterSnapshots(func() (*storagepb.ClusterSnapshot, bool, error) {
		headerSize, clusterSize, err := decodeClusterSnapshotHeader(bufferedReader)
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				return nil, false, nil
			}

			return nil, false, fmt.Errorf("failed to decode cluster header: %w", err)
		}

		if clusterSize > cap(buffer) {
			buffer = slices.Grow(buffer, clusterSize-cap(buffer))
		}

		buffer = buffer[:clusterSize]

		if _, err = io.ReadFull(bufferedReader, buffer); err != nil {
			return nil, false, fmt.Errorf("failed to read bytes: %w", err)
		}

		clusterSnapshot, err := decodeClusterSnapshot(buffer)
		if err != nil {
			return nil, false, fmt.Errorf("failed to decode cluster: %w", err)
		}

		buffer = buffer[:0]

		// update stats
		size += headerSize + clusterSize
		numClusters++
		numAffiliates += len(clusterSnapshot.Affiliates)

		for _, affiliate := range clusterSnapshot.Affiliates {
			numEndpoints += len(affiliate.Endpoints)
		}

		return clusterSnapshot, true, nil
	}); err != nil {
		return SnapshotStats{}, fmt.Errorf("failed to import clusters: %w", err)
	}

	return SnapshotStats{
		Size:          size,
		NumClusters:   numClusters,
		NumAffiliates: numAffiliates,
		NumEndpoints:  numEndpoints,
	}, nil
}

// Export exports all clusters' states into the given writer.
//
// When exporting, we avoid marshaling to the storagepb.StateSnapshot type directly, as it causes an allocation of all the cluster snapshots at once.
// Instead, we process clusters in a streaming manner, marshaling them one by one and exporting them into the writer.
func (storage *Storage) Export(writer io.Writer) (SnapshotStats, error) {
	numClusters := 0
	numAffiliates := 0
	numEndpoints := 0
	size := 0

	var buffer []byte

	bufferedWriter := bufio.NewWriter(writer)

	// marshal the clusters in a streaming manner and export them into the writer
	if err := storage.state.ExportClusterSnapshots(func(snapshot *storagepb.ClusterSnapshot) error {
		var err error

		buffer, err = encodeClusterSnapshot(buffer, snapshot)
		if err != nil {
			return fmt.Errorf("failed to encode cluster: %w", err)
		}

		written, err := bufferedWriter.Write(buffer)
		if err != nil {
			return fmt.Errorf("failed to write cluster: %w", err)
		}

		// prepare the buffer for the next iteration - reset it
		buffer = buffer[:0]

		// update stats
		size += written
		numClusters++
		numAffiliates += len(snapshot.Affiliates)

		for _, affiliate := range snapshot.Affiliates {
			numEndpoints += len(affiliate.Endpoints)
		}

		return nil
	}); err != nil {
		return SnapshotStats{}, fmt.Errorf("failed to snapshot clusters: %w", err)
	}

	if err := bufferedWriter.Flush(); err != nil {
		return SnapshotStats{}, fmt.Errorf("failed to flush writer: %w", err)
	}

	return SnapshotStats{
		Size:          size,
		NumClusters:   numClusters,
		NumAffiliates: numAffiliates,
		NumEndpoints:  numEndpoints,
	}, nil
}

func getTempFile(dst string) (*os.File, error) {
	tmpFile, err := os.OpenFile(dst+".tmp", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return tmpFile, nil
}

// commitTempFile commits the temporary file to the destination and removes it.
func commitTempFile(tmpFile *os.File, dst string) error {
	renamed := false
	closer := sync.OnceValue(tmpFile.Close)

	defer func() {
		closer() //nolint:errcheck

		if !renamed {
			os.Remove(tmpFile.Name()) //nolint:errcheck
		}
	}()

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync data: %w", err)
	}

	if err := closer(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), dst); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	renamed = true

	return nil
}

// SnapshotStats contains statistics about a snapshot.
type SnapshotStats struct {
	Size          int
	NumClusters   int
	NumAffiliates int
	NumEndpoints  int
}

// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/types/known/timestamppb"

	storagepb "github.com/siderolabs/discovery-service/api/storage"
	"github.com/siderolabs/discovery-service/internal/state/storage"
	"github.com/siderolabs/discovery-service/pkg/limits"
)

func TestExport(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct { //nolint:govet
		name     string
		snapshot *storagepb.StateSnapshot
	}{
		{
			"empty state",
			&storagepb.StateSnapshot{},
		},
		{
			"small state",
			&storagepb.StateSnapshot{Clusters: []*storagepb.ClusterSnapshot{{Id: "a"}, {Id: "b"}}},
		},
		{
			"large state",
			buildTestSnapshot(100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snapshot := buildTestSnapshot(10)
			tempDir := t.TempDir()
			path := filepath.Join(tempDir, "test.binpb")
			state := &mockSnapshotter{data: snapshot}
			logger := zaptest.NewLogger(t)

			stateStorage := storage.New(path, state, logger)

			var buffer bytes.Buffer

			exportStats, err := stateStorage.Export(&buffer)
			require.NoError(t, err)

			assert.Equal(t, statsForSnapshot(snapshot), exportStats)

			expected, err := snapshot.MarshalVT()
			require.NoError(t, err)

			require.Equal(t, expected, buffer.Bytes())
		})
	}
}

func TestImport(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct { //nolint:govet
		name     string
		snapshot *storagepb.StateSnapshot
	}{
		{
			"empty state",
			&storagepb.StateSnapshot{},
		},
		{
			"small state",
			&storagepb.StateSnapshot{Clusters: []*storagepb.ClusterSnapshot{{Id: "a"}, {Id: "b"}}},
		},
		{
			"large state",
			buildTestSnapshot(100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "test.binpb")
			state := &mockSnapshotter{data: tc.snapshot}
			logger := zaptest.NewLogger(t)

			stateStorage := storage.New(path, state, logger)

			data, err := tc.snapshot.MarshalVT()
			require.NoError(t, err)

			importStats, err := stateStorage.Import(bytes.NewReader(data))
			require.NoError(t, err)

			require.Equal(t, statsForSnapshot(tc.snapshot), importStats)

			loads := state.getLoads()

			require.Len(t, loads, 1)
			require.True(t, loads[0].EqualVT(tc.snapshot))
		})
	}
}

func TestImportMaxSize(t *testing.T) {
	t.Parallel()

	cluster := buildMaxSizeCluster()
	stateSnapshot := &storagepb.StateSnapshot{Clusters: []*storagepb.ClusterSnapshot{cluster}}
	path := filepath.Join(t.TempDir(), "test.binpb")
	state := &mockSnapshotter{data: stateSnapshot}
	logger := zaptest.NewLogger(t)

	stateStorage := storage.New(path, state, logger)

	clusterData, err := cluster.MarshalVT()
	require.NoError(t, err)

	require.Equal(t, len(clusterData), storage.MaxClusterSize)

	data, err := stateSnapshot.MarshalVT()
	require.NoError(t, err)

	t.Logf("max cluster marshaled size: %d", len(data))

	_, err = stateStorage.Import(bytes.NewReader(data))
	require.NoError(t, err)

	// add one more affiliate to trigger an overflow
	cluster.Affiliates = append(cluster.Affiliates, &storagepb.AffiliateSnapshot{
		Id: "overflow",
	})

	data, err = stateSnapshot.MarshalVT()
	require.NoError(t, err)

	_, err = stateStorage.Import(bytes.NewReader(data))
	require.ErrorIs(t, err, storage.ErrClusterSnapshotTooLarge)
}

func TestStorage(t *testing.T) {
	t.Parallel()

	snapshot := buildTestSnapshot(10)
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.binpb")
	state := &mockSnapshotter{data: snapshot}
	logger := zaptest.NewLogger(t)

	stateStorage := storage.New(path, state, logger)

	// test save

	require.NoError(t, stateStorage.Save())

	expectedData, err := snapshot.MarshalVT()
	require.NoError(t, err)

	actualData, err := os.ReadFile(path)
	require.NoError(t, err)

	require.Equal(t, expectedData, actualData)

	// test load

	require.NoError(t, stateStorage.Load())
	require.Len(t, state.getLoads(), 1)
	require.True(t, snapshot.EqualVT(state.getLoads()[0]))

	// modify, save & load again to assert that the file content gets overwritten

	snapshot.Clusters[1].Affiliates[0].Data = []byte("new aff1 data")

	require.NoError(t, stateStorage.Save())
	require.NoError(t, stateStorage.Load())
	require.Len(t, state.getLoads(), 2)
	require.True(t, snapshot.EqualVT(state.getLoads()[1]))
}

func TestSchedule(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	snapshot := buildTestSnapshot(10)
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.binpb")
	state := &mockSnapshotter{data: snapshot}
	logger := zaptest.NewLogger(t)

	stateStorage := storage.New(path, state, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	// start the periodic storage and wait for it to block on the timer

	errCh := make(chan error)

	go func() {
		errCh <- stateStorage.Start(ctx, clock, 10*time.Minute)
	}()

	require.NoError(t, clock.BlockUntilContext(ctx, 1))

	// advance time to trigger the first snapshot and assert it

	clock.Advance(13 * time.Minute)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Equal(collect, 1, state.getSnapshots())
	}, 2*time.Second, 100*time.Millisecond)

	// advance time to trigger the second snapshot and assert it

	clock.Advance(10 * time.Minute)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Equal(collect, 2, state.getSnapshots())
	}, 2*time.Second, 100*time.Millisecond)

	// cancel the context to stop the storage loop and wait for it to exit
	cancel()

	require.NoError(t, <-errCh)

	// assert that the state was saved on shutdown

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Equal(collect, 3, state.getSnapshots())
	}, 2*time.Second, 100*time.Millisecond)
}

type mockSnapshotter struct {
	data  *storagepb.StateSnapshot
	loads []*storagepb.StateSnapshot

	snapshots int

	lock sync.Mutex
}

func (m *mockSnapshotter) getSnapshots() int {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.snapshots
}

func (m *mockSnapshotter) getLoads() []*storagepb.StateSnapshot {
	m.lock.Lock()
	defer m.lock.Unlock()

	return append([]*storagepb.StateSnapshot(nil), m.loads...)
}

// ExportClusterSnapshots implements storage.Snapshotter interface.
func (m *mockSnapshotter) ExportClusterSnapshots(f func(snapshot *storagepb.ClusterSnapshot) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.snapshots++

	for _, cluster := range m.data.Clusters {
		if err := f(cluster); err != nil {
			return err
		}
	}

	return nil
}

// ImportClusterSnapshots implements storage.Snapshotter interface.
func (m *mockSnapshotter) ImportClusterSnapshots(f func() (*storagepb.ClusterSnapshot, bool, error)) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	var clusters []*storagepb.ClusterSnapshot

	for {
		cluster, ok, err := f()
		if err != nil {
			return err
		}

		if !ok {
			break
		}

		clusters = append(clusters, cluster)
	}

	m.loads = append(m.loads, &storagepb.StateSnapshot{Clusters: clusters})

	return nil
}

func statsForSnapshot(snapshot *storagepb.StateSnapshot) storage.SnapshotStats {
	numAffiliates := 0
	numEndpoints := 0

	for _, cluster := range snapshot.Clusters {
		numAffiliates += len(cluster.Affiliates)

		for _, affiliate := range cluster.Affiliates {
			numEndpoints += len(affiliate.Endpoints)
		}
	}

	return storage.SnapshotStats{
		NumClusters:   len(snapshot.Clusters),
		NumAffiliates: numAffiliates,
		NumEndpoints:  numEndpoints,
		Size:          snapshot.SizeVT(),
	}
}

func buildTestSnapshot(numClusters int) *storagepb.StateSnapshot {
	clusters := make([]*storagepb.ClusterSnapshot, 0, numClusters)

	for i := range numClusters {
		affiliates := make([]*storagepb.AffiliateSnapshot, 0, 5)

		for j := range 5 {
			affiliates = append(affiliates, &storagepb.AffiliateSnapshot{
				Id:         fmt.Sprintf("aff%d", j),
				Expiration: timestamppb.New(time.Now().Add(time.Hour)),
				Data:       []byte(fmt.Sprintf("aff%d data", j)),
			})
		}

		if i%2 == 0 {
			affiliates[0].Endpoints = []*storagepb.EndpointSnapshot{
				{
					Expiration: timestamppb.New(time.Now().Add(time.Hour)),
					Data:       []byte(fmt.Sprintf("endpoint%d data", i)),
				},
			}
		}

		clusters = append(clusters, &storagepb.ClusterSnapshot{
			Id:         fmt.Sprintf("cluster%d", i),
			Affiliates: affiliates,
		})
	}

	return &storagepb.StateSnapshot{
		Clusters: clusters,
	}
}

// buildMaxSizeCluster creates a cluster snapshot with the maximum possible marshaled size within the limits of the discovery service.
func buildMaxSizeCluster() *storagepb.ClusterSnapshot {
	largestTTL := &timestamppb.Timestamp{
		Seconds: math.MinInt64,
		Nanos:   math.MinInt32,
	} // the timestamp with the maximum possible marshaled size

	affiliates := make([]*storagepb.AffiliateSnapshot, 0, limits.ClusterAffiliatesMax)

	for range limits.ClusterAffiliatesMax {
		endpoints := make([]*storagepb.EndpointSnapshot, 0, limits.AffiliateEndpointsMax)

		for range limits.AffiliateEndpointsMax {
			endpoints = append(endpoints, &storagepb.EndpointSnapshot{
				Expiration: largestTTL,
				Data:       bytes.Repeat([]byte("a"), limits.AffiliateDataMax),
			})
		}

		affiliates = append(affiliates, &storagepb.AffiliateSnapshot{
			Id:         strings.Repeat("a", limits.AffiliateIDMax),
			Expiration: largestTTL,
			Data:       bytes.Repeat([]byte("a"), limits.AffiliateDataMax),
			Endpoints:  endpoints,
		})
	}

	return &storagepb.ClusterSnapshot{
		Id:         strings.Repeat("c", limits.ClusterIDMax),
		Affiliates: affiliates,
	}
}

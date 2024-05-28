// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state

import (
	"fmt"
	"slices"

	"github.com/siderolabs/gen/xslices"
	"google.golang.org/protobuf/types/known/timestamppb"

	storagepb "github.com/siderolabs/discovery-service/api/storage"
)

// ExportClusterSnapshots exports all cluster snapshots and calls the provided function for each one.
//
// Implements storage.Snapshotter interface.
func (state *State) ExportClusterSnapshots(f func(snapshot *storagepb.ClusterSnapshot) error) error {
	var err error

	// reuse the same snapshot in each iteration
	clusterSnapshot := &storagepb.ClusterSnapshot{}

	state.clusters.Enumerate(func(_ string, cluster *Cluster) bool {
		snapshotCluster(cluster, clusterSnapshot)

		err = f(clusterSnapshot)

		return err == nil
	})

	return err
}

// ImportClusterSnapshots imports cluster snapshots by calling the provided function until it returns false.
//
// Implements storage.Snapshotter interface.
func (state *State) ImportClusterSnapshots(f func() (*storagepb.ClusterSnapshot, bool, error)) error {
	for {
		clusterSnapshot, ok, err := f()
		if err != nil {
			return err
		}

		if !ok {
			break
		}

		cluster := clusterFromSnapshot(clusterSnapshot)

		_, loaded := state.clusters.LoadOrStore(cluster.id, cluster)
		if loaded {
			return fmt.Errorf("cluster %q already exists", cluster.id)
		}
	}

	return nil
}

func snapshotCluster(cluster *Cluster, snapshot *storagepb.ClusterSnapshot) {
	cluster.affiliatesMu.Lock()
	defer cluster.affiliatesMu.Unlock()

	snapshot.Id = cluster.id

	// reuse the same slice, resize it as needed
	if len(cluster.affiliates) > cap(snapshot.Affiliates) {
		snapshot.Affiliates = slices.Grow(snapshot.Affiliates, len(cluster.affiliates)-len(snapshot.Affiliates))
	}

	snapshot.Affiliates = snapshot.Affiliates[:len(cluster.affiliates)]

	i := 0
	for _, affiliate := range cluster.affiliates {
		if snapshot.Affiliates[i] == nil {
			snapshot.Affiliates[i] = &storagepb.AffiliateSnapshot{}
		}

		snapshot.Affiliates[i].Id = affiliate.id

		if snapshot.Affiliates[i].Expiration == nil {
			snapshot.Affiliates[i].Expiration = &timestamppb.Timestamp{}
		}

		snapshot.Affiliates[i].Expiration.Seconds = affiliate.expiration.Unix()
		snapshot.Affiliates[i].Expiration.Nanos = int32(affiliate.expiration.Nanosecond())

		snapshot.Affiliates[i].Data = affiliate.data

		// reuse the same slice, resize it as needed
		if len(affiliate.endpoints) > cap(snapshot.Affiliates[i].Endpoints) {
			snapshot.Affiliates[i].Endpoints = slices.Grow(snapshot.Affiliates[i].Endpoints, len(affiliate.endpoints)-len(snapshot.Affiliates[i].Endpoints))
		}

		snapshot.Affiliates[i].Endpoints = snapshot.Affiliates[i].Endpoints[:len(affiliate.endpoints)]

		for j, endpoint := range affiliate.endpoints {
			if snapshot.Affiliates[i].Endpoints[j] == nil {
				snapshot.Affiliates[i].Endpoints[j] = &storagepb.EndpointSnapshot{}
			}

			snapshot.Affiliates[i].Endpoints[j].Data = endpoint.data

			if snapshot.Affiliates[i].Endpoints[j].Expiration == nil {
				snapshot.Affiliates[i].Endpoints[j].Expiration = &timestamppb.Timestamp{}
			}

			snapshot.Affiliates[i].Endpoints[j].Expiration.Seconds = endpoint.expiration.Unix()
			snapshot.Affiliates[i].Endpoints[j].Expiration.Nanos = int32(endpoint.expiration.Nanosecond())
		}

		i++
	}
}

func clusterFromSnapshot(snapshot *storagepb.ClusterSnapshot) *Cluster {
	return &Cluster{
		id:         snapshot.Id,
		affiliates: xslices.ToMap(snapshot.Affiliates, affiliateFromSnapshot),
	}
}

func affiliateFromSnapshot(snapshot *storagepb.AffiliateSnapshot) (string, *Affiliate) {
	return snapshot.Id, &Affiliate{
		id:         snapshot.Id,
		expiration: snapshot.Expiration.AsTime(),
		data:       snapshot.Data,
		endpoints:  xslices.Map(snapshot.Endpoints, endpointFromSnapshot),
	}
}

func endpointFromSnapshot(snapshot *storagepb.EndpointSnapshot) Endpoint {
	return Endpoint{
		data:       snapshot.Data,
		expiration: snapshot.Expiration.AsTime(),
	}
}

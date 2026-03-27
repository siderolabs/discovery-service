// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state

import (
	"fmt"
	"slices"
	"time"

	"github.com/siderolabs/gen/xslices"
	"google.golang.org/protobuf/types/known/durationpb"

	storagepb "github.com/siderolabs/discovery-service/api/storage"
)

// ExportClusterSnapshots exports all cluster snapshots and calls the provided function for each one.
//
// Implements storage.Snapshotter interface.
func (state *State) ExportClusterSnapshots(now time.Time, f func(snapshot *storagepb.ClusterSnapshot) error) error {
	// reuse the same snapshot in each iteration
	clusterSnapshot := &storagepb.ClusterSnapshot{}

	for _, cluster := range state.clusters.All() {
		snapshotCluster(cluster, clusterSnapshot, now)

		if err := f(clusterSnapshot); err != nil {
			return err
		}
	}

	return nil
}

// ImportClusterSnapshots imports cluster snapshots by calling the provided function until it returns false.
//
// Implements storage.Snapshotter interface.
func (state *State) ImportClusterSnapshots(now time.Time, f func() (*storagepb.ClusterSnapshot, bool, error)) error {
	for {
		clusterSnapshot, ok, err := f()
		if err != nil {
			return err
		}

		if !ok {
			break
		}

		cluster := clusterFromSnapshot(clusterSnapshot, now)

		_, loaded := state.clusters.LoadOrStore(cluster.id, cluster)
		if loaded {
			return fmt.Errorf("cluster %q already exists", cluster.id)
		}
	}

	return nil
}

func snapshotCluster(cluster *Cluster, snapshot *storagepb.ClusterSnapshot, now time.Time) {
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

		if snapshot.Affiliates[i].Ttl == nil {
			snapshot.Affiliates[i].Ttl = &durationpb.Duration{}
		}

		*snapshot.Affiliates[i].Ttl = *durationpb.New(affiliate.expiration.Sub(now).Truncate(time.Second))

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

			if snapshot.Affiliates[i].Endpoints[j].Ttl == nil {
				snapshot.Affiliates[i].Endpoints[j].Ttl = &durationpb.Duration{}
			}

			*snapshot.Affiliates[i].Endpoints[j].Ttl = *durationpb.New(endpoint.expiration.Sub(now).Truncate(time.Second))
		}

		i++
	}
}

func clusterFromSnapshot(snapshot *storagepb.ClusterSnapshot, now time.Time) *Cluster {
	return &Cluster{
		id:         snapshot.Id,
		affiliates: xslices.ToMap(snapshot.Affiliates, affiliateFromSnapshot(now)),
	}
}

func affiliateFromSnapshot(now time.Time) func(*storagepb.AffiliateSnapshot) (string, *Affiliate) {
	return func(snapshot *storagepb.AffiliateSnapshot) (string, *Affiliate) {
		var expiration time.Time

		if snapshot.Ttl != nil {
			expiration = now.Add(snapshot.Ttl.AsDuration())
		} else {
			expiration = snapshot.Expiration.AsTime()
		}

		return snapshot.Id, &Affiliate{
			id:         snapshot.Id,
			expiration: expiration,
			data:       snapshot.Data,
			endpoints:  xslices.Map(snapshot.Endpoints, endpointFromSnapshot(now)),
		}
	}
}

func endpointFromSnapshot(now time.Time) func(*storagepb.EndpointSnapshot) Endpoint {
	return func(snapshot *storagepb.EndpointSnapshot) Endpoint {
		var expiration time.Time

		if snapshot.Ttl != nil {
			expiration = now.Add(snapshot.Ttl.AsDuration())
		} else {
			expiration = snapshot.Expiration.AsTime()
		}

		return Endpoint{
			data:       snapshot.Data,
			expiration: expiration,
		}
	}
}

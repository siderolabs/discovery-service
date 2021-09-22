// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package state implements server state with clusters, affiliates, subscriptions, etc.
package state

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// State keeps the discovery service state.
type State struct {
	clusters sync.Map
}

// NewState create new instance of State.
func NewState() *State {
	return &State{}
}

// GetCluster returns (or creates) new cluster by ID.
func (state *State) GetCluster(id string) *Cluster {
	if v, ok := state.clusters.Load(id); ok {
		return v.(*Cluster)
	}

	v, _ := state.clusters.LoadOrStore(id, NewCluster(id))

	return v.(*Cluster)
}

// GarbageCollect recursively each cluster, and remove empty clusters.
func (state *State) GarbageCollect(now time.Time) (removedClusters int) {
	state.clusters.Range(func(key, value interface{}) bool {
		cluster := value.(*Cluster) //nolint:errcheck,forcetypeassert

		if cluster.GarbageCollect(now) {
			state.clusters.Delete(key)

			removedClusters++
		}

		return true
	})

	return removedClusters
}

// RunGC runs the garbage collection on interval.
func (state *State) RunGC(ctx context.Context, logger *zap.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		removedClusters := state.GarbageCollect(time.Now())

		if removedClusters > 0 {
			logger.Info("garbage collection run", zap.Int("removed_clusters", removedClusters))
		}

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

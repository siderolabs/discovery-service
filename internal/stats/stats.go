// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package stats provides a cache for the state counters, which are expensive to compute.
package stats

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/siderolabs/discovery-service/internal/state"
)

func Handler(state *state.State, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	statsHandler := &statsCache{state: state, ttl: time.Hour}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := statsHandler.StatsHandler(w, r); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"error": "Oops, try again"}`) //nolint:errcheck
			logger.Error("failed to return stats", zap.Error(err))
		}
	})

	return mux
}

type statsSnapshot struct {
	expiry time.Time
	data   []byte
}

// statsCache serves the state counters as JSON, recomputing at most once per TTL.
//
// stats() is an O(n) scan, so concurrent misses are collapsed into a single
// recompute via singleflight. The snapshot is swapped atomically, so reads are
// lock-free.
type statsCache struct {
	state *state.State
	cur   atomic.Pointer[statsSnapshot]
	group singleflight.Group
	ttl   time.Duration
}

func (c *statsCache) StatsHandler(w http.ResponseWriter, _ *http.Request) error {
	snap := c.cur.Load()

	if snap == nil || time.Now().After(snap.expiry) {
		v, err, _ := c.group.Do("stats", func() (any, error) {
			b, err := json.Marshal(c.state.Stats())
			if err != nil {
				return nil, fmt.Errorf("failed to marshal stats: %w", err)
			}

			s := &statsSnapshot{data: b, expiry: time.Now().Add(c.ttl)}
			c.cur.Store(s)

			return s, nil
		})
		if err != nil {
			return err
		}

		var ok bool

		snap, ok = v.(*statsSnapshot)
		if !ok {
			return fmt.Errorf("unexpected type %T", v)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(snap.data) //nolint:errcheck

	return nil
}

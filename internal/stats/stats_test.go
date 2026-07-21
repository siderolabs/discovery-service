// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package stats //nolint:testpackage // exercises unexported statsCache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/siderolabs/discovery-service/internal/state"
)

func TestStatsCache(t *testing.T) {
	st := state.NewState(zaptest.NewLogger(t))

	require.NoError(t, st.GetCluster("c1").WithAffiliate("a1", func(a *state.Affiliate) error {
		a.Update([]byte("d"), time.Now().Add(time.Hour))

		return nil
	}))

	get := func(c *statsCache) state.Stats {
		rec := httptest.NewRecorder()
		require.NoError(t, c.StatsHandler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stats", nil)))
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var s state.Stats
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))

		return s
	}

	// long TTL: second mutation must not be visible (served from cache).
	cached := &statsCache{state: st, ttl: time.Hour}
	require.Equal(t, 1, get(cached).Affiliates)

	require.NoError(t, st.GetCluster("c1").WithAffiliate("a2", func(a *state.Affiliate) error {
		a.Update([]byte("d"), time.Now().Add(time.Hour))

		return nil
	}))

	require.Equal(t, 1, get(cached).Affiliates, "stale value should be served within TTL")

	// zero TTL: always recomputed.
	uncached := &statsCache{state: st, ttl: 0}
	require.Equal(t, 2, get(uncached).Affiliates)

	require.NoError(t, st.GetCluster("c1").WithAffiliate("a3", func(a *state.Affiliate) error {
		a.Update([]byte("d"), time.Now().Add(time.Hour))

		return nil
	}))

	require.Equal(t, 3, get(uncached).Affiliates)
}

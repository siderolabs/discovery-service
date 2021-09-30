// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package state_test

import (
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/talos-systems/discovery-service/internal/state"
)

func checkMetrics(t *testing.T, c prom.Collector) {
	problems, err := promtestutil.CollectAndLint(c)
	require.NoError(t, err)
	require.Empty(t, problems)

	assert.NotZero(t, promtestutil.CollectAndCount(c), "collector should not be unchecked")
}

func TestState(t *testing.T) {
	now := time.Now()

	st := state.NewState()

	// Check metrics before and after the test
	// to ensure that collector does not switch from being unchecked to checked and invalid.
	checkMetrics(t, st)
	t.Cleanup(func() { checkMetrics(t, st) })

	removedClusters, removedAffiliates := st.GarbageCollect(now)
	assert.Equal(t, 0, removedClusters)
	assert.Equal(t, 0, removedAffiliates)

	st.GetCluster("id1")
	assert.NoError(t, st.GetCluster("id2").WithAffiliate("af1", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data1"), now.Add(time.Minute))

		return nil
	}))

	removedClusters, removedAffiliates = st.GarbageCollect(now)
	assert.Equal(t, 1, removedClusters)
	assert.Equal(t, 0, removedAffiliates)

	removedClusters, removedAffiliates = st.GarbageCollect(now.Add(2 * time.Minute))
	assert.Equal(t, 1, removedClusters)
	assert.Equal(t, 1, removedAffiliates)

	removedClusters, removedAffiliates = st.GarbageCollect(now)
	assert.Equal(t, 0, removedClusters)
	assert.Equal(t, 0, removedAffiliates)
}

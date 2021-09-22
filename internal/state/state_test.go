// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package state_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/talos-systems/discovery-service/internal/state"
)

func TestState(t *testing.T) {
	now := time.Now()

	st := state.NewState()

	deletedClusters := st.GarbageCollect(now)
	assert.Equal(t, 0, deletedClusters)

	st.GetCluster("id1")
	st.GetCluster("id2").WithAffiliate("af1", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data1"), now.Add(time.Minute))
	})

	deletedClusters = st.GarbageCollect(now)
	assert.Equal(t, 1, deletedClusters)

	deletedClusters = st.GarbageCollect(now.Add(2 * time.Minute))
	assert.Equal(t, 1, deletedClusters)

	deletedClusters = st.GarbageCollect(now)
	assert.Equal(t, 0, deletedClusters)
}

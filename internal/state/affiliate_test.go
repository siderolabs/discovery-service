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

func TestAffiliateMutations(t *testing.T) {
	now := time.Now()

	affiliate := state.NewAffiliate("id1")

	affiliate.ClearChanged()
	assert.False(t, affiliate.IsChanged())

	affiliate.Update([]byte("data"), now.Add(time.Minute))

	assert.Equal(t, &state.AffiliateExport{
		ID:        "id1",
		Data:      []byte("data"),
		Endpoints: [][]byte{},
	}, affiliate.Export())

	assert.True(t, affiliate.IsChanged())

	affiliate.ClearChanged()

	affiliate.Update([]byte("data1"), now.Add(time.Minute))

	assert.Equal(t, &state.AffiliateExport{
		ID:        "id1",
		Data:      []byte("data1"),
		Endpoints: [][]byte{},
	}, affiliate.Export())

	assert.True(t, affiliate.IsChanged())

	affiliate.ClearChanged()

	affiliate.MergeEndpoints([][]byte{
		[]byte("e1"),
		[]byte("e2"),
	}, now.Add(time.Minute))

	assert.Equal(t, &state.AffiliateExport{
		ID:        "id1",
		Data:      []byte("data1"),
		Endpoints: [][]byte{[]byte("e1"), []byte("e2")},
	}, affiliate.Export())

	assert.True(t, affiliate.IsChanged())
	affiliate.ClearChanged()

	affiliate.MergeEndpoints([][]byte{
		[]byte("e1"),
	}, now.Add(time.Minute))

	assert.False(t, affiliate.IsChanged())

	affiliate.MergeEndpoints([][]byte{
		[]byte("e1"),
		[]byte("e3"),
	}, now.Add(3*time.Minute))

	assert.Equal(t, &state.AffiliateExport{
		ID:        "id1",
		Data:      []byte("data1"),
		Endpoints: [][]byte{[]byte("e1"), []byte("e2"), []byte("e3")},
	}, affiliate.Export())

	assert.True(t, affiliate.IsChanged())

	remove, changed := affiliate.GarbageCollect(now)
	assert.False(t, remove)
	assert.False(t, changed)

	remove, changed = affiliate.GarbageCollect(now.Add(2 * time.Minute))
	assert.False(t, remove)
	assert.True(t, changed)

	assert.Equal(t, &state.AffiliateExport{
		ID:        "id1",
		Data:      []byte("data1"),
		Endpoints: [][]byte{[]byte("e1"), []byte("e3")},
	}, affiliate.Export())

	remove, changed = affiliate.GarbageCollect(now.Add(4 * time.Minute))
	assert.True(t, remove)
	assert.True(t, changed)
}

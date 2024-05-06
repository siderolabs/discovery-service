// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/discovery-service/internal/state"
	"github.com/siderolabs/discovery-service/pkg/limits"
)

func TestAffiliateMutations(t *testing.T) {
	now := time.Now()

	affiliate := state.NewAffiliate("id1")
	affiliate.ClearChanged()
	assert.False(t, affiliate.IsChanged())

	affiliate.Update([]byte("data"), now.Add(time.Minute))

	assert.Equal(t, &state.AffiliateExport{
		ID:      "id1",
		Data:    []byte("data"),
		Changed: true,
	}, resetAffiliateExpirations(affiliate.Export()))

	assert.True(t, affiliate.IsChanged())

	affiliate.ClearChanged()

	affiliate.Update([]byte("data1"), now.Add(time.Minute))

	assert.Equal(t, &state.AffiliateExport{
		ID:      "id1",
		Data:    []byte("data1"),
		Changed: true,
	}, resetAffiliateExpirations(affiliate.Export()))

	assert.True(t, affiliate.IsChanged())

	affiliate.ClearChanged()

	assert.NoError(t, affiliate.MergeEndpoints([][]byte{
		[]byte("e1"),
		[]byte("e2"),
	}, now.Add(time.Minute)))

	assert.Equal(t, &state.AffiliateExport{
		ID:   "id1",
		Data: []byte("data1"),
		Endpoints: []*state.EndpointExport{
			{Data: []byte("e1")},
			{Data: []byte("e2")},
		},
		Changed: true,
	}, resetAffiliateExpirations(affiliate.Export()))

	assert.True(t, affiliate.IsChanged())
	affiliate.ClearChanged()

	assert.NoError(t, affiliate.MergeEndpoints([][]byte{
		[]byte("e1"),
	}, now.Add(time.Minute)))

	assert.False(t, affiliate.IsChanged())

	assert.NoError(t, affiliate.MergeEndpoints([][]byte{
		[]byte("e1"),
		[]byte("e3"),
	}, now.Add(3*time.Minute)))

	assert.Equal(t, &state.AffiliateExport{
		ID:   "id1",
		Data: []byte("data1"),
		Endpoints: []*state.EndpointExport{
			{Data: []byte("e1")},
			{Data: []byte("e2")},
			{Data: []byte("e3")},
		},
		Changed: true,
	}, resetAffiliateExpirations(affiliate.Export()))

	assert.True(t, affiliate.IsChanged())

	remove, changed := affiliate.GarbageCollect(now)
	assert.False(t, remove)
	assert.False(t, changed)

	remove, changed = affiliate.GarbageCollect(now.Add(2 * time.Minute))
	assert.False(t, remove)
	assert.True(t, changed)

	assert.Equal(t, &state.AffiliateExport{
		ID:   "id1",
		Data: []byte("data1"),
		Endpoints: []*state.EndpointExport{
			{Data: []byte("e1")},
			{Data: []byte("e3")},
		},
		Changed: true,
	}, resetAffiliateExpirations(affiliate.Export()))

	remove, changed = affiliate.GarbageCollect(now.Add(4 * time.Minute))
	assert.True(t, remove)
	assert.True(t, changed)
}

func TestAffiliateTooManyEndpoints(t *testing.T) {
	now := time.Now()

	affiliate := state.NewAffiliate("id1")

	for i := range limits.AffiliateEndpointsMax {
		assert.NoError(t, affiliate.MergeEndpoints([][]byte{[]byte(fmt.Sprintf("endpoint%d", i))}, now))
	}

	err := affiliate.MergeEndpoints([][]byte{[]byte("endpoint")}, now)
	require.Error(t, err)

	assert.ErrorIs(t, err, state.ErrTooManyEndpoints)
}

// resetAffiliatesExpirations resets the expiration fields of the affiliate exports and their endpoints.
//
// This is used to assert the exports while ignoring the expiration fields.
func resetAffiliateExpirations(export *state.AffiliateExport) *state.AffiliateExport {
	if export == nil {
		return nil
	}

	export.Expiration = time.Time{}

	for _, endpoint := range export.Endpoints {
		endpoint.Expiration = time.Time{}
	}

	return export
}

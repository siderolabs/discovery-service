// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/talos-systems/discovery-service/internal/state"
	"github.com/talos-systems/discovery-service/pkg/limits"
)

func TestClusterMutations(t *testing.T) {
	now := time.Now()

	cluster := state.NewCluster("cluster1")

	removedAffiliates, empty := cluster.GarbageCollect(now)
	assert.Zero(t, removedAffiliates)
	assert.True(t, empty)

	assert.Len(t, cluster.List(), 0)

	assert.NoError(t, cluster.WithAffiliate("af1", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data"), now.Add(time.Minute))

		return nil
	}))

	assert.Len(t, cluster.List(), 1)

	updates := make(chan *state.Notification, 1)

	snapshot, subscription := cluster.Subscribe(updates)
	defer subscription.Close()

	assert.Len(t, snapshot, 1)

	assert.NoError(t, cluster.WithAffiliate("af1", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data1"), now.Add(time.Minute))

		return nil
	}))

	assert.Len(t, cluster.List(), 1)
	assert.Equal(t, []*state.AffiliateExport{
		{
			ID:        "af1",
			Data:      []byte("data1"),
			Endpoints: [][]byte{},
		},
	}, cluster.List())

	select {
	case notification := <-updates:
		assert.Equal(t, "af1", notification.AffiliateID)
		assert.Equal(t, &state.AffiliateExport{
			ID:        "af1",
			Data:      []byte("data1"),
			Endpoints: [][]byte{},
		}, notification.Affiliate)
	case <-time.After(time.Second):
		assert.Fail(t, "no notification")
	}

	assert.NoError(t, cluster.WithAffiliate("af2", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data2"), now.Add(time.Minute))

		return nil
	}))

	assert.Len(t, cluster.List(), 2)

	list := cluster.List()
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	assert.Equal(t, []*state.AffiliateExport{
		{
			ID:        "af1",
			Data:      []byte("data1"),
			Endpoints: [][]byte{},
		},
		{
			ID:        "af2",
			Data:      []byte("data2"),
			Endpoints: [][]byte{},
		},
	}, list)

	select {
	case notification := <-updates:
		assert.Equal(t, "af2", notification.AffiliateID)
		assert.Equal(t, &state.AffiliateExport{
			ID:        "af2",
			Data:      []byte("data2"),
			Endpoints: [][]byte{},
		}, notification.Affiliate)
	case <-time.After(time.Second):
		assert.Fail(t, "no notification")
	}

	cluster.DeleteAffiliate("af1")

	assert.Len(t, cluster.List(), 1)
	assert.Equal(t, []*state.AffiliateExport{
		{
			ID:        "af2",
			Data:      []byte("data2"),
			Endpoints: [][]byte{},
		},
	}, cluster.List())

	select {
	case notification := <-updates:
		assert.Equal(t, "af1", notification.AffiliateID)
		assert.Nil(t, notification.Affiliate)
	case <-time.After(time.Second):
		assert.Fail(t, "no notification")
	}

	removedAffiliates, empty = cluster.GarbageCollect(now)
	assert.Zero(t, removedAffiliates)
	assert.False(t, empty)

	removedAffiliates, empty = cluster.GarbageCollect(now.Add(2 * time.Minute))
	assert.Equal(t, 1, removedAffiliates)
	assert.False(t, empty)

	select {
	case notification := <-updates:
		assert.Equal(t, "af2", notification.AffiliateID)
		assert.Nil(t, notification.Affiliate)
	case <-time.After(time.Second):
		assert.Fail(t, "no notification")
	}

	select {
	case err := <-subscription.ErrCh():
		assert.NoError(t, err)
	default:
	}

	subscription.Close()

	removedAffiliates, empty = cluster.GarbageCollect(now.Add(2 * time.Minute))
	assert.Equal(t, 0, removedAffiliates)
	assert.True(t, empty)
}

func TestClusterSubscriptions(t *testing.T) {
	t.Parallel()

	now := time.Now()

	cluster := state.NewCluster("cluster2")

	// create live and dead subscribers
	liveSubscribers := make([]*state.Subscription, 5)
	deadSubscribers := make([]*state.Subscription, 2)

	channels := make([]chan *state.Notification, len(liveSubscribers))

	for i := range liveSubscribers {
		var snapshot []*state.AffiliateExport

		channels[i] = make(chan *state.Notification, 16)

		snapshot, liveSubscribers[i] = cluster.Subscribe(channels[i])
		assert.Empty(t, snapshot)

		defer liveSubscribers[i].Close()
	}

	for i := range deadSubscribers {
		var snapshot []*state.AffiliateExport

		snapshot, deadSubscribers[i] = cluster.Subscribe(make(chan<- *state.Notification, 1))
		assert.Empty(t, snapshot)
	}

	assert.NoError(t, cluster.WithAffiliate("af1", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data1"), now.Add(time.Minute))

		return nil
	}))

	assert.NoError(t, cluster.WithAffiliate("af2", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data2"), now.Add(time.Minute))

		return nil
	}))

	assert.NoError(t, cluster.WithAffiliate("af2", func(affiliate *state.Affiliate) error {
		affiliate.Update([]byte("data2_1"), now.Add(time.Minute))

		return nil
	}))

	cluster.DeleteAffiliate("af2")

	// dead subscribers should have errored out
	for i := range deadSubscribers {
		select {
		case err := <-deadSubscribers[i].ErrCh():
			assert.Error(t, err)
		default:
			assert.Fail(t, "error is expected")
		}
	}

	// live subscribers should have no error
	for i := range liveSubscribers {
		select {
		case <-liveSubscribers[i].ErrCh():
			assert.Fail(t, "error is not expected")
		default:
		}
	}

	assertNotification := func(ch <-chan *state.Notification, expected *state.Notification) {
		select {
		case notification := <-ch:
			assert.Equal(t, expected, notification)
		default:
			assert.Fail(t, "no message")
		}
	}

	for _, ch := range channels {
		assertNotification(ch, &state.Notification{
			AffiliateID: "af1",
			Affiliate: &state.AffiliateExport{
				ID:        "af1",
				Data:      []byte("data1"),
				Endpoints: [][]byte{},
			},
		})

		assertNotification(ch, &state.Notification{
			AffiliateID: "af2",
			Affiliate: &state.AffiliateExport{
				ID:        "af2",
				Data:      []byte("data2"),
				Endpoints: [][]byte{},
			},
		})

		assertNotification(ch, &state.Notification{
			AffiliateID: "af2",
			Affiliate: &state.AffiliateExport{
				ID:        "af2",
				Data:      []byte("data2_1"),
				Endpoints: [][]byte{},
			},
		})

		assertNotification(ch, &state.Notification{
			AffiliateID: "af2",
		})
	}
}

func TestClusterTooManyAffiliates(t *testing.T) {
	t.Parallel()

	cluster := state.NewCluster("cluster3")

	for i := 0; i < limits.ClusterAffiliatesMax; i++ {
		assert.NoError(t, cluster.WithAffiliate(fmt.Sprintf("af%d", i), func(affiliate *state.Affiliate) error {
			return nil
		}))
	}

	err := cluster.WithAffiliate("af", func(affiliate *state.Affiliate) error {
		return nil
	})
	require.Error(t, err)
	require.ErrorIs(t, err, state.ErrTooManyAffiliates)
}

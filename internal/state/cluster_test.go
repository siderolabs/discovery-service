// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package state_test

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/talos-systems/discovery-service/internal/state"
)

func TestClusterMutations(t *testing.T) {
	now := time.Now()

	cluster := state.NewCluster("cluster1")

	remove := cluster.GarbageCollect(now)
	assert.True(t, remove)

	assert.Len(t, cluster.List(), 0)

	cluster.WithAffiliate("af1", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data"), now.Add(time.Minute))
	})

	assert.Len(t, cluster.List(), 1)

	updates := make(chan *state.Notification, 1)

	snapshot, subscription := cluster.Subscribe(updates)
	defer subscription.Close()

	assert.Len(t, snapshot, 1)

	cluster.WithAffiliate("af1", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data1"), now.Add(time.Minute))
	})

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

	cluster.WithAffiliate("af2", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data2"), now.Add(time.Minute))
	})

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

	empty := cluster.GarbageCollect(now)
	assert.False(t, empty)

	empty = cluster.GarbageCollect(now.Add(2 * time.Minute))
	assert.True(t, empty)

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

	cluster.WithAffiliate("af1", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data1"), now.Add(time.Minute))
	})

	cluster.WithAffiliate("af2", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data2"), now.Add(time.Minute))
	})

	cluster.WithAffiliate("af2", func(affiliate *state.Affiliate) {
		affiliate.Update([]byte("data2_1"), now.Add(time.Minute))
	})

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

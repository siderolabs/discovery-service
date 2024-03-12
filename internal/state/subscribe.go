// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state

import (
	"fmt"
)

// Notification about affiliate update.
//
// Affiliate is nil, then affiliate was deleted.
type Notification struct {
	Affiliate   *AffiliateExport
	AffiliateID string
}

// Subscription is a handle returned to the subscriber.
type Subscription struct {
	cluster *Cluster

	errCh chan error
	ch    chan<- *Notification
}

// ErrCh returns error channel, whenever there's error on the channel, subscription is invalid.
func (subscription *Subscription) ErrCh() <-chan error {
	return subscription.errCh
}

// Close subscription (unsubscribe).
func (subscription *Subscription) Close() {
	subscription.cluster.unsubscribe(subscription)
}

func (subscription *Subscription) notify(notification *Notification) {
	select {
	case subscription.ch <- notification:
		return
	default:
	}

	subscription.errCh <- fmt.Errorf("lost update")

	subscription.Close()
}

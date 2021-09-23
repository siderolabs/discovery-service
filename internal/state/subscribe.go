// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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

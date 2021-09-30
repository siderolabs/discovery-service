// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package state

import (
	"bytes"
	"time"

	"github.com/talos-systems/discovery-service/pkg/limits"
)

// Affiliate represents cluster affiliate state.
type Affiliate struct {
	id         string
	expiration time.Time
	data       []byte
	endpoints  []Endpoint

	changed bool
}

// Endpoint is a combination of endpoint itself and its TTL.
type Endpoint struct {
	expiration time.Time
	data       []byte
}

// NewAffiliate constructs new (empty) Affiliate.
func NewAffiliate(id string) *Affiliate {
	return &Affiliate{
		id: id,
	}
}

// ClearChanged clears the changed flag.
func (affiliate *Affiliate) ClearChanged() {
	affiliate.changed = false
}

// IsChanged returns changed flag.
func (affiliate *Affiliate) IsChanged() bool {
	return affiliate.changed
}

// Update affiliate data and expiration.
func (affiliate *Affiliate) Update(data []byte, expiration time.Time) {
	affiliate.data = data
	affiliate.expiration = expiration
	affiliate.changed = true
}

// MergeEndpoints and potentially update expiration for endpoints.
func (affiliate *Affiliate) MergeEndpoints(endpoints [][]byte, expiration time.Time) error {
	for _, endpoint := range endpoints {
		found := false

		for i := range affiliate.endpoints {
			if bytes.Equal(affiliate.endpoints[i].data, endpoint) {
				found = true

				if affiliate.endpoints[i].expiration.Before(expiration) {
					affiliate.endpoints[i].expiration = expiration
				}

				break
			}
		}

		if !found {
			if len(affiliate.endpoints) >= limits.AffiliateEndpointsMax {
				return ErrTooManyEndpoints
			}

			affiliate.endpoints = append(affiliate.endpoints, Endpoint{
				expiration: expiration,
				data:       endpoint,
			})

			affiliate.changed = true
		}
	}

	if affiliate.expiration.Before(expiration) {
		affiliate.expiration = expiration
	}

	return nil
}

// GarbageCollect affiliate data.
//
// Endpoints are removed independent of the affiliate data.
func (affiliate *Affiliate) GarbageCollect(now time.Time) (remove, changed bool) {
	if affiliate.expiration.Before(now) {
		remove = true
		changed = true

		return
	}

	n := 0

	for _, endpoint := range affiliate.endpoints {
		if endpoint.expiration.After(now) {
			affiliate.endpoints[n] = endpoint
			n++
		} else {
			changed = true
		}
	}

	affiliate.endpoints = affiliate.endpoints[:n]

	return
}

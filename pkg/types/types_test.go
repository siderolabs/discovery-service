// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package types_test

import (
	"testing"
	"time"

	"inet.af/netaddr"

	"github.com/talos-systems/kubespan-manager/pkg/types"
)

func TestEncoderDecoder(t *testing.T) {
	n := &types.Node{
		Name: "tester",
		ID:   "IHOPEfmiUG1kE832FAxm77J5WP0O1ZHp9OwqbGowL1E=",
		IP:   netaddr.MustParseIP("2001:db8:1001::1"),
		Addresses: []*types.Address{
			{
				Name:         "mynode.mydomain.com",
				Port:         51512,
				LastReported: time.Now(),
			},
			{
				IP:           netaddr.MustParseIP("2001:db8:2002::2"),
				Port:         52522,
				LastReported: time.Now(),
			},
		},
	}

	data, err := n.MarshalBinary()
	if err != nil {
		t.Errorf("failed to marshal node: %w", err)
	}

	n2 := new(types.Node)
	if err = n2.UnmarshalBinary(data); err != nil {
		t.Errorf("failed to unmarshal node: %w", err)
	}

	if n.ID != n2.ID {
		t.Errorf("IDs do not match: %s != %s", n.ID, n2.ID)
	}
}

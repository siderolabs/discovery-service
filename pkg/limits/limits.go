// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package limits provides various service limits.
package limits

import "time"

// Service limits.
const (
	ClusterIDMax          = 256
	AffiliateIDMax        = 256
	AffiliateDataMax      = 2048
	AffiliateEndpointMax  = 32
	TTLMax                = 30 * time.Minute
	ClusterAffiliatesMax  = 1024
	AffiliateEndpointsMax = 64
)

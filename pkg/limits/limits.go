// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

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

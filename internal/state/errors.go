// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state

import "fmt"

var (
	// ErrTooManyAffiliates is raised when there are too many affiliates in the cluster.
	ErrTooManyAffiliates = fmt.Errorf("too many affiliates in the cluster")
	// ErrTooManyEndpoints is raised when there are too many endpoints in the affiliate.
	ErrTooManyEndpoints = fmt.Errorf("too many endpoints in the affiliate")
)

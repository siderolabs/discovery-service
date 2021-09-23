// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package state

import "fmt"

var (
	// ErrTooManyAffiliates is raised when there are too many affiliates in the cluster.
	ErrTooManyAffiliates = fmt.Errorf("too many affiliates in the cluster")
	// ErrTooManyEndpoints is raised when there are too many endpoints in the affiliate.
	ErrTooManyEndpoints = fmt.Errorf("too many endpoints in the affiliate")
)

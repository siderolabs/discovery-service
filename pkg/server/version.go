// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"regexp"
)

var vRE = regexp.MustCompile(`^(v\d+\.\d+\.\d+\-?[^-]*)(.*)$`)

func parseVersion(v string) string {
	m := vRE.FindAllStringSubmatch(v, -1)

	if len(m) == 1 && len(m[0]) == 3 {
		res := m[0][1]
		if m[0][2] != "" {
			res += "-dev"
		}

		return res
	}

	return "unknown"
}

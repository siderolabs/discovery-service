// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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

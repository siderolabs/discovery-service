// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"strings"
)

const unknownVersion = "unknown"

func parseVersion(v string) string {
	ver, suffix, _ := strings.Cut(v, "-")

	if ver == "" || ver[0] != 'v' {
		return unknownVersion
	}

	p1 := strings.IndexByte(ver, '.')
	if p1 == -1 || p1 > 3 {
		return unknownVersion
	}

	p2 := strings.IndexByte(ver[p1+1:], '.')
	if p2 == -1 || p2 > 3 {
		return unknownVersion
	}

	if suffix != "" {
		return ver[:p1+p2+1] + "-pre"
	}

	return ver[:p1+p2+1]
}

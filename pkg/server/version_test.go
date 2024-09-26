// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server //nolint:testpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	for v, expected := range map[string]string{
		"":                                  "unknown",
		"unknown":                           "unknown",
		"v0.13.0":                           "v0.13",
		"v0.13.0-beta.0":                    "v0.13-pre",
		"v0.14.0-alpha.0-7-gf7d9f211":       "v0.14-pre",
		"v0.14.0-alpha.0-7-gf7d9f211-dirty": "v0.14-pre",
		"v1.8.3":                            "v1.8",
		"v1.8.3-7-gf7d9f211":                "v1.8-pre",
		"v1.8.0-beta.1":                     "v1.8-pre",
	} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, parseVersion(v))
		})
	}
}

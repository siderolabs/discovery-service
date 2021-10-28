// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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
		"v0.13.0":                           "v0.13.0",
		"v0.13.0-beta.0":                    "v0.13.0-beta.0",
		"v0.14.0-alpha.0-7-gf7d9f211":       "v0.14.0-alpha.0-dev",
		"v0.14.0-alpha.0-7-gf7d9f211-dirty": "v0.14.0-alpha.0-dev",
	} {
		v, expected := v, expected

		t.Run(v, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, parseVersion(v))
		})
	}
}

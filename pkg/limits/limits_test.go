// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package limits_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/talos-systems/discovery-service/pkg/limits"
)

func TestDoGC(t *testing.T) {
	t.Parallel()

	const (
		testIP1 = "1.1.1.1"
		testIP2 = "2.2.2.2"
	)

	limiter := limits.NewIPRateLimiter(1, 1)

	// hit the bucket size
	lim1 := limiter.Get(testIP1)
	lim2 := limiter.Get(testIP2)
	lim2.Allow()

	assert.Exactly(t, 2, limiter.Len())

	limiter.DoGC(time.Now())

	assert.Exactly(t, 1, limiter.Len())

	// lim1 should have been gc'd, but not lim2
	assert.NotSame(t, lim1, limiter.Get(testIP1))
	assert.Same(t, lim2, limiter.Get(testIP2))

	// one more gc with T+2
	limiter.DoGC(time.Now().Add(2 * time.Second))

	assert.Exactly(t, 0, limiter.Len())
}

func TestIndependentLimiters(t *testing.T) {
	t.Parallel()

	const (
		testIP1 = "1.1.1.1"
		testIP2 = "2.2.2.2"
	)

	limiter := limits.NewIPRateLimiter(1, 1)

	require.True(t, limiter.Get(testIP1).Allow())
	require.False(t, limiter.Get(testIP1).Allow())
	require.True(t, limiter.Get(testIP2).Allow())
}

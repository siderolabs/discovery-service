// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

// IsZero tests if generic value T is zero.
func IsZero[T comparable](t T) bool {
	var zero T

	return t == zero
}

// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package limiter provides service resource limiters.
package limiter

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/siderolabs/discovery-service/pkg/limits"
)

// IPRateLimiter applies the same limits to a group of IP addresses.
type IPRateLimiter struct {
	ips        map[netip.Addr]*rate.Limiter
	mu         sync.Mutex
	rateLimit  rate.Limit
	bucketSize int
}

// NewIPRateLimiter returns a new IPRateLimiter.
func NewIPRateLimiter(rateLimit rate.Limit, bucketSize int) *IPRateLimiter {
	return &IPRateLimiter{
		ips:        make(map[netip.Addr]*rate.Limiter),
		rateLimit:  rateLimit,
		bucketSize: bucketSize,
	}
}

// Get returns the rate limiter for the provided IP address if it exists or creates a new one.
func (iPRL *IPRateLimiter) Get(ip netip.Addr) *rate.Limiter {
	iPRL.mu.Lock()
	defer iPRL.mu.Unlock()

	limiter, exists := iPRL.ips[ip]

	if !exists {
		limiter = rate.NewLimiter(iPRL.rateLimit, iPRL.bucketSize)
		iPRL.ips[ip] = limiter
	}

	return limiter
}

// Len returns the number of limiters.
func (iPRL *IPRateLimiter) Len() int {
	iPRL.mu.Lock()
	defer iPRL.mu.Unlock()

	return len(iPRL.ips)
}

// RunGC periodically clears IPs.
func (iPRL *IPRateLimiter) RunGC(ctx context.Context) {
	ticker := time.NewTicker(limits.IPRateGarbageCollectionPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			iPRL.DoGC(time.Now())
		case <-ctx.Done():
			return
		}
	}
}

// DoGC runs a single round of garbage collection.
func (iPRL *IPRateLimiter) DoGC(now time.Time) {
	iPRL.mu.Lock()
	for key, val := range iPRL.ips {
		// AllowN on success consumes the tokens, but as the limiter is going to be dropped, it doesn't matter
		if val.AllowN(now, iPRL.bucketSize) {
			delete(iPRL.ips, key)
		}
	}
	iPRL.mu.Unlock()
}

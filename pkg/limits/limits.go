// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package limits provides various service limits.
package limits

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Service limits.
const (
	ClusterIDMax            = 256
	AffiliateIDMax          = 256
	AffiliateDataMax        = 2048
	AffiliateEndpointMax    = 32
	TTLMax                  = 30 * time.Minute
	ClusterAffiliatesMax    = 1024
	AffiliateEndpointsMax   = 64
	RequestsPerSecondMax    = 5
	BurstSizeMax            = 30
	GarbageCollectionPeriod = time.Minute
)

// IPRateLimiter applies the same limits to a group of IP addresses.
type IPRateLimiter struct {
	ips        map[string]*rate.Limiter
	mu         sync.Mutex
	rateLimit  rate.Limit
	bucketSize int
}

// NewIPRateLimiter returns a new IPRateLimiter.
func NewIPRateLimiter(rateLimit rate.Limit, bucketSize int) *IPRateLimiter {
	return &IPRateLimiter{
		ips:        make(map[string]*rate.Limiter),
		rateLimit:  rateLimit,
		bucketSize: bucketSize,
	}
}

// Get returns the rate limiter for the provided IP address if it exists or creates a new one.
func (iPRL *IPRateLimiter) Get(ip string) *rate.Limiter {
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
	ticker := time.NewTicker(GarbageCollectionPeriod)
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

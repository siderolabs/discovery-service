// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package limits provides various service limits.
package limits

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Service limits.
const (
	ClusterIDMax          = 256
	AffiliateIDMax        = 256
	AffiliateDataMax      = 2048
	AffiliateEndpointMax  = 32
	TTLMax                = 30 * time.Minute
	ClusterAffiliatesMax  = 1024
	AffiliateEndpointsMax = 64
	RequestsPerSecondMax  = 5
)

// IPRateLimiter type.
type IPRateLimiter struct {
	IPs        map[string]*rate.Limiter
	Mu         *sync.RWMutex
	RateLimit  rate.Limit
	BucketSize int
}

// NewIPRateLimiter returns newly initialized IPRateLimiter.
func NewIPRateLimiter(rateLimit rate.Limit, bucketSize int) *IPRateLimiter {
	i := &IPRateLimiter{
		IPs:        make(map[string]*rate.Limiter),
		Mu:         &sync.RWMutex{},
		RateLimit:  rateLimit,
		BucketSize: bucketSize,
	}

	return i
}

// AddIP creates a new rate limiter and adds it to the ips map,
// using the IP address as the key.
func (iPRL *IPRateLimiter) AddIP(ip string) *rate.Limiter {
	iPRL.Mu.Lock()
	defer iPRL.Mu.Unlock()

	limiter := rate.NewLimiter(iPRL.RateLimit, iPRL.BucketSize)

	iPRL.IPs[ip] = limiter

	return limiter
}

// GetLimiter returns the rate limiter for the provided IP address if it exists.
// Otherwise calls AddIP to add IP address to the map.
func (iPRL *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	iPRL.Mu.Lock()
	limiter, exists := iPRL.IPs[ip]

	if !exists {
		iPRL.Mu.Unlock()

		return iPRL.AddIP(ip)
	}

	iPRL.Mu.Unlock()

	return limiter
}

// IPAddrGarbageCollector periodically empties requests limit.
func (iPRL *IPRateLimiter) IPAddrGarbageCollector() {
	ticker := time.NewTicker(500 * time.Millisecond)

	for range ticker.C {
		for key, val := range iPRL.IPs {
			if !val.Allow() {
				delete(iPRL.IPs, key)
			}
		}
	}
}

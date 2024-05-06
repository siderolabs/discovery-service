// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state

import "time"

// EndpointExport represents a read-only copy of an endpoint.
type EndpointExport struct {
	Expiration time.Time `json:"expiration,omitempty"`
	Data       []byte    `json:"data,omitempty"`
}

// Export exports the endpoint into EndpointExport.
func (endpoint *Endpoint) Export() *EndpointExport {
	return &EndpointExport{
		Data:       endpoint.data,
		Expiration: endpoint.expiration,
	}
}

// AffiliateExport represents a read-only copy of an affiliate.
type AffiliateExport struct {
	Expiration time.Time         `json:"expiration,omitempty"`
	ID         string            `json:"id"`
	Endpoints  []*EndpointExport `json:"endpoints,omitempty"`
	Data       []byte            `json:"data,omitempty"`
	Changed    bool              `json:"changed,omitempty"`
}

// Export exports the affiliate into AffiliateExport.
func (affiliate *Affiliate) Export() *AffiliateExport {
	result := &AffiliateExport{
		ID:         affiliate.id,
		Data:       affiliate.data,
		Expiration: affiliate.expiration,
		Changed:    affiliate.changed,
	}

	for _, endpoint := range affiliate.endpoints {
		result.Endpoints = append(result.Endpoints, endpoint.Export())
	}

	return result
}

// ClusterExport represents a read-only copy of a cluster.
type ClusterExport struct {
	ID         string             `json:"id"`
	Affiliates []*AffiliateExport `json:"affiliates,omitempty"`
}

// Export exports the cluster into ClusterExport.
func (cluster *Cluster) Export() *ClusterExport {
	return &ClusterExport{
		ID:         cluster.id,
		Affiliates: cluster.List(),
	}
}

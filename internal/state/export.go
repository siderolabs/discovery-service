// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package state

// AffiliateExport represents a read-only copy of affiliate.
type AffiliateExport struct {
	ID        string
	Data      []byte
	Endpoints [][]byte
}

// Export the affiliate into AffiliateExport.
func (affiliate *Affiliate) Export() *AffiliateExport {
	result := &AffiliateExport{
		ID:   affiliate.id,
		Data: affiliate.data,
	}

	result.Endpoints = make([][]byte, 0, len(affiliate.endpoints))

	for _, endpoint := range affiliate.endpoints {
		result.Endpoints = append(result.Endpoints, endpoint.data)
	}

	return result
}

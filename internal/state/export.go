// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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

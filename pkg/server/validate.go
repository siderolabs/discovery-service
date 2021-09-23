// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package server

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/talos-systems/discovery-service/pkg/limits"
)

func validateClusterID(id string) error {
	if len(id) < 1 {
		return status.Errorf(codes.InvalidArgument, "cluster ID can't be empty")
	}

	if len(id) > limits.ClusterIDMax {
		return status.Errorf(codes.InvalidArgument, "cluster ID is too long")
	}

	return nil
}

func validateAffiliateID(id string) error {
	if len(id) < 1 {
		return status.Errorf(codes.InvalidArgument, "affiliate ID can't be empty")
	}

	if len(id) > limits.AffiliateIDMax {
		return status.Errorf(codes.InvalidArgument, "affiliate ID is too long")
	}

	return nil
}

func validateAffiliateData(data []byte) error {
	if len(data) > limits.AffiliateDataMax {
		return status.Error(codes.InvalidArgument, "affiliate data is too big")
	}

	return nil
}

func validateAffiliateEndpoints(endpoints [][]byte) error {
	for _, endpoint := range endpoints {
		if len(endpoint) > limits.AffiliateEndpointMax {
			return status.Errorf(codes.InvalidArgument, "affiliate endpoint is too big")
		}
	}

	return nil
}

func validateTTL(ttl time.Duration) error {
	if ttl > limits.TTLMax {
		return status.Errorf(codes.InvalidArgument, "ttl is too large")
	}

	return nil
}

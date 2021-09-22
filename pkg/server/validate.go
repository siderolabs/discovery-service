// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package server

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	clusterIDMax   = 256
	affiliateIDMax = 256
)

func validateClusterID(id string) error {
	if len(id) < 1 {
		return status.Errorf(codes.InvalidArgument, "cluster ID can't be empty")
	}

	if len(id) > clusterIDMax {
		return status.Errorf(codes.InvalidArgument, "cluster ID is too long")
	}

	return nil
}

func validateAffiliateID(id string) error {
	if len(id) < 1 {
		return status.Errorf(codes.InvalidArgument, "affiliate ID can't be empty")
	}

	if len(id) > affiliateIDMax {
		return status.Errorf(codes.InvalidArgument, "affiliate ID is too long")
	}

	return nil
}

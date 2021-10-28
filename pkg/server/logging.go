// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldExtractor prepares tags for logging and tracing out of the request.
func FieldExtractor(fullMethod string, req interface{}) map[string]interface{} {
	if msg, ok := req.(proto.Message); ok {
		r := msg.ProtoReflect()
		fields := r.Descriptor().Fields()

		ret := map[string]interface{}{}

		for _, name := range []string{"cluster_id", "affiliate_id", "client_version"} {
			if field := fields.ByName(protoreflect.Name(name)); field != nil {
				ret[name] = r.Get(field).String()
			}
		}

		if len(ret) > 0 {
			return ret
		}
	}

	return nil
}

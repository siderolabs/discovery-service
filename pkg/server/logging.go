// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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

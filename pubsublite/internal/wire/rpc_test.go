// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package wire

import (
	"encoding/base64"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestPubsubMetadataAddClientInfo(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		framework      FrameworkType
		libraryVersion func() (version, bool)
		wantClientInfo *structpb.Struct
	}{
		{
			desc: "minimal",
			libraryVersion: func() (version, bool) {
				return version{}, false
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
		{
			desc:      "cps shim",
			framework: FrameworkCloudPubSubShim,
			libraryVersion: func() (version, bool) {
				return version{}, false
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language":  stringValue("GOLANG"),
					"framework": stringValue("CLOUD_PUBSUB_SHIM"),
				},
			},
		},
		{
			desc:      "version valid",
			framework: FrameworkCloudPubSubShim,
			libraryVersion: func() (version, bool) {
				return version{Major: "1", Minor: "2"}, true
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language":      stringValue("GOLANG"),
					"framework":     stringValue("CLOUD_PUBSUB_SHIM"),
					"major_version": stringValue("1"),
					"minor_version": stringValue("2"),
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			metadata := newPubsubMetadata()
			metadata.doAddClientInfo(tc.framework, tc.libraryVersion)

			b, err := base64.StdEncoding.DecodeString(metadata["x-goog-pubsub-context"])
			if err != nil {
				t.Errorf("Failed to decode base64 pubsub context header: %v", err)
				return
			}
			gotClientInfo := new(structpb.Struct)
			if err := proto.Unmarshal(b, gotClientInfo); err != nil {
				t.Errorf("Failed to unmarshal pubsub context structpb: %v", err)
				return
			}
			if diff := testutil.Diff(gotClientInfo, tc.wantClientInfo); diff != "" {
				t.Errorf("Pubsub context structpb: got: -, want: +\n%s", diff)
			}
		})
	}
}

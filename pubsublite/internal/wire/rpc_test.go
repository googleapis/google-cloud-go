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
	"runtime/debug"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestPubsubMetadataAddClientInfo(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		framework      FrameworkType
		buildInfo      *debug.BuildInfo
		wantClientInfo *structpb.Struct
	}{
		{
			desc: "minimal",
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
		{
			desc:      "cps shim",
			framework: FrameworkCloudPubSubShim,
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
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "v1.2.2"},
					{Path: "cloud.google.com/go/pubsub", Version: "v1.8.3"},
				},
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
		{
			desc: "version corner case",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "2.3"},
				},
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language":      stringValue("GOLANG"),
					"major_version": stringValue("2"),
					"minor_version": stringValue("3"),
				},
			},
		},
		{
			desc: "version missing",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsub", Version: "v1.8.3"},
				},
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
		{
			desc: "minor version invalid",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "v1.a.2"},
				},
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
		{
			desc: "major version invalid",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "vb.1.2"},
				},
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
		{
			desc: "minor version missing",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "v4"},
				},
			},
			wantClientInfo: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"language": stringValue("GOLANG"),
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			metadata := newPubsubMetadata()
			metadata.doAddClientInfo(tc.framework, tc.buildInfo)

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

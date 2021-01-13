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

// +build go1.12

package wire

import (
	"runtime/debug"
	"testing"

	"cloud.google.com/go/internal/testutil"
)

func TestPubsubliteModuleVersion(t *testing.T) {
	for _, tc := range []struct {
		desc        string
		buildInfo   *debug.BuildInfo
		wantVersion version
		wantOk      bool
	}{
		{
			desc: "version valid",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "v1.2.2"},
					{Path: "cloud.google.com/go/pubsub", Version: "v1.8.3"},
				},
			},
			wantVersion: version{Major: "1", Minor: "2"},
			wantOk:      true,
		},
		{
			desc: "version corner case",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "2.3"},
				},
			},
			wantVersion: version{Major: "2", Minor: "3"},
			wantOk:      true,
		},
		{
			desc: "version missing",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsub", Version: "v1.8.3"},
				},
			},
			wantOk: false,
		},
		{
			desc: "minor version invalid",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "v1.a.2"},
				},
			},
			wantOk: false,
		},
		{
			desc: "major version invalid",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "vb.1.2"},
				},
			},
			wantOk: false,
		},
		{
			desc: "minor version missing",
			buildInfo: &debug.BuildInfo{
				Deps: []*debug.Module{
					{Path: "cloud.google.com/go/pubsublite", Version: "v4"},
				},
			},
			wantOk: false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if gotVersion, gotOk := pubsubliteModuleVersion(tc.buildInfo); !testutil.Equal(gotVersion, tc.wantVersion) || gotOk != tc.wantOk {
				t.Errorf("pubsubliteModuleVersion(): got (%v, %v), want (%v, %v)", gotVersion, gotOk, tc.wantVersion, tc.wantOk)
			}
		})
	}
}

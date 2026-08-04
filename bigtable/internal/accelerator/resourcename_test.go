// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package accelerator

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testProject  = "p"
	testInstance = "i"
)

// scopedTestChannel returns a Channel with only the scope the resource-name
// parsers depend on (no session.Client dialed).
func scopedTestChannel() *Channel {
	return &Channel{scopePrefix: scopePrefixFor(testProject, testInstance)}
}

func TestParseTableName(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantLeaf string
		wantCode codes.Code
	}{
		{"in-scope", "projects/p/instances/i/tables/t", "t", codes.OK},
		{"wrong-project", "projects/other/instances/i/tables/t", "", codes.InvalidArgument},
		{"wrong-instance", "projects/p/instances/other/tables/t", "", codes.InvalidArgument},
		{"not-a-table", "projects/p/instances/i/materializedViews/mv", "", codes.InvalidArgument},
		{"empty-leaf", "projects/p/instances/i/tables/", "", codes.InvalidArgument},
		{"extra-segment", "projects/p/instances/i/tables/t/authorizedViews/v", "", codes.InvalidArgument},
		{"bare-leaf", "t", "", codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scopedTestChannel().parseTableName(tc.in)
			if status.Code(err) != tc.wantCode {
				t.Fatalf("parseTableName(%q) code = %v; want %v (err=%v)", tc.in, status.Code(err), tc.wantCode, err)
			}
			if got != tc.wantLeaf {
				t.Errorf("parseTableName(%q) = %q; want %q", tc.in, got, tc.wantLeaf)
			}
		})
	}
}

func TestParseAuthorizedViewName(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantTable string
		wantView  string
		wantCode  codes.Code
	}{
		{"in-scope", "projects/p/instances/i/tables/t/authorizedViews/v", "t", "v", codes.OK},
		{"wrong-project", "projects/other/instances/i/tables/t/authorizedViews/v", "", "", codes.InvalidArgument},
		{"missing-view", "projects/p/instances/i/tables/t", "", "", codes.InvalidArgument},
		{"empty-view", "projects/p/instances/i/tables/t/authorizedViews/", "", "", codes.InvalidArgument},
		{"empty-table", "projects/p/instances/i/tables//authorizedViews/v", "", "", codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotT, gotV, err := scopedTestChannel().parseAuthorizedViewName(tc.in)
			if status.Code(err) != tc.wantCode {
				t.Fatalf("parseAuthorizedViewName(%q) code = %v; want %v (err=%v)", tc.in, status.Code(err), tc.wantCode, err)
			}
			if gotT != tc.wantTable || gotV != tc.wantView {
				t.Errorf("parseAuthorizedViewName(%q) = (%q, %q); want (%q, %q)", tc.in, gotT, gotV, tc.wantTable, tc.wantView)
			}
		})
	}
}

func TestParseMaterializedViewName(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantLeaf string
		wantCode codes.Code
	}{
		{"in-scope", "projects/p/instances/i/materializedViews/mv", "mv", codes.OK},
		{"wrong-instance", "projects/p/instances/other/materializedViews/mv", "", codes.InvalidArgument},
		{"not-a-mat-view", "projects/p/instances/i/tables/t", "", codes.InvalidArgument},
		{"empty-leaf", "projects/p/instances/i/materializedViews/", "", codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scopedTestChannel().parseMaterializedViewName(tc.in)
			if status.Code(err) != tc.wantCode {
				t.Fatalf("parseMaterializedViewName(%q) code = %v; want %v (err=%v)", tc.in, status.Code(err), tc.wantCode, err)
			}
			if got != tc.wantLeaf {
				t.Errorf("parseMaterializedViewName(%q) = %q; want %q", tc.in, got, tc.wantLeaf)
			}
		})
	}
}

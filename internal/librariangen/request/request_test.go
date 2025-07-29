// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package request

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		want    *Request
		wantErr bool
	}{
		{
			name: "valid request",
			content: `{
				"id": "google-cloud-asset-v1",
				"version": "1.15.0",
				"apis": [
					{
						"path": "google/cloud/asset/v1",
						"service_config": "cloudasset_v1.yaml"
					}
				],
				"source_paths": ["asset/apiv1"],
				"preserve_regex": ["asset/apiv1/foo.go"],
				"remove_regex": ["asset/apiv1/bar.go"]
			}`,
			want: &Request{
				ID:      "google-cloud-asset-v1",
				Version: "1.15.0",
				APIs: []API{
					{
						Path:          "google/cloud/asset/v1",
						ServiceConfig: "cloudasset_v1.yaml",
					},
				},
				SourcePaths:   []string{"asset/apiv1"},
				PreserveRegex: []string{"asset/apiv1/foo.go"},
				RemoveRegex:   []string{"asset/apiv1/bar.go"},
			},
			wantErr: false,
		},
		{
			name:    "malformed json",
			content: `{"id": "foo",`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			reqPath := filepath.Join(tmpDir, "generate-request.json")
			if err := os.WriteFile(reqPath, []byte(tc.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			got, err := Parse(reqPath)

			if (err != nil) != tc.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if !tc.wantErr {
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("non-existent-file.json")
	if err == nil {
		t.Error("Parse() expected error for non-existent file, got nil")
	}
}

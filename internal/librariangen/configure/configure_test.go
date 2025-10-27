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

package configure

import (
	"testing"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
				SourceDir:    "d",
				RepoDir:      "e",
			},
			wantErr: false,
		},
		{
			name: "missing librarian dir",
			cfg: &Config{
				InputDir:  "b",
				OutputDir: "c",
				SourceDir: "d",
				RepoDir:   "e",
			},
			wantErr: true,
		},
		{
			name: "missing input dir",
			cfg: &Config{
				LibrarianDir: "a",
				OutputDir:    "c",
				SourceDir:    "d",
				RepoDir:      "e",
			},
			wantErr: true,
		},
		{
			name: "missing output dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				SourceDir:    "d",
				RepoDir:      "e",
			},
			wantErr: true,
		},
		{
			name: "missing source dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
				RepoDir:      "e",
			},
			wantErr: true,
		},
		{
			name: "missing repo dir",
			cfg: &Config{
				LibrarianDir: "a",
				InputDir:     "b",
				OutputDir:    "c",
				SourceDir:    "d",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFindLibraryAndAPIToConfigure(t *testing.T) {
	tests := []struct {
		name     string
		req      *Request
		wantID   string
		wantPath string
		wantErr  bool
	}{
		{
			name: "valid new library",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "old1",
						APIs: []request.API{
							{
								Path: "old1",
							},
						},
					},
					{
						ID: "new",
						APIs: []request.API{
							{
								Path:   "a/b/c",
								Status: NewAPIStatus,
							},
						},
					},
					{
						ID: "old2",
						APIs: []request.API{
							{
								Path: "old2",
							},
						},
					},
				},
			},
			wantID:   "new",
			wantPath: "a/b/c",
		},
		{
			name: "valid updated library",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "old1",
						APIs: []request.API{
							{
								Path: "old1",
							},
						},
					},
					{
						ID: "updated",
						APIs: []request.API{
							{
								Path: "a/b/c",
							},
							{
								Path:   "e/f/g",
								Status: NewAPIStatus,
							},
							{
								Path: "old",
							},
						},
					},
					{
						ID: "old2",
						APIs: []request.API{
							{
								Path: "old2",
							},
						},
					},
				},
			},
			wantID:   "updated",
			wantPath: "e/f/g",
		},
		{
			name: "invalid no new APIs",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "old1",
						APIs: []request.API{
							{
								Path: "old1",
							},
						},
					},
					{
						ID: "old2",
						APIs: []request.API{
							{
								Path: "old2",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple new libraries",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "new1",
						APIs: []request.API{
							{
								Path:   "new1",
								Status: NewAPIStatus,
							},
						},
					},
					{
						ID: "new1",
						APIs: []request.API{
							{
								Path:   "new2",
								Status: NewAPIStatus,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple new APIs in one library",
			req: &Request{
				Libraries: []*request.Library{
					{
						ID: "new1",
						APIs: []request.API{
							{
								Path:   "new1",
								Status: NewAPIStatus,
							},
							{
								Path:   "new2",
								Status: NewAPIStatus,
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib, api, err := findLibraryAndAPIToConfigure(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("findLibraryToConfigure error = %v, wantErr %v", err, tt.wantErr)
			}
			// We assume that if the ID is correct, the rest is right too (i.e. we're just
			// picking the right struct).
			if tt.wantID != "" && lib.ID != tt.wantID {
				t.Errorf("mismatched ID, got=%s, want=%s", lib.ID, tt.wantID)
			}
			if tt.wantPath != "" && api.Path != tt.wantPath {
				t.Errorf("mismatched API path, got=%s, want=%s", api.Path, tt.wantPath)
			}
		})
	}
}

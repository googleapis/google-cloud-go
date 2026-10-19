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

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateModFile(t *testing.T) {
	for _, tc := range []struct {
		desc         string
		fileContents []byte
		probeVersion string
		wantErr      bool
	}{
		{
			desc:    "empty",
			wantErr: true,
		},
		{
			desc: "invalid",
			fileContents: []byte(
				`
				random gobbledegook

				`),
			wantErr: true,
		},
		{
			desc: "valid but mismatch",
			fileContents: []byte(
				`
				module foo

				go 1.23.4
				`),
			probeVersion: "1.25",
			wantErr:      true,
		},
		{
			desc: "validated",
			fileContents: []byte(
				`module foobarbaz

				go 1.23.4
				`),
			probeVersion: "1.23.4",
			wantErr:      false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			tmpDir := t.TempDir()
			modPath := filepath.Join(tmpDir, "go.mod")
			err := os.WriteFile(modPath, tc.fileContents, 0644)
			if err != nil {
				t.Fatalf("failed to write test data (%q): %v", modPath, err)
			}
			err = validateModFile(modPath, tc.probeVersion)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("wanted error, got success")
				}
			} else {
				if err != nil {
					t.Fatalf("wanted success, got err: %v", err)
				}
			}
		})

	}

}

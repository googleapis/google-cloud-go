// Copyright 2023 Google LLC
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

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/internal/gapicgen/git"
	"github.com/google/go-cmp/cmp"
)

var googleapisDir string

func TestMain(m *testing.M) {
	flag.StringVar(&googleapisDir, "googleapis-dir", "", "Enter local googleapisDir to avoid cloning")
	flag.Parse()

	if googleapisDir == "" {
		log.Println("creating temp dir")
		tmpDir, err := os.MkdirTemp("", "update-postprocessor")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		log.Printf("working out %s\n", tmpDir)
		googleapisDir = filepath.Join(tmpDir, "googleapis")
		if err := git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir); err != nil {
			log.Fatalf("%v", err)
		}
	}

	os.Exit(m.Run())
}

func TestProcessCommit(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		body    string
		want    string
		want1   string
		wantErr bool
	}{
		{
			name:  "first test",
			title: "feat: [REPLACEME] Adds named reservation to InstancePolicy",
			body: `- [ ] Regenerate this pull request now.

---
docs:Remove "not yet implemented" for Accelerator & Refine Volume API docs

---
docs: update the job id format requirement

PiperOrigin-RevId: 489502315

Source-Link: https://togithub.com/googleapis/googleapis/commit/db1cc1139fe0def1e87ead1fffbc5bedbeccb887

Source-Link: https://togithub.com/googleapis/googleapis-gen/commit/fcc564ef064c7dff31d7970e12318ad084703ac6
Copy-Tag: eyJwIjoiamF2YS1iYXRjaC8uT3dsQm90LnlhbWwiLCJoIjoiZmNjNTY0ZWYwNjRjN2RmZjMxZDc5NzBlMTIzMThhZDA4NDcwM2FjNiJ9

BEGIN_NESTED_COMMIT
feat: [REPLACEME] Adds named reservation to InstancePolicy
---
docs:Remove "not yet implemented" for Accelerator & Refine Volume API docs

---
docs: update the job id format requirement

PiperOrigin-RevId: 489501779

Source-Link: https://togithub.com/googleapis/googleapis/commit/488a4bdeebf9c7f505f48bed23f0b95fcbbec0bb

Source-Link: https://togithub.com/googleapis/googleapis-gen/commit/5b3d3a550015e9367ad13ee5f9febe0c3f84cf33
Copy-Tag: eyJwIjoiamF2YS1iYXRjaC8uT3dsQm90LnlhbWwiLCJoIjoiNWIzZDNhNTUwMDE1ZTkzNjdhZDEzZWU1ZjlmZWJlMGMzZjg0Y2YzMyJ9
END_NESTED_COMMIT`,
			want: "feat(batch): Adds named reservation to InstancePolicy",
			want1: `- [ ] Regenerate this pull request now.

---
docs:Remove "not yet implemented" for Accelerator & Refine Volume API docs

---
docs: update the job id format requirement

PiperOrigin-RevId: 489502315

Source-Link: https://togithub.com/googleapis/googleapis/commit/db1cc1139fe0def1e87ead1fffbc5bedbeccb887

Source-Link: https://togithub.com/googleapis/googleapis-gen/commit/fcc564ef064c7dff31d7970e12318ad084703ac6
Copy-Tag: eyJwIjoiamF2YS1iYXRjaC8uT3dsQm90LnlhbWwiLCJoIjoiZmNjNTY0ZWYwNjRjN2RmZjMxZDc5NzBlMTIzMThhZDA4NDcwM2FjNiJ9

BEGIN_NESTED_COMMIT
feat: Adds named reservation to InstancePolicy
---
docs:Remove "not yet implemented" for Accelerator & Refine Volume API docs

---
docs: update the job id format requirement

PiperOrigin-RevId: 489501779

Source-Link: https://togithub.com/googleapis/googleapis/commit/488a4bdeebf9c7f505f48bed23f0b95fcbbec0bb

Source-Link: https://togithub.com/googleapis/googleapis-gen/commit/5b3d3a550015e9367ad13ee5f9febe0c3f84cf33
Copy-Tag: eyJwIjoiamF2YS1iYXRjaC8uT3dsQm90LnlhbWwiLCJoIjoiNWIzZDNhNTUwMDE1ZTkzNjdhZDEzZWU1ZjlmZWJlMGMzZjg0Y2YzMyJ9
END_NESTED_COMMIT`,
		},
	}
	for _, tt := range tests {

		c := &config{
			googleapisDir: googleapisDir,
		}
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := c.processCommit(tt.title, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("processCommit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("processCommit() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("processCommit() got1 = %v, want %v", got1, tt.want1)
			}
			if diff := cmp.Diff(tt.want1, got1); diff != "" {
				t.Errorf("processCommit() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

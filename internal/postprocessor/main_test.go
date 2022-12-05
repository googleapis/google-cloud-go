package main

import (
	"log"
	"path/filepath"
	"testing"

	"cloud.google.com/go/internal/gapicgen/git"
)

func TestProcessCommit(t *testing.T) {
	log.Println("creating temp dir")
	tmpDir := t.TempDir()

	log.Printf("working out %s\n", tmpDir)
	googleapisDir := filepath.Join(tmpDir, "googleapis")
	if err := git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir); err != nil {
		t.Fatalf("%v", err)
	}

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
feat(batch): Adds named reservation to InstancePolicy
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
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := processCommit(tt.title, tt.body, googleapisDir)
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
		})
	}
}

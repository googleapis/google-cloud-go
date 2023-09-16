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
	"bytes"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		if err := DeepClone("https://github.com/googleapis/googleapis", googleapisDir); err != nil {
			log.Fatalf("%v", err)
		}
	}

	os.Exit(m.Run())
}

func TestProcessCommit(t *testing.T) {
	tests := []struct {
		name         string
		title        string
		bodyFilename string
		wantTitle    string
		wantFilename string
	}{
		{
			name:         "nested commits",
			title:        "feat: Adds named reservation to InstancePolicy",
			bodyFilename: "testdata/nested-commits.input",
			wantTitle:    "feat(batch): Adds named reservation to InstancePolicy",
			wantFilename: "testdata/nested-commits.output",
		},
		{
			name:         "nested client scope",
			title:        "feat: added JSON_PACKAGE field to ExportAgentRequest",
			bodyFilename: "testdata/nested-client-scope.input",
			wantTitle:    "feat(dialogflow/cx): added JSON_PACKAGE field to ExportAgentRequest",
			wantFilename: "testdata/nested-client-scope.output",
		},
		{
			name:         "add commit delimiters",
			title:        "feat: Adds named reservation to InstancePolicy",
			bodyFilename: "testdata/add-commit-delimiters.input",
			wantTitle:    "feat(batch): Adds named reservation to InstancePolicy",
			wantFilename: "testdata/add-commit-delimiters.output",
		},
		{
			name:         "separate multiple commits",
			title:        "feat: Adds named reservation to InstancePolicy",
			bodyFilename: "testdata/separate-multiple-commits.input",
			wantTitle:    "feat(batch): Adds named reservation to InstancePolicy",
			wantFilename: "testdata/separate-multiple-commits.output",
		},
		{
			name:         "don't modify",
			title:        "feat(batch): Adds named reservation to InstancePolicy",
			bodyFilename: "testdata/separate-multiple-commits2.input",
			wantTitle:    "feat(batch): Adds named reservation to InstancePolicy",
			wantFilename: "testdata/separate-multiple-commits2.output",
		},
	}
	for _, tt := range tests {
		p := &postProcessor{
			googleapisDir:  googleapisDir,
			googleCloudDir: "../..",
		}
		p.loadConfig()
		t.Run(tt.name, func(t *testing.T) {
			body, err := os.ReadFile(tt.bodyFilename)
			if err != nil {
				t.Fatalf("os.ReadFile() = %v", err)
			}
			wantBody, err := os.ReadFile(tt.wantFilename)
			if err != nil {
				t.Fatalf("os.ReadFile() = %v", err)
			}
			gotTitle, gotBody, err := p.processCommit(tt.title, string(body))
			if err != nil {
				t.Errorf("processCommit() = %v", err)
				return
			}
			if diff := cmp.Diff(tt.wantTitle, gotTitle); diff != "" {
				t.Errorf("processCommit() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(string(wantBody), gotBody); diff != "" {
				t.Errorf("processCommit() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUpdateSnippetsMetadata(t *testing.T) {
	p := &postProcessor{
		config: &config{
			ClientRelPaths: []string{
				"/video/stitcher/apiv1",
			},
		},
		modules: []string{
			"video",
		},
		googleCloudDir: "testdata",
	}
	err := p.UpdateSnippetsMetadata()
	if err != nil {
		t.Errorf("UpdateSnippetsMetadata() = %v", err)
	}

	// Assert result and restore testdata
	f := filepath.FromSlash("testdata/internal/generated/snippets/video/stitcher/apiv1/snippet_metadata.google.cloud.video.stitcher.v1.json")
	read, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(read), "3.45.6") {
		s := strings.Replace(string(read), "3.45.6", "$VERSION", 1)
		err = os.WriteFile(f, []byte(s), 0)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatalf("UpdateSnippetsMetadata() did not update metadata as expected, check %s", f)
	}

}

func TestUpdateConfigFile(t *testing.T) {
	var b bytes.Buffer
	if err := updateConfigFile(&b, []string{"accessapproval", "newmod"}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/release-please-config-yoshi-submodules.want")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, b.Bytes()); diff != "" {
		t.Errorf("updateConfigFile() mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateManifestFile(t *testing.T) {
	existing, err := os.ReadFile("testdata/.release-please-manifest-submodules.json")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := updateManifestFile(&b, existing, []string{"accessapproval", "newmod"}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/.release-please-manifest-submodules.want")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, b.Bytes()); diff != "" {
		t.Errorf("updateConfigFile() mismatch (-want +got):\n%s", diff)
	}
}

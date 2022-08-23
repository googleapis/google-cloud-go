// Copyright 2022 Google LLC
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

//go:build go1.18
// +build go1.18

package aliasgen

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	// needed for package loading/parsing to work properly
	"github.com/google/go-cmp/cmp"
	_ "google.golang.org/grpc"
)

var updateGoldens bool

func TestMain(m *testing.M) {
	flag.BoolVar(&updateGoldens, "update-goldens", false, "Update the golden files")
	flag.Parse()
	isTest = true
	os.Exit(m.Run())
}

func TestGolden(t *testing.T) {
	srcDir := "testdata/fakepb"
	goldenDir := "testdata/golden"
	destDir := t.TempDir()

	if updateGoldens {
		os.RemoveAll(goldenDir)
		if err := Run(srcDir, filepath.Join(goldenDir, "fake")); err != nil {
			t.Fatalf("Run: %v", err)
		}
		t.Logf("Successfully updated golden files in %q", goldenDir)
		return
	}
	if err := Run(srcDir, destDir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// compare files excluding first header line with year
	gotBytes, err := os.ReadFile(filepath.Join(destDir, "alias.go"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	wantBytes, err := os.ReadFile(filepath.Join(goldenDir, "fake", "alias.go"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	gotBytes = gotBytes[bytes.IndexRune(gotBytes, '\n')+1:]
	wantBytes = wantBytes[bytes.IndexRune(wantBytes, '\n')+1:]
	if diff := cmp.Diff(wantBytes, gotBytes); diff != "" {
		t.Errorf("bytes mismatch (-want +got):\n%s", diff)
	}

	if ok := bytes.Equal(gotBytes, wantBytes); !ok {
		t.Fatalf("got %s, want %s", gotBytes, wantBytes)
	}
}

// Copyright 2016 Google LLC
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

package cloud

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sentinels = []string{
	"Copyright",
	"Google",
	`Licensed under the Apache License, Version 2.0 (the "License");`,
}

func TestLicense(t *testing.T) {
	t.Parallel()
	skip := map[string]bool{
		// Automatically generated.
		"bigtable/cmd/cbt/cbtdoc.go": true,

		// BSD license, which is compatible, is embedded in the file.
		"cmd/go-cloud-debug-agent/internal/debug/elf/elf.go": true,

		// From https://github.com/golang/pkgsite.
		"third_party/pkgsite/print_type.go":   true,
		"third_party/pkgsite/synopsis.go":     true,
		"third_party/go/doc/comment.go":       true,
		"third_party/go/doc/comment_test.go":  true,
		"third_party/go/doc/doc.go":           true,
		"third_party/go/doc/example.go":       true,
		"third_party/go/doc/example_test.go":  true,
		"third_party/go/doc/exports.go":       true,
		"third_party/go/doc/filter.go":        true,
		"third_party/go/doc/headscan.go":      true,
		"third_party/go/doc/reader.go":        true,
		"third_party/go/doc/synopsis.go":      true,
		"third_party/go/doc/synopsis_test.go": true,
	}
	err := filepath.Walk(".", func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if ext := filepath.Ext(path); ext != ".go" && ext != ".proto" {
			return nil
		}
		if strings.HasSuffix(path, ".pb.go") {
			// .pb.go files are generated from the proto files.
			// .proto files must have license headers.
			return nil
		}
		if skip[path] {
			return nil
		}
		src, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}
		src = src[:300] // Ensure all of the sentinel values are at the top of the file.

		// Find license
		for _, sentinel := range sentinels {
			if !bytes.Contains(src, []byte(sentinel)) {
				t.Errorf("%v: license header not present. want %q", path, sentinel)
				return nil
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

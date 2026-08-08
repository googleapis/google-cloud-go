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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	outDir := "obj/api/help"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("failed to create directory: %v", err)
	}

	files := []string{"README.md", "debug.md"}
	for _, f := range files {
		path := filepath.Join("internal", "help", f)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("failed to read %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, f), data, 0644); err != nil {
			log.Fatalf("failed to write %s: %v", f, err)
		}
	}

	now := time.Now().UTC()
	metadata := fmt.Sprintf(`update_time {
  seconds: %d
  nanos: %d
}
name: "help"
version: "1.0.0"
language: "go"
`, now.Unix(), now.Nanosecond())
	if err := os.WriteFile(filepath.Join(outDir, "docs.metadata"), []byte(metadata), 0644); err != nil {
		log.Fatalf("failed to write docs.metadata: %v", err)
	}

	toc := `### YamlMime:TableOfContent
items:
  - name: 'Getting Started'
    href: README.md
  - name: Troubleshooting
    href: debug.md
`
	if err := os.WriteFile(filepath.Join(outDir, "toc.yaml"), []byte(toc), 0644); err != nil {
		log.Fatalf("failed to write toc.yaml: %v", err)
	}
	fmt.Println("Generated help package in", outDir)
}

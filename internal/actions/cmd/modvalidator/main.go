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
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

var (
	dir      = flag.String("dir", "", "the root directory to evaluate")
	enforced = flag.String("enforced_version", "", "go version string to be enforced")
)

func main() {
	flag.Parse()
	if *enforced == "" {
		log.Fatalf("invalid -enforced_version specified.  failing.")
	}
	paths, err := collectModFiles(*dir)
	if err != nil {
		log.Fatalf("collectModFiles: %v", err)
	}

	var failCount, successCount int
	for _, p := range paths {
		if err := validateModFile(p, *enforced); err != nil {
			log.Printf("validation failed for %q: %v", p, err)
			failCount = failCount + 1
		} else {
			successCount = successCount + 1
		}
	}
	log.Printf("%d mod files failed, %d succeeded validation", failCount, successCount)
	if failCount > 0 {
		os.Exit(1)
	}
}

func validateModFile(path string, enforcedVersion string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mf, err := modfile.Parse(path, b, nil)
	if err != nil {
		return err
	}
	if mf.Go == nil {
		return fmt.Errorf("modfile missing Go version")
	}
	if mf.Go.Version != enforcedVersion {
		return fmt.Errorf("version mismatch, modfile set to %q", mf.Go.Version)
	}
	return nil
}

// traverse a directory and its subdirectories to collect the paths to all go.mod files.
func collectModFiles(rootDir string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() && filepath.Base(path) == "go.mod" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

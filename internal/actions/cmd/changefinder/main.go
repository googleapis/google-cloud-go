// Copyright 2021 Google LLC
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
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	rootDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		rootDir = os.Args[1]
	}
	log.Printf("Root dir: %q", rootDir)
	var modDirs []string
	// Find all external modules
	filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "internal" {
			return filepath.SkipDir
		}
		if d.Name() == "go.mod" {
			modDirs = append(modDirs, filepath.Dir(path))
		}
		return nil
	})

	// Find relative sub-module
	submodules := map[string]bool{}
	for _, dir := range modDirs {
		name, err := modName(dir)
		if err != nil {
			log.Fatalf("unable to lookup mod dir")
		}
		// Skip non-submodule
		if name == "cloud.google.com/go" {
			continue
		}
		name = strings.TrimPrefix(name, "cloud.google.com/go/")
		submodules[name] = true
	}

	c := exec.Command("git", "pull", "--tags")
	c.Dir = rootDir
	if err := c.Run(); err != nil {
		log.Fatalf("unable to pull tags: %v", err)
	}

	tag, err := latestTag(rootDir)
	if err != nil {
		log.Fatalf("unable to find tag: %v", err)
	}
	log.Printf("Latest release: %s", tag)

	c = exec.Command("git", "reset", "--hard", tag)
	c.Dir = rootDir
	if err := c.Run(); err != nil {
		log.Fatalf("unable to reset to tag: %v", err)
	}

	changes, err := gitFilesChanges(rootDir)
	if err != nil {
		log.Fatalf("unable to get files changed: %v", err)
	}

	updatedSubmodulesSet := map[string]bool{}
	for _, change := range changes {
		//TODO(codyoss): This will not work with nested sub-modules. If we add
		// those this needs to be updated.
		pkg := strings.Split(change, "/")[0]
		log.Printf("update to path: %s", pkg)
		if submodules[pkg] {
			updatedSubmodulesSet[pkg] = true
		}
	}

	updatedSubmodule := []string{}
	for mod := range updatedSubmodulesSet {
		updatedSubmodule = append(updatedSubmodule, mod)
	}
	b, err := json.Marshal(updatedSubmodule)
	if err != nil {
		log.Fatalf("unable to marshal submodules: %v", err)
	}
	fmt.Printf("::set-output name=submodules::%s", b)
}

func modName(dir string) (string, error) {
	c := exec.Command("go", "list", "-m")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return "", err
	}
	b = bytes.TrimSpace(b)
	return string(b), nil
}

func latestTag(dir string) (string, error) {
	c := exec.Command("git", "rev-list", "--tags", "--max-count=1")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return "", err
	}
	commit := string(bytes.TrimSpace(b))
	c = exec.Command("git", "describe", "--tags", commit)
	c.Dir = dir
	b, err = c.Output()
	if err != nil {
		return "", err
	}
	b = bytes.TrimSpace(b)
	return string(b), nil
}

func gitFilesChanges(dir string) ([]string, error) {
	c := exec.Command("git", "diff", "--name-only", "origin/main")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return nil, err
	}
	b = bytes.TrimSpace(b)
	log.Printf("Files changed:\n%s", b)
	return strings.Split(string(b), "\n"), nil
}

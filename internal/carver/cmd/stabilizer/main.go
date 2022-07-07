// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	parent := flag.String("parent", "", "The path to the parent module. Required.")
	child := flag.String("child", "", "The comma seperated relative paths to the child modules from the parent. Required.")
	flag.Parse()
	if *parent == "" || *child == "" {
		log.Fatalf("parent and child are required")
	}
	children := strings.Split(strings.TrimSpace(*child), ",")
	for _, child := range children {
		if err := stabilize(*parent, child); err != nil {
			log.Fatalf("unable to stabilize %v: %v", child, err)
		}
		tidy(filepath.Join(*parent, child))
	}
	tidy(*parent)
	printCommands(children)
}

func stabilize(parentPath, child string) error {
	// Remove file that is no longer needed
	tidyHackPath := filepath.Join(parentPath, child, "go_mod_tidy_hack.go")
	if err := os.Remove(tidyHackPath); err != nil {
		return fmt.Errorf("unable to remove file in %v: %v", child, err)
	}

	// Update CHANGES.md
	changesPath := filepath.Join(parentPath, child, "CHANGES.md")
	f, err := os.OpenFile(changesPath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("unable to open CHANGES.md in %v: %v", child, err)
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	ss := strings.Split(strings.TrimSpace(string(content)), "\n")
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for i, line := range ss {
		// Insert content after header
		if i == 2 {
			fmt.Fprint(f, "## 1.0.0\n\n")
			fmt.Fprint(f, "Stabilize GA surface.\n\n")
		}
		fmt.Fprintf(f, "%s\n", line)
	}
	return nil
}

func tidy(dir string) error {
	c := exec.Command("go", "mod", "tidy")
	c.Dir = dir
	return c.Run()
}

func printCommands(children []string) {
	fmt.Print("Tags for commit message:\n")
	for _, v := range children {
		fmt.Printf("- %s/v1.0.0\n", v)
	}
	fmt.Print("\nCommands to run:\n")
	for _, v := range children {
		fmt.Printf("git tag %s/v1.0.0 $COMMIT_SHA\n", v)
	}
	for _, v := range children {
		fmt.Printf("git push $REMOTE refs/tags/%s/v1.0.0\n", v)
	}
}

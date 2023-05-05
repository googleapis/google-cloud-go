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
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var (
	pullRequest = flag.Bool("pr", false, "indicates if the tool is run against a PR")
	dir         = flag.String("dir", "", "the root directory to evaluate")
	quiet       = flag.Bool("q", false, "quiet mode, minimal logging")
	// Only used in quiet mode, printed in the event of an error.
	logBuffer []string
)

func main() {
	flag.Parse()
	rootDir, err := os.Getwd()
	if err != nil {
		fatalE(err)
	}
	if *dir != "" {
		rootDir = *dir
	}
	logg("Root dir: %q", rootDir)

	submodules, err := mods(rootDir)
	if err != nil {
		fatalE(err)
	}

	if !*pullRequest {
		resetLatestTag(rootDir)
	}

	changes, err := gitFilesChanges(rootDir)
	if err != nil {
		fatal("unable to get files changed: %v", err)
	}

	modulesSeen := map[string]bool{}
	updatedSubmodules := []string{}
	for _, change := range changes {
		if strings.HasPrefix(change, "internal") {
			continue
		}
		submod, ok := owner(change, submodules)
		if !ok {
			logg("no module for: %s", change)
			continue
		}
		if _, seen := modulesSeen[submod]; !seen {
			logg("changes in submodule: %s", submod)
			updatedSubmodules = append(updatedSubmodules, submod)
			modulesSeen[submod] = true
		}
	}

	b, err := json.Marshal(updatedSubmodules)
	if err != nil {
		fatal("unable to marshal submodules: %v", err)
	}
	fmt.Printf("::set-output name=submodules::%s", b)
}

func resetLatestTag(rootDir string) {
	c := exec.Command("git", "pull", "--tags")
	c.Dir = rootDir
	if err := c.Run(); err != nil {
		fatal("unable to pull tags: %v", err)
	}

	tag, err := latestTag(rootDir)
	if err != nil {
		fatal("unable to find tag: %v", err)
	}
	logg("Latest release: %s", tag)

	c = exec.Command("git", "reset", "--hard", tag)
	c.Dir = rootDir
	if err := c.Run(); err != nil {
		fatal("unable to reset to tag: %v", err)
	}
}

func owner(file string, submodules []string) (string, bool) {
	submod := ""
	for _, mod := range submodules {
		if strings.HasPrefix(file, mod) && len(mod) > len(submod) {
			submod = mod
		}
	}

	return submod, submod != ""
}

func mods(dir string) (submodules []string, err error) {
	c := exec.Command("go", "list", "-m")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return submodules, err
	}
	list := strings.Split(strings.TrimSpace(string(b)), "\n")

	submodules = []string{}
	for _, mod := range list {
		// Skip non-submodule or internal submodules.
		if mod == "cloud.google.com/go" || strings.Contains(mod, "internal") {
			continue
		}
		logg("found module: %s", mod)
		mod = strings.TrimPrefix(mod, "cloud.google.com/go/")
		submodules = append(submodules, mod)
	}

	return submodules, nil
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
	logg("Files changed:\n%s", b)
	return strings.Split(string(b), "\n"), nil
}

// logg is a potentially quiet log.Printf.
func logg(format string, values ...interface{}) {
	if *quiet {
		logBuffer = append(logBuffer, fmt.Sprintf(format, values...))
		return
	}
	log.Printf(format, values...)
}

func fatalE(err error) {
	if *quiet {
		log.Print(strings.Join(logBuffer, "\n"))
	}
	log.Fatal(err)
}

func fatal(format string, values ...interface{}) {
	if *quiet {
		log.Print(strings.Join(logBuffer, "\n"))
	}
	log.Fatalf(format, values...)
}

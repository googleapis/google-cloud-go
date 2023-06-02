// Copyright 2023 Google LLC
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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/actions/changes"
	"cloud.google.com/go/internal/actions/logg"
)

var (
	dir       = flag.String("dir", "", "the root directory to evaluate")
	format    = flag.String("format", "plain", "output format, one of [plain|github], defaults to 'plain'")
	ghVarName = flag.String("gh-var", "submodules", "github format's variable name to set output for, defaults to 'submodules'.")
	base      = flag.String("base", "origin/main", "the base ref to compare to, defaults to 'origin/main'")
	cleanup   = flag.Bool("cleanup", false, "cleans up the baseline files created by apidiff")
	baselines []string
)

func main() {
	flag.BoolVar(&logg.Quiet, "q", false, "quiet mode, minimal logging")
	flag.Parse()
	rootDir, err := os.Getwd()
	if err != nil {
		logg.Fatal(err)
	}
	if *dir != "" {
		rootDir = *dir
	}
	logg.Printf("Root dir: %q", rootDir)

	updatedSubmoduleDirs, err := changes.GatherChangedModuleDirs(rootDir, *base)
	if err != nil {
		logg.Fatal(err)
	}
	if len(updatedSubmoduleDirs) == 0 {
		logg.Printf("No modules changed.\n")
		os.Exit(0)
	}

	c := exec.Command("git", "rev-parse", "HEAD")
	c.Dir = rootDir
	data, err := c.Output()
	if err != nil {
		logg.Fatal(err)
	}
	head := strings.TrimSpace(string(data))
	logg.Printf("HEAD is: %s\n", head)

	c = exec.Command("git", "checkout", *base)
	c.Dir = rootDir
	_, err = c.Output()
	if err != nil {
		logg.Fatal(err)
	}
	logg.Printf("checked out: %s\n", *base)

	for _, d := range updatedSubmoduleDirs {
		modDir := filepath.Join(rootDir, d)
		baseline := filepath.Join(modDir, fmt.Sprintf("%s.pkg", d))
		c = exec.Command("apidiff", "-w", baseline, "-m", ".")
		c.Dir = modDir
		_, err := c.Output()
		if err != nil {
			logg.Fatal(err)
		}
		logg.Printf("baseline written to: %s\n", baseline)
		baselines = append(baselines, baseline)
	}

	c = exec.Command("git", "checkout", head)
	c.Dir = rootDir
	data, err = c.CombinedOutput()
	if err != nil {
		logg.Fatalf("%s: %s", data, err)
	}
	logg.Printf("checked out HEAD: %s\n", head)

	var summary []string
	for _, d := range updatedSubmoduleDirs {
		modDir := filepath.Join(rootDir, d)
		baseline := filepath.Join(modDir, fmt.Sprintf("%s.pkg", d))
		c = exec.Command("apidiff", "-m", baseline, ".")
		c.Dir = modDir
		b, err := c.CombinedOutput()
		if err != nil {
			logg.Fatalf("error while diffing: %v", err)
		}
		raw := string(b)
		var findings []string
		for _, l := range strings.Split(raw, "\n") {
			if strings.HasPrefix(l, "Ignoring") || l == "" {
				continue
			}
			findings = append(findings, l)
		}
		if len(findings) > 0 {
			summary = append(summary, fmt.Sprintf("# %s", d))
			summary = append(summary, findings...)
		}
	}

	if len(summary) > 0 {
		s := strings.Join(summary, "\n")
		fmt.Println(s)
	}

	if *cleanup && len(baselines) > 0 {
		for _, b := range baselines {
			if err := os.Remove(b); err != nil {
				logg.Printf("Error cleaning up %s: %s\n", b, err)
			}
		}
	}
}

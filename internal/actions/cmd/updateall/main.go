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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"cloud.google.com/go/internal/actions/logg"
)

var (
	quiet         = flag.Bool("q", false, "optional, quiet mode, minimal logging, defaults to false")
	dep           = flag.String("dep", "", "required, the module dependency to update")
	version       = flag.String("version", "latest", "optional, the version for the go get command, defaults to 'latest'")
	noIndirect    = flag.Bool("no-indirect", false, "optional, ignores indirect deps, defaults to false")
	commitLvl     = flag.String("commit-level", "fix", "optional, nested commit conventional commits type for output, defaults to 'fix'")
	msg           = flag.String("msg", "", "optional, nested commit message for output, defaults to 'update <dep> to <version>'")
	indirectDep   *regexp.Regexp
	directDep     *regexp.Regexp
	commitLevel   string
	commitMsg     string
	nestedCommits strings.Builder
)

func main() {
	logg.Quiet = *quiet
	flag.Parse()
	if *dep == "" {
		logg.Fatalf("Missing required option: -dep=[module]")
	}
	if *version != "latest" {
		directDep = regexp.MustCompile(fmt.Sprintf(`%s %s`, *dep, *version))
	}
	if *noIndirect {
		indirectDep = regexp.MustCompile(fmt.Sprintf(`%s [\-\/\.a-zA-Z0-9]+ \/\/ indirect`, *dep))
	}
	commitLevel = *commitLvl
	commitMsg := *msg
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("update %s to %s", *dep, *version)
	}

	rootDir, err := os.Getwd()
	if err != nil {
		logg.Fatal(err)
	}
	logg.Printf("Root dir: %s", rootDir)

	modDirs, err := modDirs(rootDir)
	if err != nil {
		logg.Fatalf("error listing submodules: %v", err)
	}

	for _, m := range modDirs {
		modFile := filepath.Join(m, "go.mod")
		depends, err := dependson(modFile, *dep, *version)
		if err != nil {
			logg.Fatalf("error checking for dep: %v", err)
		}
		if !depends {
			continue
		}
		if err := update(m, *dep, *version, rootDir); err != nil {
			logg.Printf("(non-fatal) failed to update %s: %s", m, err)
		}
	}

	if !*quiet {
		fmt.Println(nestedCommits.String())
	}
}

func modDirs(dir string) (submodules []string, err error) {
	// Find all external modules
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "go.mod" {
			submodules = append(submodules, filepath.Dir(path))
		}
		return nil
	})
	return submodules, err
}

func dependson(mod, dep, version string) (bool, error) {
	b, err := os.ReadFile(mod)
	if err != nil {
		return false, err
	}
	content := string(b)
	target := fmt.Sprintf("%s ", dep)
	has := strings.Contains(content, target)
	eligible := version == "latest" || !directDep.MatchString(content)
	if *noIndirect {
		eligible = eligible && !indirectDep.MatchString(content)
	}

	return has && eligible, nil
}

func update(modDir, dep, version, rootDir string) error {
	c := exec.Command("go", "get", fmt.Sprintf("%s@%s", dep, version))
	c.Dir = modDir
	_, err := c.Output()
	if err != nil {
		return err
	}

	c = exec.Command("go", "mod", "tidy")
	c.Dir = modDir
	_, err = c.Output()
	if err != nil {
		return err
	}

	if !*quiet {
		modDir = strings.TrimSpace(modDir)
		scope := strings.TrimPrefix(modDir, rootDir+"/")
		if scope == modDir || scope == "" || scope == "main" || strings.Contains(scope, "internal/") {
			return nil
		}
		nestedCommits.WriteString("BEGIN_NESTED_COMMIT\n")
		nestedCommits.WriteString(fmt.Sprintf("%s(%s): %s\n", commitLevel, scope, commitMsg))
		nestedCommits.WriteString("END_NESTED_COMMIT\n")
	}

	return nil
}

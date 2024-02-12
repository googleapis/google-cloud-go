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
	"os"
	"os/exec"
	"path"
	"strings"

	"cloud.google.com/go/internal/actions/logg"
)

var (
	dir       = flag.String("dir", "", "the root directory to evaluate")
	format    = flag.String("format", "plain", "output format, one of [plain|github|commit], defaults to 'plain'")
	ghVarName = flag.String("gh-var", "submodules", "github format's variable name to set output for, defaults to 'submodules'.")
	base      = flag.String("base", "origin/main", "the base ref to compare to, defaults to 'origin/main'")
	// See https://git-scm.com/docs/git-diff#Documentation/git-diff.txt---diff-filterACDMRTUXB82308203
	filter          = flag.String("diff-filter", "", "the git diff filter to apply [A|C|D|M|R|T|U|X|B] - lowercase to exclude")
	pathFilter      = flag.String("path-filter", "", "filter commits by changes to target path(s)")
	contentPattern  = flag.String("content-regex", "", "regular expression to execute against contents of diff")
	commitMessage   = flag.String("commit-message", "", "message to use with the module in nested commit format")
	commitScope     = flag.String("commit-scope", "", "scope to use in commit message - only for format=commit")
	touch           = flag.Bool("touch", false, "touches the CHANGES.md file to elicit a submodule change - only works when used with -format=commit")
	includeInternal = flag.Bool("internal", false, "toggles inclusion of the internal modules")
)

func main() {
	flag.BoolVar(&logg.Quiet, "q", false, "quiet mode, minimal logging")
	flag.Parse()
	if *format == "commit" && (*commitMessage == "" || *commitScope == "") {
		logg.Fatalf("requested format=commit and missing commit-message or commit-scope")
	}
	if *touch && *format != "commit" {
		logg.Fatalf("requested modules be touched without using format=commit")
	}
	rootDir, err := os.Getwd()
	if err != nil {
		logg.Fatal(err)
	}
	if *dir != "" {
		rootDir = *dir
	}
	logg.Printf("Root dir: %q", rootDir)

	submodulesDirs, err := modDirs(rootDir)
	if err != nil {
		logg.Fatal(err)
	}

	changes, err := gitFilesChanges(rootDir)
	if err != nil {
		logg.Fatalf("unable to get files changed: %v", err)
	}

	modulesSeen := map[string]bool{}
	updatedSubmoduleDirs := []string{}
	for _, change := range changes {
		if strings.HasPrefix(change, "internal") && !*includeInternal {
			continue
		}
		submodDir, ok := owner(change, submodulesDirs)
		if !ok {
			logg.Printf("no module for: %s", change)
			continue
		}
		if _, seen := modulesSeen[submodDir]; !seen {
			logg.Printf("changes in submodule: %s", submodDir)
			updatedSubmoduleDirs = append(updatedSubmoduleDirs, submodDir)
			modulesSeen[submodDir] = true
			if *touch {
				if err := touchModule(rootDir, submodDir); err != nil {
					logg.Printf("error touching module %q: %v", submodDir, err)
				}
			}
		}
	}

	output(updatedSubmoduleDirs)
}

func output(s []string) error {
	switch *format {
	case "github":
		b, err := json.Marshal(s)
		if err != nil {
			logg.Fatalf("unable to marshal submodules: %v", err)
		}
		fmt.Printf("::set-output name=%s::%s", *ghVarName, b)
	case "commit":
		for _, m := range s {
			fmt.Println("BEGIN_NESTED_COMMIT")
			fmt.Printf("%s(%s):%s\n", *commitScope, m, *commitMessage)
			fmt.Println("END_NESTED_COMMIT")
		}
	case "plain":
		fallthrough
	default:
		fmt.Println(strings.Join(s, "\n"))
	}
	return nil
}

func owner(file string, submoduleDirs []string) (string, bool) {
	submod := ""
	for _, mod := range submoduleDirs {
		if strings.HasPrefix(file, mod) && len(mod) > len(submod) {
			submod = mod
		}
	}

	return submod, submod != ""
}

func modDirs(dir string) (submodulesDirs []string, err error) {
	c := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return submodulesDirs, err
	}
	// Skip the root mod
	list := strings.Split(strings.TrimSpace(string(b)), "\n")[1:]

	submodulesDirs = []string{}
	for _, modPath := range list {
		if strings.Contains(modPath, "internal") && !*includeInternal {
			continue
		}
		logg.Printf("found module: %s", modPath)
		modPath = strings.TrimPrefix(modPath, dir+"/")
		submodulesDirs = append(submodulesDirs, modPath)
	}

	return submodulesDirs, nil
}

func gitFilesChanges(dir string) ([]string, error) {
	args := []string{"diff", "--name-only"}
	if *filter != "" {
		args = append(args, "--diff-filter", *filter)
	}
	if *contentPattern != "" {
		args = append(args, "-G", *contentPattern)
	}
	args = append(args, *base)

	if *pathFilter != "" {
		args = append(args, "--", *pathFilter)
	}

	c := exec.Command("git", args...)
	logg.Printf(c.String())

	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return nil, err
	}
	b = bytes.TrimSpace(b)
	logg.Printf("Files changed:\n%s", b)
	return strings.Split(string(b), "\n"), nil
}

func touchModule(root, mod string) error {
	c := exec.Command("echo")
	logg.Printf(c.String())

	f, err := os.OpenFile(path.Join(root, mod, "CHANGES.md"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	c.Stdout = f

	err = c.Run()
	if err != nil {
		return err
	}

	logg.Printf("Module touched: %s", mod)
	return nil
}

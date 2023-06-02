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

package changes

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"cloud.google.com/go/internal/actions/logg"
)

// GatherChangedModuleDirs collects the changes to the module(s) in the given directory compared
// to the given base ref.
func GatherChangedModuleDirs(dir, base string) ([]string, error) {
	submodulesDirs, err := modDirs(dir)
	if err != nil {
		return nil, err
	}

	changes, err := gitFilesChanges(dir, base)
	if err != nil {
		return nil, fmt.Errorf("unable to get files changed: %w", err)
	}

	modulesSeen := map[string]bool{}
	updatedSubmoduleDirs := []string{}
	for _, change := range changes {
		if strings.HasPrefix(change, "internal") {
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
		}
	}

	return updatedSubmoduleDirs, nil
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
		// Skip non-submodule or internal submodules.
		if strings.Contains(modPath, "internal") {
			continue
		}
		logg.Printf("found module: %s", modPath)
		modPath = strings.TrimPrefix(modPath, dir+"/")
		submodulesDirs = append(submodulesDirs, modPath)
	}

	return submodulesDirs, nil
}

func gitFilesChanges(dir, base string) ([]string, error) {
	c := exec.Command("git", "diff", "--name-only", base, ":!*go.mod", ":!*go.sum", ":!.github")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return nil, err
	}
	b = bytes.TrimSpace(b)
	logg.Printf("Files changed:\n%s", b)
	return strings.Split(string(b), "\n"), nil
}

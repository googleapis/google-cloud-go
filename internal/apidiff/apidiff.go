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

//go:build linux || darwin
// +build linux darwin

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	osexec "os/exec"
	"path"
	"strings"
)

// TODO(noahdietz): remove this once the fix in golang.org/x/tools is released.
// https://github.com/golang/go/issues/44796
const ignored = "- MaxPublishRequestBytes: value changed from 0.000582077 to 10000000"
const rootMod = "cloud.google.com/go"

var verbose bool
var mod string
var base string

func init() {
	flag.StringVar(&mod, "mod", "", "import path of a specific module to diff")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose command logging")
	flag.StringVar(&base, "base", "", "path to a copy of google-cloud-go to use as the base")
}

func main() {
	flag.Parse()

	head, err := exec("git", "log", "-2")
	if err != nil {
		log.Fatalln(err)
	}
	if checkAllowBreakingChange(head) {
		return
	}

	root, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = exec("go", "install", "golang.org/x/exp/cmd/apidiff@latest")
	if err != nil {
		log.Fatalln(err)
	}

	if base == "" {
		temp, err := ioutil.TempDir("/tmp", "google-cloud-go-*")
		if err != nil {
			log.Fatalln(err)
		}
		defer os.RemoveAll(temp)

		_, err = exec("git", "clone", "https://github.com/googleapis/google-cloud-go", temp)
		if err != nil {
			log.Fatalln(err)
		}
	}

	diffs, diffingErrs, err := diffModules(root, base)
	if err != nil {
		log.Fatalln(err)
	}

	if len(diffingErrs) > 0 {
		fmt.Fprintln(os.Stderr, "The following packages encountered errors:")
		for imp, err := range diffingErrs {
			fmt.Fprintf(os.Stderr, "%s: %s\n", imp, err)
		}
	}

	if len(diffs) > 0 {
		fmt.Fprintln(os.Stderr, "The following breaking changes were found:")
		for imp, d := range diffs {
			fmt.Fprintf(os.Stderr, "%s:\n%s\n", imp, d)
		}
		os.Exit(1)
	}
}

func diffModules(root, baseDir string) (map[string]string, map[string]error, error) {
	diffs := map[string]string{}
	issues := map[string]error{}

	m, err := mods()
	if err != nil {
		return nil, nil, err
	}

	for _, modDir := range m {
		modPkg := strings.TrimPrefix(modDir, "./")
		modPkg = strings.TrimSuffix(modPkg, "/go.mod")
		modImp := rootMod + "/" + modPkg
		modAbsDir := path.Join(root, modPkg)

		if mod != "" && modImp != mod {
			continue
		}

		baseModDir := path.Join(baseDir, modPkg)

		subp, err := subpackages(baseModDir)
		if err != nil {
			return nil, nil, err
		}

		for _, sub := range subp {
			if sub == "." {
				continue
			}
			subImp := modImp + strings.TrimPrefix(sub, ".")

			// Create apidiff base from repo remote HEAD.
			base, err := writeBase(baseModDir, sub)
			if err != nil {
				issues[subImp] = err
				continue
			}

			// Diff the current checked out change against remote HEAD base.
			out, err := diff(modAbsDir, sub, base)
			if err != nil {
				issues[subImp] = err
				continue
			}

			if out != "" && out != ignored {
				diffs[subImp] = out
			}
		}
	}

	return diffs, issues, nil
}

func writeBase(baseModDir, subPkg string) (string, error) {
	base := path.Join(baseModDir, "pkg.main")
	_, err := execDir(baseModDir, "apidiff", "-w", base, subPkg)

	return base, err
}

func diff(modDir, subpkg, base string) (string, error) {
	out, err := execDir(modDir, "apidiff", "-allow-internal", "-incompatible", base, subpkg)
	if err != nil {
		return "", err
	}

	return out, err
}

func checkAllowBreakingChange(commit string) bool {
	if strings.Contains(commit, "BREAKING CHANGE:") {
		log.Println("Not running apidiff because description contained tag BREAKING_CHANGE.")
		return true
	}

	split := strings.Split(commit, "\n")
	for _, s := range split {
		if strings.Contains(s, "!:") || strings.Contains(s, "!(") {
			log.Println("Not running apidiff because description contained breaking change indicator '!'.")
			return true
		}
	}

	return false
}

func mods() ([]string, error) {
	out, err := exec("find", ".", "-name", "go.mod", "-not", "-path", "./internal/*", "-not", "-path", "./go.mod")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

func subpackages(base string) ([]string, error) {
	out, err := execDir(base, "find", ".", "-mindepth", "1", "-name", "doc.go", "-exec", "dirname", "{}", "\\", ";")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

func execDir(dir, cmd string, args ...string) (string, error) {
	if verbose {
		log.Printf("+ %s %s\n", cmd, strings.Join(args, " "))
	}
	c := osexec.Command(cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func exec(cmd string, args ...string) (string, error) {
	if verbose {
		log.Printf("+ %s %s\n", cmd, strings.Join(args, " "))
	}
	out, err := osexec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

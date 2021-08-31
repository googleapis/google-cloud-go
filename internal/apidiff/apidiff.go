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
	"encoding/json"
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

var repoMetadataPath string
var verbose bool
var gapic string

func init() {
	flag.StringVar(&repoMetadataPath, "repo-metadata", "", "path to a repo-metadata-full JSON file [required]")
	flag.StringVar(&gapic, "gapic", "", "import path of a specific GAPIC to diff")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose command logging")
}

func main() {
	flag.Parse()
	if repoMetadataPath == "" {
		log.Fatalln("Missing required flag: -repo-metadata")
	}

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

	f, err := os.Open(repoMetadataPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()

	var m manifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		log.Fatalln(err)
	}

	_, err = exec("go", "install", "golang.org/x/exp/cmd/apidiff@latest")
	if err != nil {
		log.Fatalln(err)
	}

	temp, err := ioutil.TempDir("/tmp", "google-cloud-go-*")
	if err != nil {
		log.Fatalln(err)
	}
	defer os.RemoveAll(temp)

	_, err = exec("git", "clone", "https://github.com/googleapis/google-cloud-go", temp)
	if err != nil {
		log.Fatalln(err)
	}

	diffs, diffingErrs, err := diffModules(root, temp, m)
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

// manifestEntry is used for JSON marshaling in manifest.
// Copied from internal/gapicgen/generator/gapics.go.
type manifestEntry struct {
	DistributionName  string `json:"distribution_name"`
	Description       string `json:"description"`
	Language          string `json:"language"`
	ClientLibraryType string `json:"client_library_type"`
	DocsURL           string `json:"docs_url"`
	ReleaseLevel      string `json:"release_level"`
}

type manifest map[string]manifestEntry

func diffModules(root, baseDir string, m manifest) (map[string]string, map[string]error, error) {
	diffs := map[string]string{}
	issues := map[string]error{}

	for imp, entry := range m {
		if gapic != "" && imp != gapic {
			continue
		}

		// Prepare module directory paths relative to the repo root.
		pkg := strings.TrimPrefix(imp, rootMod+"/")
		baseModDir := baseDir
		modDir := root

		// Manual clients are also submodules, so we need to run apidiff in the
		// submodule.
		if entry.ClientLibraryType == "manual" {
			baseModDir = path.Join(baseModDir, pkg)
			modDir = path.Join(modDir, pkg)
		}

		// Create apidiff base from repo remote HEAD.
		base, err := writeBase(m, baseModDir, imp, pkg)
		if err != nil {
			issues[imp] = err
			continue
		}

		// Diff the current checked out change against remote HEAD base.
		out, err := diff(m, modDir, imp, pkg, base)
		if err != nil {
			issues[imp] = err
			continue
		}

		if out != "" && out != ignored {
			diffs[imp] = out
		}
	}

	return diffs, issues, nil
}

func writeBase(m manifest, baseModDir, imp, pkg string) (string, error) {
	if err := cd(baseModDir); err != nil {
		return "", err
	}

	base := path.Join(baseModDir, "pkg.master")
	out, err := exec("apidiff", "-w", base, imp)
	if err != nil && !isSubModErr(out) {
		return "", err
	}

	// If there was an issue with loading a submodule, change into that
	// submodule directory and try again.
	if isSubModErr(out) {
		parent := manualParent(m, imp)
		if parent == pkg {
			return "", fmt.Errorf("unable to find parent module for %q", imp)
		}
		if err := cd(parent); err != nil {
			return "", err
		}
		out, err := exec("apidiff", "-w", base, imp)
		if err != nil {
			return "", fmt.Errorf("%s: %s", err, out)
		}
	}
	return base, nil
}

func diff(m manifest, modDir, imp, pkg, base string) (string, error) {
	if err := cd(modDir); err != nil {
		return "", err
	}
	out, err := exec("apidiff", "-incompatible", base, imp)
	if err != nil && !isSubModErr(out) {
		return "", err
	}
	if isSubModErr(out) {
		parent := manualParent(m, imp)
		if parent == pkg {
			return "", fmt.Errorf("unable to find parent module for %q", imp)
		}
		if err := cd(parent); err != nil {
			return "", err
		}
		out, err = exec("apidiff", "-incompatible", base, imp)
		if err != nil {
			return "", fmt.Errorf("%s: %s", err, out)
		}
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

func manualParent(m manifest, imp string) string {
	pkg := strings.TrimPrefix(imp, rootMod)
	split := strings.Split(pkg, "/")

	mod := rootMod
	for _, seg := range split {
		mod = path.Join(mod, seg)
		if parent, ok := m[mod]; ok && parent.ClientLibraryType == "manual" {
			return strings.TrimPrefix(mod, rootMod+"/")
		}
	}

	return pkg
}

func isSubModErr(msg string) bool {
	return strings.Contains(msg, "missing") || strings.Contains(msg, "required")
}

func cd(dir string) error {
	if verbose {
		log.Printf("+ cd %s\n", dir)
	}
	return os.Chdir(dir)
}

func exec(cmd string, args ...string) (string, error) {
	if verbose {
		log.Printf("+ %s %s\n", cmd, strings.Join(args, " "))
	}
	out, err := osexec.Command(cmd, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

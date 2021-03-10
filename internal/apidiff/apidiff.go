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

// +build linux darwin

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
)

// TODO(noahdietz): remove this once the fix in golang.org/x/tools is released.
// https://github.com/golang/go/issues/44796
const ignored = "- MaxPublishRequestBytes: value changed from 0.000582077 to 10000000"
const rootMod = "cloud.google.com/go"

var repoMetadataPath string
var verbose bool

func init() {
	flag.StringVar(&repoMetadataPath, "repo-metadata", "", "path to a repo-metadata-full JSON file [required]")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose command logging")
}

func main() {
	flag.Parse()
	if repoMetadataPath == "" {
		log.Fatalln("Missing required flag: -repo-metadata")
	}

	run, err := shouldRun()
	if err != nil {
		log.Fatalln(err)
	}
	if !run {
		log.Println("Not running apidiff because description contained tag BREAKING_CHANGE_ACCEPTABLE.")
		return
	}

	d, err := newDiffer()
	if err != nil {
		log.Fatalln(err)
	}

	err = d.loadManifest()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = execE("go", "install", "golang.org/x/exp/cmd/apidiff@latest")
	if err != nil {
		log.Fatalln(err)
	}

	err = d.diffModules()
	if err != nil {
		log.Fatalln(err)
	}

	hadErrors := len(d.errs) > 0
	if hadErrors {
		fmt.Fprintln(os.Stderr, "The following packages encountered errors:")
		for imp, err := range d.errs {
			fmt.Fprintf(os.Stderr, "%s: %s\n", imp, err)
		}
	}

	hadDiffs := len(d.results) > 0
	if hadDiffs {
		fmt.Fprintln(os.Stderr, "The following breaking changes were found:")
		for imp, d := range d.results {
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

type differ struct {
	root     string
	manifest map[string]manifestEntry
	results  map[string]string
	errs     map[string]error
}

func newDiffer() (*differ, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &differ{
		root:    root,
		results: map[string]string{},
		errs:    map[string]error{},
	}, nil
}

func (d *differ) diffModules() error {
	goCloud, err := cloneGoCloud()
	if err != nil {
		return err
	}
	defer os.RemoveAll(goCloud)

	for imp, entry := range d.manifest {
		// Only diff stable clients.
		if entry.ReleaseLevel != "ga" {
			continue
		}

		// Prepare module directory paths relative to the repo root.
		pkg := strings.TrimPrefix(imp, rootMod+"/")
		baseModDir := goCloud
		modDir := d.root

		// Manual clients are also submodules, so we need to run apidiff in the
		// submodule.
		if entry.ClientLibraryType == "manual" {
			baseModDir = path.Join(baseModDir, pkg)
			modDir = path.Join(modDir, pkg)
		}

		// Create apidiff base from repo remote HEAD.
		base, err := d.writeBase(baseModDir, imp, pkg)
		if err != nil {
			d.errs[imp] = err
			continue
		}

		// Diff the current checked out change against remote HEAD base.
		out, err := d.diff(modDir, imp, pkg, base)
		if err != nil {
			d.errs[imp] = err
			continue
		}

		if out != "" && out != ignored {
			d.results[imp] = out
		}
	}

	return nil
}

func (d *differ) writeBase(baseModDir, imp, pkg string) (string, error) {
	if err := cd(baseModDir); err != nil {
		return "", err
	}

	base := path.Join(baseModDir, "pkg.master")
	out, err := execE("apidiff", "-w", base, imp)
	if err != nil && !isSubModErr(out) {
		return "", err
	}

	// If there was an issue with loading a submodule, change into that
	// submodule directory and try again.
	if isSubModErr(out) {
		parent := d.manualParent(imp)
		if parent == pkg {
			return "", fmt.Errorf("unable to find parent module for %q", imp)
		}
		if err := cd(parent); err != nil {
			return "", err
		}
		out, err := execE("apidiff", "-w", base, imp)
		if err != nil {
			return "", fmt.Errorf("%s: %s", err, out)
		}
	}
	return base, nil
}

func (d *differ) diff(modDir, imp, pkg, base string) (string, error) {
	if err := cd(modDir); err != nil {
		return "", err
	}
	out, err := execE("apidiff", "-incompatible", base, imp)
	if err != nil && !isSubModErr(out) {
		return "", err
	}
	if isSubModErr(out) {
		parent := d.manualParent(imp)
		if parent == pkg {
			return "", fmt.Errorf("unable to find parent module for %q", imp)
		}
		if err := cd(parent); err != nil {
			return "", err
		}
		out, err = execE("apidiff", "-w", base, imp)
		if err != nil {
			return "", fmt.Errorf("%s: %s", err, out)
		}
	}

	return out, err
}

func (d *differ) loadManifest() error {
	f, err := os.Open(repoMetadataPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewDecoder(f).Decode(&d.manifest)
}

func (d *differ) manualParent(imp string) string {
	pkg := strings.TrimPrefix(imp, rootMod)
	split := strings.Split(pkg, "/")

	mod := rootMod
	for _, seg := range split {
		mod = path.Join(mod, seg)
		if parent, ok := d.manifest[mod]; ok && parent.ClientLibraryType == "manual" {
			return strings.TrimPrefix(mod, rootMod+"/")
		}
	}

	return pkg
}

func shouldRun() (bool, error) {
	head, err := execE("git", "log", "-1")
	if err != nil {
		return false, err
	}
	return !strings.Contains(head, "BREAKING_CHANGE"), nil
}

func cloneGoCloud() (string, error) {
	temp, err := ioutil.TempDir("/tmp", "google-cloud-go-*")
	if err != nil {
		return "", err
	}
	_, err = execE("git", "clone", "https://github.com/googleapis/google-cloud-go", temp)
	return temp, err
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

func execE(cmd string, args ...string) (string, error) {
	if verbose {
		log.Printf("+ %s %s\n", cmd, strings.Join(args, " "))
	}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

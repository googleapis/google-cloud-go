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

// Package gocmd provides helers for invoking Go tooling.
package gocmd

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/gapicgen/execv"
)

var (
	// ErrBuildConstraint is returned when the Go command returns this error.
	ErrBuildConstraint error = errors.New("build constraints exclude all Go files")
)

// ModTidy tidies go.mod file in the specified directory.
func ModTidy(dir string) error {
	c := execv.Command("go", "mod", "tidy")
	c.Dir = dir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// ModTidyAll tidies all mod files from the specified root directory.
func ModTidyAll(dir string) error {
	log.Printf("[%s] finding all modules", dir)
	var modDirs []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "go.mod" {
			modDirs = append(modDirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, modDir := range modDirs {
		if err := ModTidy(modDir); err != nil {
			return err
		}
	}
	return nil
}

// ListModName finds a modules name for a given directory.
func ListModName(dir string) (string, error) {
	modC := execv.Command("go", "list", "-m")
	modC.Dir = dir
	mod, err := modC.Output()
	return string(mod), err
}

// ListModDirName finds the directory in which the module resides. Returns
// ErrBuildConstraint if all files in a module are constrained.
func ListModDirName(dir string) (string, error) {
	var out []byte
	var err error
	c := execv.Command("go", "list", "-f", "'{{.Module.Dir}}'")
	c.Dir = dir
	if out, err = c.Output(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(ee.Stderr), "build constraints exclude all Go files") {
				return "", ErrBuildConstraint
			}
		}
		return "", err
	}
	return strings.Trim(strings.TrimSpace(string(out)), "'"), nil
}

// Build attempts to build all packages recursively from the given directory.
func Build(dir string) error {
	log.Println("building generated code")
	c := execv.Command("go", "build", "./...")
	c.Dir = dir
	if _, err := c.Output(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			log.Printf("Error Output: %s", ee.Stderr)
		}
		return err
	}
	return nil
}

// Vet runs linters on all .go files recursively from the given directory.
func Vet(dir string) error {
	log.Println("vetting generated code")
	c := execv.Command("goimports", "-w", ".")
	c.Dir = dir
	if err := c.Run(); err != nil {
		return err
	}

	c = execv.Command("gofmt", "-s", "-d", "-w", "-l", ".")
	c.Dir = dir
	return c.Run()
}

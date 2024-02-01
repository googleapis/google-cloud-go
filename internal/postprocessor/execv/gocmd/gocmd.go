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

	"cloud.google.com/go/internal/postprocessor/execv"
)

var (
	// ErrBuildConstraint is returned when the Go command returns this error.
	ErrBuildConstraint error = errors.New("build constraints exclude all Go files")
)

// ModInit creates a new module in the specified directory.
func ModInit(dir, importPath, goVersion string) error {
	c := execv.Command("go", "mod", "init", importPath)
	c.Dir = dir
	if err := c.Run(); err != nil {
		return err
	}

	c2 := execv.Command("go", "get", fmt.Sprintf("go@%s", goVersion))
	c2.Dir = dir
	return c2.Run()
}

// ModTidy tidies go.mod file in the specified directory.
func ModTidy(dir string) error {
	c := execv.Command("go", "mod", "tidy")
	c.Dir = dir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")),
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

// ListModName finds a module's name for a given directory.
func ListModName(dir string) (string, error) {
	modC := execv.Command("go", "list", "-m")
	modC.Dir = dir
	modC.Env = []string{"GOWORK=off"}
	mod, err := modC.Output()
	return strings.TrimSpace(string(mod)), err
}

// Build attempts to build all packages recursively from the given directory.
func Build(dir string) error {
	log.Println("building generated code")
	c := execv.Command("go", "build", "./...")
	c.Dir = dir
	if _, err := c.Output(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
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

// CurrentMod returns the module name of the provided directory.
func CurrentMod(dir string) (string, error) {
	log.Println("detecting current module")
	c := execv.Command("go", "list", "-m")
	c.Dir = dir
	c.Env = []string{"GOWORK=off"}
	var out []byte
	var err error
	if out, err = c.Output(); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// EditReplace edits a module dependency with a local reference.
func EditReplace(dir, mod, modPath string) error {
	log.Printf("%s: editing dependency %q", dir, mod)
	c := execv.Command("go", "mod", "edit", "-replace", fmt.Sprintf("%s=%s", mod, modPath))
	c.Dir = dir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")),
	}
	return c.Run()
}

// WorkUse updates the go.work file for added modules.
func WorkUse(dir string) error {
	c := execv.Command("go", "work", "use", "-r", ".")
	c.Dir = dir
	return c.Run()
}

// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// main is the entrypoint for the stategen tool, which updates a Librarian state.yaml
// file to include the specified modules. The first argument is a path to the repository
// root; all subsequent arguments are module names.
func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("GOOGLE_SDK_GO_LOGGING_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))
	slog.Info("stategen: invoked", "args", os.Args)
	if err := run(os.Args[1:]); err != nil {
		slog.Error("stategen: failed", "error", err)
		os.Exit(1)
	}
	slog.Info("stategen: finished successfully")
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("stategen: expected a root directory and at least one module")
	}
	repoRoot := args[0]
	stateFilePath := filepath.Join(repoRoot, ".librarian/state.yaml")
	state, err := parseLibrarianState(stateFilePath)
	if err != nil {
		return err
	}
	for _, moduleName := range args[1:] {
		if stateContainsModule(state, moduleName) {
			slog.Info("skipping existing module", "module", moduleName)
			continue
		}
		err = addModule(repoRoot, state, moduleName)
		if err != nil {
			return err
		}
	}
	return saveLibrarianState(stateFilePath, state)
}

func stateContainsModule(state *LibrarianState, moduleName string) bool {
	for _, library := range state.Libraries {
		if library.ID == moduleName {
			return true
		}
	}
	return false
}

func addModule(repoRoot string, state *LibrarianState, moduleName string) error {
	slog.Info("adding module", "module", moduleName)
	moduleRoot := filepath.Join(repoRoot, moduleName)

	// Start off with the basics which need
	library := &LibraryState{
		ID: moduleName,
		SourceRoots: []string{
			moduleName,
			"internal/generated/snippets/" + moduleName,
		},
		RemoveRegex: []string{
			moduleName + "/README\\.md",
			moduleName + "/go\\.mod",
			moduleName + "/go\\.sum",
			moduleName + "/internal/version\\.go",
			"internal/generated/snippets/" + moduleName,
		},
	}

	version, err := loadVersion(moduleRoot)
	if err != nil {
		return err
	}
	library.Version = version

	err = addAPIProtoPaths(repoRoot, moduleName, library)
	if err != nil {
		return err
	}

	err = addGeneratedCodeRemovals(repoRoot, moduleRoot, library)
	if err != nil {
		return err
	}

	state.Libraries = append(state.Libraries, library)
	return nil
}

// addAPIProtoPaths walks the generates snippets directory to find the API proto paths for the library.
func addAPIProtoPaths(repoRoot, moduleName string, library *LibraryState) error {
	return filepath.WalkDir(filepath.Join(repoRoot, "internal/generated/snippets/"+moduleName), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if match, _ := filepath.Match("snippet_metadata.*.json", d.Name()); match {
			parts := strings.Split(d.Name(), ".")
			parts = parts[1 : len(parts)-1]
			api := &API{
				Path: strings.Join(parts, "/"),
			}
			library.APIs = append(library.APIs, api)
		}
		return nil
	})
}

// addApiPaths walk the module source directory to find the files to remove.
func addGeneratedCodeRemovals(repoRoot, moduleRoot string, library *LibraryState) error {
	return filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "apiv") {
			return nil
		}
		repoRelativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		apiParts := strings.Split(path, "/")
		protobufDir := apiParts[len(apiParts)-2] + "pb"
		generatedPaths := []string{
			"[^/]*_client\\.go",
			"[^/]*_client_example_go123_test\\.go",
			"[^/]*_client_example_test\\.go",
			"auxiliary\\.go",
			"auxiliary_go123\\.go",
			"doc\\.go",
			"gapic_metadata\\.json",
			"helpers\\.go",
			"version\\.go",
			protobufDir,
		}
		for _, generatedPath := range generatedPaths {
			library.RemoveRegex = append(library.RemoveRegex, repoRelativePath+"/"+generatedPath)
		}
		return nil
	})
}

func loadVersion(moduleRoot string) (string, error) {
	// Load internal/version.go to figure out the existing version.
	versionPath := filepath.Join(moduleRoot, "internal/version.go")
	versionBytes, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(versionBytes), "\n")
	lastLine := lines[len(lines)-1]
	// If the actual last line is empty, use the previous one instead.
	if lastLine == "" {
		lastLine = lines[len(lines)-2]
	}
	if !strings.HasPrefix(lastLine, "const Version") {
		return "", fmt.Errorf("stategen: version file not in expected format for module: %s; %s", versionPath, lastLine)
	}

	versionParts := strings.Split(lastLine, "\"")
	if len(versionParts) != 3 {
		return "", fmt.Errorf("stategen: last line of version file not in expected format for module: %s", versionPath)
	}
	return versionParts[1], nil
}

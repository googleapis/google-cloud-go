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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

	postProcessorConfigPath := filepath.Join(repoRoot, "internal/postprocessor/config.yaml")
	ppc, err := loadPostProcessorConfig(postProcessorConfigPath)
	if err != nil {
		return err
	}

	googleapisCommit, err := findLatestGoogleapisCommit()
	if err != nil {
		return err
	}

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
		if err := addModule(repoRoot, ppc, state, moduleName, googleapisCommit); err != nil {
			return err
		}
	}
	return saveLibrarianState(stateFilePath, state)
}

func findLatestGoogleapisCommit() (string, error) {
	// We don't need authentication for this API call, fortunately.
	resp, err := http.Get("https://api.github.com/repos/googleapis/googleapis/branches/master")
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch branch metadata for googleapis: %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var branch GitHubBranch
	if err := json.Unmarshal(respBody, &branch); err != nil {
		return "", err
	}
	hash := branch.Commit.Hash
	if hash == "" {
		return "", errors.New("failed to fetch hash from GitHub API response")
	}
	slog.Info("Fetched googleapis head commit", "hash", hash)
	return hash, nil
}

func stateContainsModule(state *LibrarianState, moduleName string) bool {
	for _, library := range state.Libraries {
		if library.ID == moduleName {
			return true
		}
	}
	return false
}

func addModule(repoRoot string, ppc *postProcessorConfig, state *LibrarianState, moduleName, googleapisCommit string) error {
	slog.Info("adding module", "module", moduleName)
	moduleRoot := filepath.Join(repoRoot, moduleName)

	// Start off with the basics which need
	library := &LibraryState{
		ID:                  moduleName,
		LastGeneratedCommit: googleapisCommit,
		SourceRoots: []string{
			moduleName,
		},
		TagFormat: "{id}/v{version}",
	}

	version, err := loadVersion(moduleRoot)
	if err != nil {
		return err
	}
	library.Version = version

	addAPIProtoPaths(ppc, moduleName, library)

	if len(library.APIs) > 0 {
		library.SourceRoots = append(library.SourceRoots, "internal/generated/snippets/"+moduleName)
		library.RemoveRegex = append(library.RemoveRegex, "^internal/generated/snippets/"+moduleName+"/")
		// Probably irrelevant after the first release, but changes within the snippets aren't release-relevant;
		// for the first release after onboarding, we will see an OwlBot commit updating snippet metadata with the
		// final release-please-based commit, and we don't want to use that.
		library.ReleaseExcludePaths = append(library.ReleaseExcludePaths, "internal/generated/snippets/"+moduleName+"/")
	}

	if err := addGeneratedCodeRemovals(repoRoot, moduleRoot, library); err != nil {
		return err
	}

	state.Libraries = append(state.Libraries, library)
	return nil
}

// addAPIProtoPaths uses the legacy post-processor config to determine which API paths contribute
// to the specified module.
func addAPIProtoPaths(ppc *postProcessorConfig, moduleName string, library *LibraryState) {
	importPrefix := "cloud.google.com/go/" + moduleName + "/"

	for _, serviceConfig := range ppc.ServiceConfigs {
		if strings.HasPrefix(serviceConfig.ImportPath, importPrefix) {
			api := &API{
				Path: serviceConfig.InputDirectory,
			}
			library.APIs = append(library.APIs, api)
		}
	}
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
		protobufDir := apiParts[len(apiParts)-2] + "pb/.*"
		generatedPaths := []string{
			"[^/]*_client\\.go",
			"[^/]*_client_example_go123_test\\.go",
			"[^/]*_client_example_test\\.go",
			"auxiliary\\.go",
			"auxiliary_go123\\.go",
			"doc\\.go",
			"gapic_metadata\\.json",
			"helpers\\.go",
			protobufDir,
		}
		for _, generatedPath := range generatedPaths {
			library.RemoveRegex = append(library.RemoveRegex, "^"+repoRelativePath+"/"+generatedPath+"$")
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

// GitHubBranch is the representation of a repository branch as returned by the GitHub
// API. We only need the commit.
type GitHubBranch struct {
	// Commit is the commit at the head of the branch
	Commit GitHubCommit `json:"commit"`
}

// GitHubCommit is the representation of a commit as returned by the GitHub
// API. We only need the SHA.
type GitHubCommit struct {
	// Hash is the SHA-256 hash of the commit
	Hash string `json:"sha"`
}

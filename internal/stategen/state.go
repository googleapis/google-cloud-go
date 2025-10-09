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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Copied from https://github.com/googleapis/librarian/blob/main/internal/config/state.go

// LibrarianState defines the contract for the state.yaml file.
type LibrarianState struct {
	// The name and tag of the generator image to use. tag is required.
	Image string `yaml:"image" json:"image"`
	// A list of library configurations.
	Libraries []*LibraryState `yaml:"libraries" json:"libraries"`
}

// LibraryState represents the state of a single library within state.yaml.
type LibraryState struct {
	// A unique identifier for the library, in a language-specific format.
	// A valid ID should not be empty and only contains alphanumeric characters, slashes, periods, underscores, and hyphens.
	ID string `yaml:"id" json:"id"`
	// The last released version of the library, following SemVer.
	Version string `yaml:"version" json:"version"`
	// The commit hash from the API definition repository at which the library was last generated.
	LastGeneratedCommit string `yaml:"last_generated_commit" json:"last_generated_commit"`
	// The changes from the language repository since the library was last released.
	// This field is ignored when writing to state.yaml.
	Changes []*Change `yaml:"-" json:"changes,omitempty"`
	// A list of APIs that are part of this library.
	APIs []*API `yaml:"apis" json:"apis"`
	// A list of directories in the language repository where Librarian contributes code.
	SourceRoots []string `yaml:"source_roots" json:"source_roots"`
	// A list of regular expressions for files and directories to preserve during the copy and remove process.
	PreserveRegex []string `yaml:"preserve_regex" json:"preserve_regex"`
	// A list of regular expressions for files and directories to remove before copying generated code.
	// If not set, this defaults to the `source_roots`.
	// A more specific `preserve_regex` takes precedence.
	RemoveRegex []string `yaml:"remove_regex" json:"remove_regex"`
	// Path of commits to be excluded from parsing while calculating library changes.
	// If all files from commit belong to one of the paths it will be skipped.
	ReleaseExcludePaths []string `yaml:"release_exclude_paths,omitempty" json:"release_exclude_paths,omitempty"`
	// Specifying a tag format allows librarian to honor this format when creating
	// a tag for the release of the library. The replacement values of {id} and {version}
	// permitted to reference the values configured in the library. If not specified
	// the assumed format is {id}-{version}. e.g., {id}/v{version}.
	TagFormat string `yaml:"tag_format,omitempty" json:"tag_format,omitempty"`
	// Whether including this library in a release.
	// This field is ignored when writing to state.yaml.
	ReleaseTriggered bool `yaml:"-" json:"release_triggered,omitempty"`
	// An error message from the docker response.
	// This field is ignored when writing to state.yaml.
	ErrorMessage string `yaml:"-" json:"error,omitempty"`
}

// API represents an API that is part of a library.
type API struct {
	// The path to the API, relative to the root of the API definition repository (e.g., "google/storage/v1").
	Path string `yaml:"path" json:"path"`
	// The name of the service config file, relative to the API `path`.
	ServiceConfig string `yaml:"service_config" json:"service_config"`
	// The status of the API, one of "new" or "existing".
	// This field is ignored when writing to state.yaml.
	Status string `yaml:"-" json:"status"`
}

// Change represents the changelog of a library.
type Change struct {
	// The type of the change, should be one of the conventional type.
	Type string `yaml:"type" json:"type"`
	// The summary of the change.
	Subject string `yaml:"subject" json:"subject"`
	// The body of the change.
	Body string `yaml:"body" json:"body"`
	// The Changelist number in piper associated with this change.
	ClNum string `yaml:"piper_cl_number" json:"piper_cl_number"`
	// The commit hash in the source repository associated with this change.
	CommitHash string `yaml:"source_commit_hash" json:"source_commit_hash"`
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

var googleapisURL = "https://api.github.com/repos/googleapis/googleapis/branches/master"

func findLatestGoogleapisCommit() (string, error) {
	// We don't need authentication for this API call, fortunately.
	resp, err := http.Get(googleapisURL)
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
		PreserveRegex: []string{},
		TagFormat:     "{id}/v{version}",
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
				Path:          serviceConfig.InputDirectory,
				ServiceConfig: serviceConfig.ServiceConfig,
			}
			library.APIs = append(library.APIs, api)
		}
	}
}

// addGeneratedCodeRemovals walks the module root directory to find directories containing
// GAPIC-generated files. These files are registered in state file to be removed on generation,
// as part of the clean operation (before newly-generated files are copied).
// We use removals rather than preservation as this is safer in the face of handwritten code.
// Even "pure GAPIC libraries" can (very occasionally) contain handwritten code, and
// having two different ways of expressing what should be cleaned up is likely to result
// in something being misconfigured and code being deleted accidentally.
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

// Copied from https://github.com/googleapis/librarian/blob/main/internal/librarian/state.go
// with minimal modification
func saveLibrarianState(path string, state *LibrarianState) error {
	sortStateLibraries(state)
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	err := encoder.Encode(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0644)
}

func sortStateLibraries(state *LibrarianState) {
	sort.Slice(state.Libraries, func(i, j int) bool {
		return state.Libraries[i].ID < state.Libraries[j].ID
	})
}

func parseLibrarianState(path string) (*LibrarianState, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s LibrarianState
	if err := yaml.Unmarshal(bytes, &s); err != nil {
		return nil, fmt.Errorf("unmarshaling librarian state: %w", err)
	}
	return &s, nil
}

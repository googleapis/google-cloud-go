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
	"fmt"
	"os"

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

// Copied from https://github.com/googleapis/librarian/blob/main/internal/librarian/state.go
// with minimal modification
func saveLibrarianState(path string, state *LibrarianState) error {
	bytes, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0644)
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

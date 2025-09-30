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

package request

import (
	"encoding/json"
	"fmt"
	"os"
)

// Library is the combination of all the fields used by CLI requests and responses.
// Each CLI command has its own request/response type, but they all use Library.
type Library struct {
	ID      string `json:"id,omitempty"`
	Version string `json:"version,omitempty"`
	APIs    []API  `json:"apis,omitempty"`
	// SourcePaths are the directories to which librarian contributes code.
	// For Go, this is typically the Go module directory.
	SourcePaths []string `json:"source_roots,omitempty"`
	// PreserveRegex are files/directories to leave untouched during generation.
	// This is useful for preserving handwritten helper files or customizations.
	PreserveRegex []string `json:"preserve_regex,omitempty"`
	// RemoveRegex are files/directories to remove during generation.
	RemoveRegex []string `json:"remove_regex,omitempty"`
	// Changes are the changes being released (in a release request)
	Changes []*Change `json:"changes,omitempty"`
	// Specifying a tag format allows librarian to honor this format when creating
	// a tag for the release of the library. The replacement values of {id} and {version}
	// permitted to reference the values configured in the library. If not specified
	// the assumed format is {id}-{version}. e.g., {id}/v{version}.
	TagFormat string `yaml:"tag_format,omitempty" json:"tag_format,omitempty"`
	// ReleaseTriggered indicates whether this library is being released (in a release request)
	ReleaseTriggered bool `json:"release_triggered,omitempty"`
}

// API corresponds to a single API definition within a librarian request/response.
type API struct {
	Path          string `json:"path,omitempty"`
	ServiceConfig string `json:"service_config,omitempty"`
}

// Change represents a single commit change for a library.
type Change struct {
	Type             string `json:"type"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	PiperCLNumber    string `json:"piper_cl_number"`
	SourceCommitHash string `json:"source_commit_hash"`
}

// ParseLibrary reads a file from the given path and unmarshals
// it into a Library struct. This is used for build and generate, where the requests
// are simply the library, with no wrapping.
func ParseLibrary(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("librariangen: failed to read request file from %s: %w", path, err)
	}

	var req Library
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("librariangen: failed to unmarshal request file %s: %w", path, err)
	}

	return &req, nil
}

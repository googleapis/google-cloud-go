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

// Request corresponds to a librarian request (e.g., generate-request.json).
// It is unmarshalled from the generate-request.json file.
type Request struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	APIs    []API  `json:"apis"`
	// SourcePaths are the directories to which librarian contributes code.
	// For Go, this is typically the Go module directory.
	SourcePaths []string `json:"source_paths"`
	// PreserveRegex are files/directories to leave untouched during generation.
	// This is useful for preserving handwritten helper files or customizations.
	PreserveRegex []string `json:"preserve_regex"`
	// RemoveRegex are files/directories to remove during generation.
	RemoveRegex []string `json:"remove_regex"`
}

// API corresponds to a single API definition within a librarian request.
type API struct {
	Path          string `json:"path"`
	ServiceConfig string `json:"service_config"`
}

// Parse reads a generate-request.json file from the given path and unmarshals
// it into a Request struct.
func Parse(path string) (*Request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read request file from %s: %w", path, err)
	}

	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request file %s: %w", path, err)
	}

	return &req, nil
}

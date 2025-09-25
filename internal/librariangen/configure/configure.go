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

package configure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// NewAPIStatus is the API.Status value used to represent "this is a new API being configured".
const NewAPIStatus = "new"

// Test substitution vars.
var (
	requestParse = Parse
	responseSave = saveResponse
)

// Config holds the internal librariangen configuration for the configure command.
type Config struct {
	// LibrarianDir is the path to the librarian-tool input directory.
	// It is expected to contain the configure-request.json file.
	LibrarianDir string
	// InputDir is the path to the .librarian/generator-input directory from the
	// language repository.
	InputDir string
	// OutputDir is the path to the empty directory where librariangen writes
	// its output for global files.
	OutputDir string
	// SourceDir is the path to a complete checkout of the googleapis repository.
	SourceDir string
	// RepoDir is the path to a read-only mount of existing relevant (global or library-specific)
	// files in the language repository.
	RepoDir string
}

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.LibrarianDir == "" {
		return errors.New("librariangen: librarian directory must be set")
	}
	if c.InputDir == "" {
		return errors.New("librariangen: input directory must be set")
	}
	if c.OutputDir == "" {
		return errors.New("librariangen: output directory must be set")
	}
	if c.SourceDir == "" {
		return errors.New("librariangen: source directory must be set")
	}
	if c.RepoDir == "" {
		return errors.New("librariangen: repo directory must be set")
	}
	return nil
}

// Configure configures a new library, or a new API within an existing library.
// This is effectively the entry point of the "configure" container command.
func Configure(ctx context.Context, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("librariangen: invalid configuration: %w", err)
	}
	slog.Debug("librariangen: configure command started")
	configureReq, err := readConfigureReq(cfg.LibrarianDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read request: %w", err)
	}
	library, err := findLibraryToConfigure(configureReq)
	if err != nil {
		return err
	}

	response, err := configureLibrary(cfg, library)
	if err != nil {
		return err
	}
	if err := saveConfigureResp(response, cfg.LibrarianDir); err != nil {
		return fmt.Errorf("librariangen: failed to save response: %w", err)
	}

	return nil
}

// readConfigureReq reads generate-request.json from the librarian-tool input directory.
// The request file tells librariangen which library and APIs to generate.
// It is prepared by the Librarian tool and mounted at /librarian.
func readConfigureReq(librarianDir string) (*Request, error) {
	reqPath := filepath.Join(librarianDir, "configure-request.json")
	slog.Debug("librariangen: reading generate request", "path", reqPath)

	configureReq, err := requestParse(reqPath)
	if err != nil {
		return nil, err
	}
	slog.Debug("librariangen: successfully unmarshalled request")
	return configureReq, nil
}

// saveConfigureResp saves the response in configure-response.json in the librarian-tool input directory.
// The response file tells Librarian how to reconfigure the library in its state file.
func saveConfigureResp(resp *request.Library, librarianDir string) error {
	respPath := filepath.Join(librarianDir, "configure-response.json")
	slog.Debug("librariangen: saving configure response", "path", respPath)

	if err := responseSave(resp, respPath); err != nil {
		return err
	}
	slog.Debug("librariangen: successfully marshalled response")
	return nil
}

func findLibraryToConfigure(req *Request) (*request.Library, error) {
	var library *request.Library
	for _, candidate := range req.Libraries {
		var hasNewAPI bool
		for _, api := range candidate.APIs {
			if api.Status == NewAPIStatus {
				if hasNewAPI {
					return nil, fmt.Errorf("librariangen: library %s has multiple new APIs", candidate.ID)
				}
				hasNewAPI = true
			}
		}
		if hasNewAPI {
			if library != nil {
				return nil, fmt.Errorf("librariangen: multiple libraries have new APIs (at least %s and %s)", library.ID, candidate.ID)
			}
			library = candidate
		}
	}
	if library == nil {
		return nil, fmt.Errorf("librariangen: no libraries have new APIs")
	}
	return library, nil
}

// configureLibrary performs the real work of configuring a new or updated module,
// creating files and populating the state file entry.
func configureLibrary(cfg *Config, library *request.Library) (*request.Library, error) {
	return nil, errors.New("configure unimplemented")
}

// Request corresponds to a librarian configure request.
// It is unmarshalled from the configure-request.json file. Note that
// this request is in a different form from most other requests, as it
// contains all libraries.
type Request struct {
	// All libraries configured within the repository.
	Libraries []*request.Library `json:"libraries"`
}

// Parse reads a configure-request.json file from the given path and unmarshals
// it into a ConfigureRequest struct.
func Parse(path string) (*Request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("librariangen: failed to read request file from %s: %w", path, err)
	}

	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("librariangen: failed to unmarshal request file %s: %w", path, err)
	}

	return &req, nil
}

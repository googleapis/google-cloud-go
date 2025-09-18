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

package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// Test substitution vars.
var (
	execvRun     = execv.Run
	requestParse = request.ParseLibrary
)

// Config holds the internal librariangen configuration for the build command.
type Config struct {
	// LibrarianDir is the path to the librarian-tool input directory.
	// It is expected to contain the build-request.json file.
	LibrarianDir string
	// RepoDir is the path to ehte entire language repository.
	RepoDir string
}

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.LibrarianDir == "" {
		return errors.New("librariangen: librarian directory must be set")
	}
	if c.RepoDir == "" {
		return errors.New("librariangen: repo directory must be set")
	}
	return nil
}

// Build is the main entrypoint for the `build` command. It runs `go build`
// and then `go test`.
func Build(ctx context.Context, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("librariangen: invalid configuration: %w", err)
	}
	slog.Debug("librariangen: generate command started")

	buildReq, err := readBuildReq(cfg.LibrarianDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read request: %w", err)
	}
	moduleDir := filepath.Join(cfg.RepoDir, buildReq.ID)
	if err := goBuild(ctx, moduleDir, buildReq.ID); err != nil {
		return fmt.Errorf("librariangen: failed to run 'go build': %w", err)
	}
	if err := goTest(ctx, moduleDir, buildReq.ID); err != nil {
		return fmt.Errorf("librariangen: failed to run 'go test': %w", err)
	}
	return nil
}

// goBuild builds all the code under the specified directory
func goBuild(ctx context.Context, dir, module string) error {
	slog.Info("librariangen: building", "module", module)
	args := []string{"go", "build", "./..."}
	return execvRun(ctx, args, dir)
}

// goTest builds all the code under the specified directory
func goTest(ctx context.Context, dir, module string) error {
	slog.Info("librariangen: testing", "module", module)
	args := []string{"go", "test", "./...", "-short"}
	return execvRun(ctx, args, dir)
}

// readBuildReq reads generate-request.json from the librarian-tool input directory.
// The request file tells librariangen which library and APIs to generate.
// It is prepared by the Librarian tool and mounted at /librarian.
func readBuildReq(librarianDir string) (*request.Library, error) {
	reqPath := filepath.Join(librarianDir, "build-request.json")
	slog.Debug("librariangen: reading build request", "path", reqPath)

	buildReq, err := requestParse(reqPath)
	if err != nil {
		return nil, err
	}
	slog.Debug("librariangen: successfully unmarshalled request", "library_id", buildReq.ID)
	return buildReq, nil
}

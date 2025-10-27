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

package postprocessor

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/configure"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/module"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// Test substitution vars.
var (
	execvRun = execv.Run
)

// PostProcess is the entrypoint for post-processing generated files. It runs
// formatters and other tools to ensure code quality. The high-level steps are:
//
//  1. Modify the generated snippets to specify the current version
//  2. Run `goimports` to format the code.
func PostProcess(ctx context.Context, req *request.Library, outputDir, moduleDir string, moduleConfig *config.ModuleConfig) error {
	slog.Debug("librariangen: starting post-processing", "directory", moduleDir)

	if len(req.APIs) == 0 {
		slog.Debug("librariangen: no APIs in request, skipping module initialization")
		return nil
	}

	if req.Version == "" {
		return fmt.Errorf("librariangen: no version for API: %s (required for post-processing)", req.ID)
	}

	if err := module.UpdateSnippetsMetadata(req, outputDir, outputDir, moduleConfig); err != nil {
		return fmt.Errorf("librariangen: failed to update snippets metadata: %w", err)
	}

	if err := goimports(ctx, moduleDir); err != nil {
		return fmt.Errorf("librariangen: failed to run 'goimports': %w", err)
	}

	// If we have a single API, and it's new, then this must be the first time generating this library.
	// We run go mod init and go mod tidy *only* this time. We can only run this once because once go.mod and go.sum have
	// been created, Librarian should refuse to copy it over unless the old version is deleted first...
	// and we *don't* want to run it every time (partly because generate shouldn't be updating dependencies,
	// and partly because there might be handwritten code in the library, which generate can't "see").
	// When configuring the first generated API for a library, we assume the whole library is new.
	//
	// We can't even run "go mod init" from configure and just "go mod tidy" here, as files written
	// by the configure command aren't available during generate.
	if len(req.APIs) == 1 && req.APIs[0].Status == configure.NewAPIStatus {
		if err := goModInit(ctx, moduleDir); err != nil {
			return fmt.Errorf("librariangen: failed to run 'go mod init': %w", err)
		}
		if err := goModTidy(ctx, moduleDir); err != nil {
			return fmt.Errorf("librariangen: failed to run 'go mod tidy': %w", err)
		}
	}

	slog.Debug("librariangen: post-processing finished successfully")
	return nil
}

// goimports runs the goimports tool on a directory to format Go files and
// manage imports.
func goimports(ctx context.Context, dir string) error {
	slog.Debug("librariangen: running goimports", "directory", dir)
	// The `.` argument will make goimports process all go files in the directory
	// and its subdirectories. The -w flag writes results back to source files.
	args := []string{"goimports", "-w", "."}
	return execvRun(ctx, args, dir)
}

// goModInit runs "go mod init" on a directory to initialize the module.
func goModInit(ctx context.Context, dir string) error {
	slog.Debug("librariangen: running go mod init", "directory", dir)
	args := []string{"go", "mod", "init"}
	return execvRun(ctx, args, dir)
}

// goModTidy runs "go mod tidy" on a directory to add appropriate dependencies.
func goModTidy(ctx context.Context, dir string) error {
	slog.Debug("librariangen: running go mod tidy", "directory", dir)
	args := []string{"go", "mod", "tidy"}
	return execvRun(ctx, args, dir)
}

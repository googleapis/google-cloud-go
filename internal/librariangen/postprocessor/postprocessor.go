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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
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
func PostProcess(ctx context.Context, req *request.Request, outputDir, moduleDir string, moduleConfig *config.ModuleConfig) error {
	slog.Debug("librariangen: starting post-processing", "directory", moduleDir)

	if len(req.APIs) == 0 {
		slog.Debug("librariangen: no APIs in request, skipping module initialization")
		return nil
	}

	if req.Version == "" {
		return fmt.Errorf("librariangen: no version for API: %s (required for post-processing)", req.ID)
	}

	if err := updateSnippetsMetadata(req, outputDir, moduleConfig); err != nil {
		return fmt.Errorf("librariangen: failed to update snippets metadata: %w", err)
	}

	if err := goimports(ctx, moduleDir); err != nil {
		return fmt.Errorf("librariangen: failed to run 'goimports': %w", err)
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

// updateSnippetsMetadata updates all snippet files to populate the $VERSION placeholder.
func updateSnippetsMetadata(req *request.Request, outputDir string, moduleConfig *config.ModuleConfig) error {
	moduleName := req.ID
	version := req.Version

	slog.Debug("librariangen: updating snippets metadata")
	snpDir := filepath.Join(outputDir, "internal", "generated", "snippets", moduleName)

	for _, api := range req.APIs {
		apiConfig := moduleConfig.GetAPIConfig(api.Path)
		clientDirName, err := apiConfig.GetClientDirectory()
		if err != nil {
			return err
		}

		snippetFile := "snippet_metadata." + apiConfig.GetProtoPackage() + ".json"
		path := filepath.Join(snpDir, clientDirName, snippetFile)
		slog.Debug("librariangen: updating snippet metadata file", "path", path)
		read, err := os.ReadFile(path)
		if err != nil {
			// If the snippet metadata doesn't exist, that's probably because this API path
			// is proto-only (so the GAPIC generator hasn't run). Continue to the next API path.
			if errors.Is(err, os.ErrNotExist) {
				slog.Info("librariangen: snippet metadata file not found; assuming proto-only package", "path", path)
				continue
			}
			return err
		}
		if strings.Contains(string(read), "$VERSION") {
			s := strings.Replace(string(read), "$VERSION", version, 1)
			err = os.WriteFile(path, []byte(s), 0)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

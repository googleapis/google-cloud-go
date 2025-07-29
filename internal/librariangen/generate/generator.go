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

package generate

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/bazel"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/postprocessor"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/protoc"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// TODO: The determination of whether a module is new or not should be
// driven by configuration passed down from the orchestrating Librarian tool.
// For now, we hardcode it to false, assuming we are always regenerating
// existing modules.
const isNewModule = false

// Test substitution vars.
var (
	postProcess  = postprocessor.PostProcess
	bazelParse   = bazel.Parse
	protocRun    = protoc.Run
	requestParse = request.Parse
)

// Config holds the internal librariangen configuration for the generate command.
type Config struct {
	// LibrarianDir is the path to the librarian-tool input directory.
	// It is expected to contain the generate-request.json file.
	LibrarianDir string
	// InputDir is the path to the .librarian/generator-input directory from the
	// language repository.
	InputDir string
	// OutputDir is the path to the empty directory where librariangen writes
	// its output.
	OutputDir string
	// SourceDir is the path to a complete checkout of the googleapis repository.
	SourceDir string
	// EnablePostProcessor controls whether the post-processor is run.
	// This should always be true in production.
	EnablePostProcessor bool
}

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.LibrarianDir == "" {
		return fmt.Errorf("librarian directory must be set")
	}
	if c.InputDir == "" {
		return fmt.Errorf("input directory must be set")
	}
	if c.OutputDir == "" {
		return fmt.Errorf("output directory must be set")
	}
	if c.SourceDir == "" {
		return fmt.Errorf("source directory must be set")
	}
	return nil
}

// Generate is the main entrypoint for the `generate` command. It orchestrates
// the entire generation process. The high-level steps are:
//
//  1. Validate the configuration.
//  2. Invoke `protoc` for each API specified in the request, generating Go
//     files into a nested directory structure (e.g.,
//     `/output/cloud.google.com/go/...`).
//  3. Fix the permissions of all generated `.go` files to `0644`.
//  4. Flatten the output directory, moving the generated module(s) to the top
//     level of the output directory (e.g., `/output/chronicle`).
//  5. If the `EnablePostProcessor` flag is true, run the post-processor on the
//     generated module(s) to add module files (`go.mod`, `README.md`, etc.).
//
// The `EnablePostProcessor` flag should always be true in production. It can be
// disabled during development to inspect the "raw" protoc output before any
// post-processing is applied.
func Generate(ctx context.Context, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	modulePath, err := handleGapicgen(ctx, cfg)
	if err != nil {
		return fmt.Errorf("gapic generation failed: %w", err)
	}
	if err := fixPermissions(cfg.OutputDir); err != nil {
		return fmt.Errorf("failed to fix permissions: %w", err)
	}
	if err := flattenOutput(cfg.OutputDir); err != nil {
		return fmt.Errorf("failed to flatten output: %w", err)
	}
	slog.Info("using module path from final API", "importpath", modulePath)

	if cfg.EnablePostProcessor {
		slog.Debug("post-processor enabled")
		generateReq, err := readGenerateReq(cfg.LibrarianDir)
		if err != nil {
			return err
		}
		// The module name is the first part of the API path.
		// E.g. google/cloud/workflows/v1 -> workflows
		moduleName := ""
		if len(generateReq.APIs) > 0 {
			parts := strings.Split(generateReq.APIs[0].Path, "/")
			if len(parts) > 2 {
				moduleName = parts[2]
			}
		}
		if moduleName == "" {
			return fmt.Errorf("could not determine module name from API path")
		}
		moduleDir := filepath.Join(cfg.OutputDir, moduleName)
		if err := postProcess(ctx, generateReq, moduleDir, isNewModule); err != nil {
			return fmt.Errorf("post-processing failed: %w", err)
		}
	}

	slog.Debug("generate command finished")
	return nil
}

// handleGapicgen handles the protoc GAPIC generation logic for the 'generate' CLI command.
// It reads a request file, and for each API specified, it invokes protoc
// to generate the client library.
func handleGapicgen(ctx context.Context, cfg *Config) (string, error) {
	slog.Debug("generate command started")

	generateReq, err := readGenerateReq(cfg.LibrarianDir)
	if err != nil {
		return "", err
	}
	var bazelConfig *bazel.Config
	for _, api := range generateReq.APIs {
		apiServiceDir := filepath.Join(cfg.SourceDir, api.Path)
		slog.Info("processing api", "service_dir", apiServiceDir)
		var err error
		bazelConfig, err = bazelParse(apiServiceDir)
		if err != nil {
			return "", fmt.Errorf("failed to parse BUILD.bazel for %s: %w", apiServiceDir, err)
		}
		if err := bazelConfig.Validate(); err != nil {
			return "", fmt.Errorf("invalid bazel config for %s: %w", apiServiceDir, err)
		}
		slog.Info("bazel config loaded", "conf", fmt.Sprintf("%+v", bazelConfig))
		args, err := protoc.Build(generateReq, &api, apiServiceDir, bazelConfig, cfg.SourceDir, cfg.OutputDir)
		if err != nil {
			return "", fmt.Errorf("failed to build protoc command for api %q in library %q: %w", api.Path, generateReq.ID, err)
		}
		if err := protocRun(ctx, args, cfg.OutputDir); err != nil {
			return "", fmt.Errorf("protoc failed for api %q in library %q: %w", api.Path, generateReq.ID, err)
		}
	}

	// We'll use the import path of the last API's BUILD.bazel to initialize the module.
	// This assumes all APIs in the request belong to the same module.
	// TODO: Ensure the root module path is used here.
	modulePath := bazelConfig.ModulePath()
	return modulePath, nil
}

// readGenerateReq reads generate-request.json from the librarian-tool input directory.
// The request file tells librariangen which library and APIs to generate.
// It is prepared by the Librarian tool and mounted at /librarian.
func readGenerateReq(librarianDir string) (*request.Request, error) {
	reqPath := filepath.Join(librarianDir, "generate-request.json")
	slog.Debug("reading generate request", "path", reqPath)

	generateReq, err := requestParse(reqPath)
	if err != nil {
		return nil, err
	}
	slog.Debug("successfully unmarshalled request", "library_id", generateReq.ID)
	return generateReq, nil
}

// fixPermissions recursively finds all .go files in the given directory and sets
// their permissions to 0644.
func fixPermissions(dir string) error {
	slog.Debug("fixing file permissions", "dir", dir)
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			slog.Debug("fixing file", "path", path)
			if err := os.Chmod(path, 0644); err != nil {
				return fmt.Errorf("failed to chmod %s: %w", path, err)
			}
		}
		return nil
	})
}

// flattenOutput moves the contents of /output/cloud.google.com/go/ to the top
// level of /output.
func flattenOutput(outputDir string) error {
	slog.Debug("flattening output directory", "dir", outputDir)
	goDir := filepath.Join(outputDir, "cloud.google.com", "go")
	if _, err := os.Stat(goDir); os.IsNotExist(err) {
		slog.Warn("go directory does not exist, skipping flatten", "path", goDir)
		return nil
	}
	files, err := os.ReadDir(goDir)
	if err != nil {
		return fmt.Errorf("failed to read dir %s: %w", goDir, err)
	}
	for _, f := range files {
		oldPath := filepath.Join(goDir, f.Name())
		newPath := filepath.Join(outputDir, f.Name())
		slog.Debug("moving file", "from", oldPath, "to", newPath)
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("failed to move %s to %s: %w", oldPath, newPath, err)
		}
	}
	// Remove the now-empty cloud.google.com directory.
	if err := os.RemoveAll(filepath.Join(outputDir, "cloud.google.com")); err != nil {
		return fmt.Errorf("failed to remove cloud.google.com: %w", err)
	}
	return nil
}

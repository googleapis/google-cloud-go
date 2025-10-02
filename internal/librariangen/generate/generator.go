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
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/bazel"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/postprocessor"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/protoc"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// Test substitution vars.
var (
	postProcess  = postprocessor.PostProcess
	bazelParse   = bazel.Parse
	execvRun     = execv.Run
	requestParse = request.ParseLibrary
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
	// DisablePostProcessor controls whether the post-processor is run.
	// This should always be false in production.
	DisablePostProcessor bool
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
//  5. If the `DisablePostProcessor` flag is false, run the post-processor on the
//     generated module(s), updating versions for snippet metadata,
//     running go mod tidy etc.
//
// The `DisablePostProcessor` flag should always be false in production. It can be
// true during development to inspect the "raw" protoc output before any
// post-processing is applied.
func Generate(ctx context.Context, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("librariangen: invalid configuration: %w", err)
	}
	slog.Debug("librariangen: generate command started")

	generateReq, err := readGenerateReq(cfg.LibrarianDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read request: %w", err)
	}
	repoConfig, err := config.LoadRepoConfig(cfg.LibrarianDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to load repo config: %w", err)
	}
	moduleConfig := repoConfig.GetModuleConfig(generateReq.ID)

	if err := invokeProtoc(ctx, cfg, generateReq, moduleConfig); err != nil {
		return fmt.Errorf("librariangen: gapic generation failed: %w", err)
	}
	if err := fixPermissions(cfg.OutputDir); err != nil {
		return fmt.Errorf("librariangen: failed to fix permissions: %w", err)
	}
	if err := flattenOutput(cfg.OutputDir); err != nil {
		return fmt.Errorf("librariangen: failed to flatten output: %w", err)
	}

	if err := applyModuleVersion(cfg.OutputDir, generateReq.ID, moduleConfig.GetModulePath()); err != nil {
		return fmt.Errorf("librariangen: failed to apply module version to output directories: %w", err)
	}

	if !cfg.DisablePostProcessor {
		slog.Debug("librariangen: post-processor enabled")
		if len(generateReq.APIs) == 0 {
			return errors.New("librariangen: no APIs in request")
		}
		moduleDir := filepath.Join(cfg.OutputDir, generateReq.ID)
		if err := postProcess(ctx, generateReq, cfg.OutputDir, moduleDir, moduleConfig); err != nil {
			return fmt.Errorf("librariangen: post-processing failed: %w", err)
		}
	}

	slog.Debug("librariangen: generate command finished")
	return nil
}

// invokeProtoc handles the protoc GAPIC generation logic for the 'generate' CLI command.
// It reads a request file, and for each API specified, it invokes protoc
// to generate the client library. It returns the module path and the path to the service YAML.
func invokeProtoc(ctx context.Context, cfg *Config, generateReq *request.Library, moduleConfig *config.ModuleConfig) error {
	for _, api := range generateReq.APIs {
		apiServiceDir := filepath.Join(cfg.SourceDir, api.Path)
		slog.Info("processing api", "service_dir", apiServiceDir)
		bazelConfig, err := bazelParse(apiServiceDir)
		apiConfig := moduleConfig.GetAPIConfig(api.Path)
		if apiConfig.HasDisableGAPIC() {
			bazelConfig.DisableGAPIC()
		}
		if err != nil {
			return fmt.Errorf("librariangen: failed to parse BUILD.bazel for %s: %w", apiServiceDir, err)
		}
		args, err := protoc.Build(generateReq, &api, apiServiceDir, bazelConfig, cfg.SourceDir, cfg.OutputDir)
		if err != nil {
			return fmt.Errorf("librariangen: failed to build protoc command for api %q in library %q: %w", api.Path, generateReq.ID, err)
		}
		if err := execvRun(ctx, args, cfg.OutputDir); err != nil {
			return fmt.Errorf("librariangen: protoc failed for api %q in library %q: %w", api.Path, generateReq.ID, err)
		}
	}
	return nil
}

// readGenerateReq reads generate-request.json from the librarian-tool input directory.
// The request file tells librariangen which library and APIs to generate.
// It is prepared by the Librarian tool and mounted at /librarian.
func readGenerateReq(librarianDir string) (*request.Library, error) {
	reqPath := filepath.Join(librarianDir, "generate-request.json")
	slog.Debug("librariangen: reading generate request", "path", reqPath)

	generateReq, err := requestParse(reqPath)
	if err != nil {
		return nil, err
	}
	slog.Debug("librariangen: successfully unmarshalled request", "library_id", generateReq.ID)
	return generateReq, nil
}

// fixPermissions recursively finds all .go files in the given directory and sets
// their permissions to 0644.
func fixPermissions(dir string) error {
	slog.Debug("librariangen: changing file permissions to 644", "dir", dir)
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			if err := os.Chmod(path, 0644); err != nil {
				return fmt.Errorf("librariangen: failed to chmod %s: %w", path, err)
			}
		}
		return nil
	})
}

// flattenOutput moves the contents of /output/cloud.google.com/go/ to the top
// level of /output.
func flattenOutput(outputDir string) error {
	slog.Debug("librariangen: flattening output directory", "dir", outputDir)
	goDir := filepath.Join(outputDir, "cloud.google.com", "go")
	if _, err := os.Stat(goDir); os.IsNotExist(err) {
		return fmt.Errorf("librariangen: go directory does not exist in path: %s", goDir)
	}
	if err := moveFiles(goDir, outputDir); err != nil {
		return err
	}
	// Remove the now-empty cloud.google.com directory.
	if err := os.RemoveAll(filepath.Join(outputDir, "cloud.google.com")); err != nil {
		return fmt.Errorf("librariangen: failed to remove cloud.google.com: %w", err)
	}
	return nil
}

// applyModuleVersion reorganizes the (already flattened) output directory
// appropriately for versioned modules. For a module path of the form
// cloud.google.com/go/{module-id}/{version}, we expect to find
// /output/{id}/{version} and /output/internal/generated/snippets/{module-id}/{version}.
// In most cases, we only support a single major version of the module, rooted at
// /{module-id} in the repository, so the content of these directories are moved into
// /output/{module-id} and /output/internal/generated/snippets/{id}.
//
// However, when we need to support multiple major versions, we use {module-id}/{version}
// as the *library* ID (in the state file etc). That indicates that the module is rooted
// in that versioned directory (e.g. "pubsub/v2"). In that case, the flattened code is
// already in the right place, so this function doesn't need to do anything.
func applyModuleVersion(outputDir, libraryID, modulePath string) error {
	parts := strings.Split(modulePath, "/")
	// Just cloud.google.com/go/xyz
	if len(parts) == 3 {
		return nil
	}
	if len(parts) != 4 {
		return fmt.Errorf("librariangen: unexpected module path format: %s", modulePath)
	}
	// e.g. dataproc
	id := parts[2]
	// e.g. v2
	version := parts[3]

	if libraryID == id+"/"+version {
		return nil
	}

	srcDir := filepath.Join(outputDir, id)
	srcVersionDir := filepath.Join(srcDir, version)
	snippetsDir := filepath.Join(outputDir, "internal", "generated", "snippets", id)
	snippetsVersionDir := filepath.Join(snippetsDir, version)

	if err := moveFiles(srcVersionDir, srcDir); err != nil {
		return err
	}
	if err := os.RemoveAll(srcVersionDir); err != nil {
		return fmt.Errorf("librariangen: failed to remove %s: %w", srcVersionDir, err)
	}

	if err := moveFiles(snippetsVersionDir, snippetsDir); err != nil {
		return err
	}
	if err := os.RemoveAll(snippetsVersionDir); err != nil {
		return fmt.Errorf("librariangen: failed to remove %s: %w", snippetsVersionDir, err)
	}
	return nil
}

// moveFiles moves all files (and directories) from sourceDir to targetDir.
func moveFiles(sourceDir, targetDir string) error {
	files, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read dir %s: %w", sourceDir, err)
	}
	for _, f := range files {
		oldPath := filepath.Join(sourceDir, f.Name())
		newPath := filepath.Join(targetDir, f.Name())
		slog.Debug("librariangen: moving file", "from", oldPath, "to", newPath)
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("librariangen: failed to move %s to %s: %w", oldPath, newPath, err)
		}
	}
	return nil
}

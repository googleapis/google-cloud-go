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
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/postprocessor"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/protoc"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
	"gopkg.in/yaml.v3"
)

// TODO(quartzmo): The determination of whether a module is new or not should be
// driven by configuration passed down from the orchestrating Librarian tool.
// For now, we hardcode it to false, assuming we are always regenerating
// existing modules.
// See https://github.com/googleapis/librarian/issues/1022.
const isNewModule = false

// Test substitution vars.
var (
	postProcess  = postprocessor.PostProcess
	bazelParse   = bazel.Parse
	execvRun     = execv.Run
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
//     generated module(s) to add module files (`go.mod`, `README.md`, etc.).
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
		return fmt.Errorf("librariangen: failed to flatten output: %w", err)
	}
	if err := invokeProtoc(ctx, cfg, generateReq); err != nil {
		return fmt.Errorf("librariangen: gapic generation failed: %w", err)
	}
	if err := fixPermissions(cfg.OutputDir); err != nil {
		return fmt.Errorf("librariangen: failed to fix permissions: %w", err)
	}
	if err := flattenOutput(cfg.OutputDir); err != nil {
		return fmt.Errorf("librariangen: failed to flatten output: %w", err)
	}

	if !cfg.DisablePostProcessor {
		slog.Debug("librariangen: post-processor enabled")
		if len(generateReq.APIs) == 0 {
			return errors.New("librariangen: no APIs in request")
		}
		// Get the name of the service config YAML from the first API's BUILD.bazel file.
		firstAPIServiceDir := filepath.Join(cfg.SourceDir, generateReq.APIs[0].Path)
		bazelConfig, err := bazelParse(firstAPIServiceDir)
		if err != nil {
			return fmt.Errorf("librariangen: failed to parse BUILD.bazel for %s: %w", firstAPIServiceDir, err)
		}
		// Get the module title from the first API's service config YAML.
		// This assumes all APIs in the request belong to the same module.
		serviceYAMLPath := filepath.Join(firstAPIServiceDir, bazelConfig.ServiceYAML())
		title, err := readTitleFromServiceYAML(serviceYAMLPath)
		if err != nil {
			return fmt.Errorf("librariangen: failed to read title from service yaml: %w", err)
		}
		moduleDir := filepath.Join(cfg.OutputDir, generateReq.ID)
		if err := postProcess(ctx, generateReq, cfg.OutputDir, moduleDir, isNewModule, title); err != nil {
			return fmt.Errorf("librariangen: post-processing failed: %w", err)
		}
	}

	slog.Debug("librariangen: generate command finished")
	return nil
}

// invokeProtoc handles the protoc GAPIC generation logic for the 'generate' CLI command.
// It reads a request file, and for each API specified, it invokes protoc
// to generate the client library. It returns the module path and the path to the service YAML.
func invokeProtoc(ctx context.Context, cfg *Config, generateReq *request.Request) error {
	for _, api := range generateReq.APIs {
		apiServiceDir := filepath.Join(cfg.SourceDir, api.Path)
		slog.Info("processing api", "service_dir", apiServiceDir)
		bazelConfig, err := bazelParse(apiServiceDir)
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
func readGenerateReq(librarianDir string) (*request.Request, error) {
	reqPath := filepath.Join(librarianDir, "generate-request.json")
	slog.Debug("librariangen: reading generate request", "path", reqPath)

	generateReq, err := requestParse(reqPath)
	if err != nil {
		return nil, err
	}
	slog.Debug("librariangen: successfully unmarshalled request", "library_id", generateReq.ID)
	return generateReq, nil
}

// readTitleFromServiceYAML reads the service YAML file and returns the title.
func readTitleFromServiceYAML(path string) (string, error) {
	slog.Debug("librariangen: reading service yaml", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("librariangen: failed to read service yaml file: %w", err)
	}
	var serviceConfig struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal(data, &serviceConfig); err != nil {
		return "", fmt.Errorf("librariangen: failed to unmarshal service yaml: %w", err)
	}
	if serviceConfig.Title == "" {
		return "", errors.New("librariangen: title not found in service yaml")
	}
	return serviceConfig.Title, nil
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
	files, err := os.ReadDir(goDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read dir %s: %w", goDir, err)
	}
	for _, f := range files {
		oldPath := filepath.Join(goDir, f.Name())
		newPath := filepath.Join(outputDir, f.Name())
		slog.Debug("librariangen: moving file", "from", oldPath, "to", newPath)
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("librariangen: failed to move %s to %s: %w", oldPath, newPath, err)
		}
	}
	// Remove the now-empty cloud.google.com directory.
	if err := os.RemoveAll(filepath.Join(outputDir, "cloud.google.com")); err != nil {
		return fmt.Errorf("librariangen: failed to remove cloud.google.com: %w", err)
	}
	return nil
}

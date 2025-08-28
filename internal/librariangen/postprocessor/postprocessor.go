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
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/module"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// External string template vars.
var (
	//go:embed _README.md.txt
	readmeTmpl string
	//go:embed _version.go.txt
	versionTmpl string
)

// Test substitution vars.
var (
	execvRun = execv.Run
)

// PostProcess is the entrypoint for post-processing generated files. It runs
// formatters and other tools to ensure code quality. The high-level steps are:
//
//  1. Run `goimports` to format the code.
//  2. If `newModule` is true, perform one-time initialization for a new module
//     by generating a placeholder `CHANGES.md`.
//  3. Generate a module-level `internal/version.go`. This is required for both
//     new and existing modules because client-level `version.go` files import
//     it, and `go mod tidy` will fail without it.
//  4. Generate a client-level `version.go` for each API version specified in
//     the request.
//  5. Generate a `README.md`.
//  6. Run `go mod init`.
//  8. Run `go mod tidy` to clean up the `go.mod` file.
func PostProcess(ctx context.Context, req *request.Request, outputDir, moduleDir string, newModule bool, title string) error {
	slog.Debug("librariangen: starting post-processing", "directory", moduleDir, "new_module", newModule)

	if len(req.APIs) == 0 {
		slog.Debug("librariangen: no APIs in request, skipping module initialization")
		return nil
	}

	if req.Version == "" {
		return fmt.Errorf("librariangen: no version for API: %s (required for post-processing)", req.ID)
	}

	// E.g. cloud.google.com/go/chronicle
	modulePath := "cloud.google.com/go/" + req.ID

	if newModule {
		slog.Debug("librariangen: initializing new module")
		if err := generateChanges(moduleDir); err != nil {
			return fmt.Errorf("librariangen: failed to generate CHANGES.md: %w", err)
		}
	}
	if err := module.GenerateInternalVersionFile(moduleDir, req.Version); err != nil {
		return fmt.Errorf("librariangen: failed to generate internal/version.go: %w", err)
	}

	if err := generateClientVersionFiles(req, moduleDir, req.ID); err != nil {
		return fmt.Errorf("librariangen: failed to generate client version files: %w", err)
	}

	if err := module.UpdateSnippetsMetadata(outputDir, req.ID, req.Version); err != nil {
		return fmt.Errorf("librariangen: failed to update snippets metadata: %w", err)
	}

	// The README should be updated on every run.
	if err := generateReadme(moduleDir, modulePath, title); err != nil {
		return fmt.Errorf("librariangen: failed to generate README.md: %w", err)
	}

	if err := goModInit(ctx, modulePath, moduleDir); err != nil {
		return fmt.Errorf("librariangen: failed to run 'go mod init': %w", err)
	}

	if err := goimports(ctx, moduleDir); err != nil {
		return fmt.Errorf("librariangen: failed to run 'goimports': %w", err)
	}

	if err := goModTidy(ctx, moduleDir); err != nil {
		return fmt.Errorf("librariangen: failed to run 'go mod tidy': %w", err)
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

// goModInit initializes a go.mod file in the given directory.
func goModInit(ctx context.Context, modulePath, dir string) error {
	slog.Debug("librariangen: running go mod init", "directory", dir, "modulePath", modulePath)
	args := []string{"go", "mod", "init", modulePath}
	return execvRun(ctx, args, dir)
}

// goModTidy tidies the go.mod file, adding missing and removing unused dependencies.
func goModTidy(ctx context.Context, dir string) error {
	slog.Debug("librariangen: running go mod tidy", "directory", dir)
	args := []string{"go", "mod", "tidy"}
	return execvRun(ctx, args, dir)
}

// generateReadme creates a README.md file for a new module.
func generateReadme(path, modulePath, title string) error {
	readmePath := filepath.Join(path, "README.md")
	slog.Debug("librariangen: creating file", "path", readmePath)
	readmeFile, err := os.Create(readmePath)
	if err != nil {
		return err
	}
	defer readmeFile.Close()
	t := template.Must(template.New("readme").Parse(readmeTmpl))
	readmeData := struct {
		Name       string
		ModulePath string
	}{
		Name:       title,
		ModulePath: modulePath,
	}
	return t.Execute(readmeFile, readmeData)
}

// generateChanges creates a CHANGES.md file for a new module.
func generateChanges(moduleDir string) error {
	changesPath := filepath.Join(moduleDir, "CHANGES.md")
	slog.Debug("librariangen: creating file", "path", changesPath)
	content := "# Changes\n"
	return os.WriteFile(changesPath, []byte(content), 0644)
}

// generateClientVersionFiles iterates through the APIs in the request and
// generates a version.go file for each corresponding client directory.
func generateClientVersionFiles(req *request.Request, moduleDir, moduleName string) error {
	for _, api := range req.APIs {
		// E.g. google/cloud/chronicle/v1 -> apiv1
		parts := strings.Split(api.Path, "/")
		if len(parts) < 2 {
			return fmt.Errorf("librariangen: unexpected API path format: %s", api.Path)
		}
		clientDirName := "api" + parts[len(parts)-1]
		clientDir := filepath.Join(moduleDir, clientDirName)
		if err := generateClientVersionFile(clientDir, moduleName); err != nil {
			return err
		}
	}
	return nil
}

// generateClientVersionFile creates a version.go file for a client.
func generateClientVersionFile(clientDir, moduleName string) error {
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		return err
	}
	versionPath := filepath.Join(clientDir, "version.go")
	slog.Debug("librariangen: creating file", "path", versionPath)
	t := template.Must(template.New("version").Parse(versionTmpl))
	versionData := struct {
		Year               int
		Package            string
		ModuleRootInternal string
	}{
		Year:               time.Now().Year(),
		Package:            moduleName,
		ModuleRootInternal: "cloud.google.com/go/" + moduleName + "/internal",
	}
	f, err := os.Create(versionPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, versionData)
}

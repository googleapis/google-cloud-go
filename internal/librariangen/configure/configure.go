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
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/module"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
	"gopkg.in/yaml.v3"
)

// External string template vars.
var (
	//go:embed _README.md.txt
	readmeTmpl string
	//go:embed _version.go.txt
	versionTmpl string
)

// NewAPIStatus is the API.Status value used to represent "this is a new API being configured".
const NewAPIStatus = "new"

// Test substitution vars.
var (
	execvRun     = execv.Run
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
	slog.Info("librariangen: configure command started")
	configureReq, err := readConfigureReq(cfg.LibrarianDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read request: %w", err)
	}
	library, api, err := findLibraryAndAPIToConfigure(configureReq)
	if err != nil {
		return err
	}

	response, err := configureLibrary(ctx, cfg, library, api)
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
	slog.Debug("librariangen: reading configure request", "path", reqPath)

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

// findLibraryAndAPIToConfigure examines a request, and finds a single library
// containing a single new API, returning both of them. An error is returned
// if there is not exactly one library containing exactly one new API.
func findLibraryAndAPIToConfigure(req *Request) (*request.Library, *request.API, error) {
	var library *request.Library
	var api *request.API
	for _, candidate := range req.Libraries {
		var newAPI *request.API
		for _, api := range candidate.APIs {
			if api.Status == NewAPIStatus {
				if newAPI != nil {
					return nil, nil, fmt.Errorf("librariangen: library %s has multiple new APIs", candidate.ID)
				}
				newAPI = &api
			}
		}

		if newAPI != nil {
			if library != nil {
				return nil, nil, fmt.Errorf("librariangen: multiple libraries have new APIs (at least %s and %s)", library.ID, candidate.ID)
			}
			library = candidate
			api = newAPI
		}
	}
	if library == nil {
		return nil, nil, fmt.Errorf("librariangen: no libraries have new APIs")
	}
	return library, api, nil
}

// configureLibrary performs the real work of configuring a new or updated module,
// creating files and populating the state file entry.
// In theory we could just have a return type of "error", but logically this is
// returning the configure-response... it just happens to be "the library being configured"
// at the moment. If the format of configure-response ever changes, we'll need fewer
// changes if we don't make too many assumptions now.
func configureLibrary(ctx context.Context, cfg *Config, library *request.Library, api *request.API) (*request.Library, error) {
	// It's just *possible* the new path has a manually configured
	// client directory - but even if not, RepoConfig has the logic
	// for figuring out the client directory. Even if the new path
	// doesn't have a custom configuration, we can use this to
	// work out the module path, e.g. if there's a major version other
	// than v1.
	repoConfig, err := config.LoadRepoConfig(cfg.LibrarianDir)
	if err != nil {
		return nil, err
	}
	var moduleConfig = repoConfig.GetModuleConfig(library.ID)

	moduleRoot := filepath.Join(cfg.OutputDir, library.ID)
	if err := os.Mkdir(moduleRoot, 0755); err != nil {
		return nil, err
	}
	// Only a single API path can be added on each configure call, so we can tell
	// if this is a new library if it's got exactly one API path.
	// In that case, we need to add:
	// - CHANGES.md (static text: "# Changes")
	// - README.md
	// - internal/version.go
	// - go.mod
	if len(library.APIs) == 1 {
		library.SourcePaths = []string{library.ID, "internal/generated/", "internal/generated/snippets/" + library.ID}
		library.RemoveRegex = []string{"^internal/generated/snippets/" + library.ID + "/"}
		library.TagFormat = "{id}/v{version}"
		library.Version = "0.0.0"
		if err := generateReadme(cfg, library); err != nil {
			return nil, err
		}
		if err := generateChanges(cfg, library); err != nil {
			return nil, err
		}
		if err := module.GenerateInternalVersionFile(moduleRoot, library.Version); err != nil {
			return nil, err
		}
		if err := goModEditReplaceInSnippets(ctx, cfg, moduleConfig.GetModulePath(), "../../../"+library.ID); err != nil {
			return nil, err
		}
		// The postprocessor for the generate command will run "go mod init" and "go mod tidy"
		// - because it has the source code at that point. It *won't* have the version files we've
		// created here though. That's okay so long as our version.go files don't have any dependencies.
	}

	// Whether it's a new library or not, generate a version file for the new client directory.
	if err := generateClientVersionFile(cfg, moduleConfig, api.Path); err != nil {
		return nil, err
	}

	// Make changes in the Library object, to communicate state file changes back to
	// Librarian.
	if err := updateLibraryState(moduleConfig, library, api); err != nil {
		return nil, err
	}

	return library, nil
}

// generateReadme generates a README.md file in the module's root directory,
// using the service config for the first API in the library to obtain the
// service's title.
func generateReadme(cfg *Config, library *request.Library) error {
	readmePath := filepath.Join(cfg.OutputDir, library.ID, "README.md")
	serviceYAMLPath := filepath.Join(cfg.SourceDir, library.APIs[0].Path, library.APIs[0].ServiceConfig)
	title, err := readTitleFromServiceYAML(serviceYAMLPath)
	if err != nil {
		return fmt.Errorf("librariangen: failed to read title from service yaml: %w", err)
	}

	slog.Info("librariangen: creating file", "path", readmePath)
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
		ModulePath: "cloud.google.com/go/" + library.ID,
	}
	return t.Execute(readmeFile, readmeData)
}

// generateChanges generates a CHANGES.md file at the root of the module.
func generateChanges(cfg *Config, library *request.Library) error {
	changesPath := filepath.Join(cfg.OutputDir, library.ID, "CHANGES.md")
	slog.Info("librariangen: creating file", "path", changesPath)
	content := "# Changes\n"
	return os.WriteFile(changesPath, []byte(content), 0644)
}

// generateClientVersionFile creates a version.go file for a client.
func generateClientVersionFile(cfg *Config, moduleConfig *config.ModuleConfig, apiPath string) error {
	var apiConfig = moduleConfig.GetAPIConfig(apiPath)
	clientDir, err := apiConfig.GetClientDirectory()
	if err != nil {
		return err
	}

	fullClientDir := filepath.Join(cfg.OutputDir, moduleConfig.Name, clientDir)
	if err := os.MkdirAll(fullClientDir, 0755); err != nil {
		return err
	}
	versionPath := filepath.Join(fullClientDir, "version.go")
	slog.Info("librariangen: creating file", "path", versionPath)
	t := template.Must(template.New("version").Parse(versionTmpl))
	versionData := struct {
		Year               int
		Package            string
		ModuleRootInternal string
	}{
		Year:               time.Now().Year(),
		Package:            moduleConfig.Name,
		ModuleRootInternal: moduleConfig.GetModulePath() + "/internal",
	}
	f, err := os.Create(versionPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, versionData)
}

// goModEditReplaceInSnippets copies internal/generated/snippets/go.mod from
// cfg.RepoDir to cfg.OutputDir, then runs go mod edit to replace the specified
// modulePath with relativeDir which is expected to the location of the module
// relative to internal/generated/snippets
func goModEditReplaceInSnippets(ctx context.Context, cfg *Config, modulePath, relativeDir string) error {
	outputSnippetsDir := filepath.Join(cfg.OutputDir, "internal", "generated", "snippets")
	if err := os.MkdirAll(outputSnippetsDir, 0755); err != nil {
		return err
	}
	copyRepoFileToOutput(cfg, "internal/generated/snippets/go.mod")
	args := []string{"go", "mod", "edit", "-replace", fmt.Sprintf("%s=%s", modulePath, relativeDir)}
	return execvRun(ctx, args, outputSnippetsDir)
}

// copyRepoFileToOutput copies a single file (identified via path)
// from cfg.RepoDir to cfg.OutputDir.
func copyRepoFileToOutput(cfg *Config, path string) error {
	src := filepath.Join(cfg.RepoDir, path)
	dst := filepath.Join(cfg.OutputDir, path)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// updateLibraryState updates the library to add any required removal/preservation
// regexes for the specified API.
func updateLibraryState(moduleConfig *config.ModuleConfig, library *request.Library, api *request.API) error {
	apiConfig := moduleConfig.GetAPIConfig(api.Path)
	clientDirectory, err := apiConfig.GetClientDirectory()
	if err != nil {
		return err
	}
	apiParts := strings.Split(api.Path, "/")
	protobufDir := apiParts[len(apiParts)-2] + "pb/.*"
	generatedPaths := []string{
		"[^/]*_client\\.go",
		"[^/]*_client_example_go123_test\\.go",
		"[^/]*_client_example_test\\.go",
		"auxiliary\\.go",
		"auxiliary_go123\\.go",
		"doc\\.go",
		"gapic_metadata\\.json",
		"helpers\\.go",
		protobufDir,
	}
	for _, generatedPath := range generatedPaths {
		library.RemoveRegex = append(library.RemoveRegex, "^"+clientDirectory+"/"+generatedPath+"$")
	}
	return nil
}

// readTitleFromServiceYAML reads the service YAML file and returns the title.
func readTitleFromServiceYAML(path string) (string, error) {
	slog.Info("librariangen: reading service yaml", "path", path)
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

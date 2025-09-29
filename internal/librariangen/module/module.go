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

// Package module provides functions for creating and updating Go module files.
package module

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

var (
	//go:embed _internal_version.go.txt
	internalVersionTmpl string
)

// GenerateInternalVersionFile creates an internal/version.go file for the module.
func GenerateInternalVersionFile(moduleDir, version string) error {
	internalDir := filepath.Join(moduleDir, "internal")
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		return err
	}
	versionPath := filepath.Join(internalDir, "version.go")
	slog.Debug("librariangen: creating file", "path", versionPath)
	t := template.Must(template.New("internal_version").Parse(internalVersionTmpl))
	internalVersionData := struct {
		Year    int
		Version string
	}{
		Year:    time.Now().Year(),
		Version: version,
	}
	if err := os.MkdirAll(filepath.Dir(versionPath), 0755); err != nil {
		return fmt.Errorf("librariangen: creating directory for version file: %w", err)
	}

	f, err := os.Create(versionPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, internalVersionData)
}

// UpdateSnippetsMetadata updates all snippet files to populate the $VERSION placeholder, reading them from
// the sourceDir and writing them to the destDir. These two may be the same, but don't have to be.
func UpdateSnippetsMetadata(lib *request.Library, sourceDir string, destDir string, moduleConfig *config.ModuleConfig) error {
	moduleName := lib.ID
	version := lib.Version

	slog.Debug("librariangen: updating snippets metadata")
	snpDir := filepath.Join("internal", "generated", "snippets", moduleName)

	for _, api := range lib.APIs {
		apiConfig := moduleConfig.GetAPIConfig(api.Path)
		clientDirName, err := apiConfig.GetClientDirectory()
		if err != nil {
			return err
		}

		snippetFile := "snippet_metadata." + apiConfig.GetProtoPackage() + ".json"
		path := filepath.Join(snpDir, clientDirName, snippetFile)
		slog.Info("librariangen: updating snippet metadata file", "path", path)
		read, err := os.ReadFile(filepath.Join(sourceDir, path))
		if err != nil {
			// If the snippet metadata doesn't exist, that's probably because this API path
			// is proto-only (so the GAPIC generator hasn't run). Continue to the next API path.
			if errors.Is(err, os.ErrNotExist) {
				slog.Info("librariangen: snippet metadata file not found; assuming proto-only package", "path", path)
				continue
			}
			return err
		}

		content := string(read)
		var newContent string
		var oldVersion string

		if strings.Contains(content, "$VERSION") {
			newContent = strings.Replace(content, "$VERSION", version, 1)
			oldVersion = "$VERSION"
		} else {
			// This regex finds a version string like "1.2.3".
			re := regexp.MustCompile(`\d+\.\d+\.\d+`)
			if foundVersion := re.FindString(content); foundVersion != "" {
				newContent = strings.Replace(content, foundVersion, version, 1)
				oldVersion = foundVersion
			}
		}

		if newContent == "" {
			return fmt.Errorf("librariangen: no version number or placeholder found in '%s'", snippetFile)
		}

		destPath := filepath.Join(destDir, path)
		slog.Info("librariangen: updating version in snippets metadata file", "destPath", path, "old", oldVersion, "new", version)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("librariangen: creating directory for snippet file: %w", err)
		}
		err = os.WriteFile(destPath, []byte(newContent), 0644)
		if err != nil {
			return err
		}
	}
	return nil
}

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
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	f, err := os.Create(versionPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, internalVersionData)
}

// UpdateSnippetsMetadata updates the library version in all snippet metadata files,
// replacing the old version or the $VERSION placeholder.
func UpdateSnippetsMetadata(outputDir, moduleName, version string) error {
	slog.Debug("librariangen: updating snippets metadata")
	snpDir := filepath.Join(outputDir, "internal", "generated", "snippets", moduleName)
	err := filepath.WalkDir(snpDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if match, _ := filepath.Match("snippet_metadata.*.json", d.Name()); match {
			read, err := os.ReadFile(path)
			if err != nil {
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

			if newContent != "" {
				slog.Info("librariangen: updating version in snippets metadata file", "path", path, "old", oldVersion, "new", version)
				if err := os.WriteFile(path, []byte(newContent), 0); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return err
}

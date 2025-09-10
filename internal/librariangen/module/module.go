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
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
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

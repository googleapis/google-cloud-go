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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/bazel"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
	"gopkg.in/yaml.v3"
)

const betaIndicator = "It is not stable"

// ManifestEntry is used for JSON marshaling in manifest.
type ManifestEntry struct {
	APIShortname        string      `json:"api_shortname" yaml:"api-shortname"`
	DistributionName    string      `json:"distribution_name" yaml:"distribution-name"`
	Description         string      `json:"description" yaml:"description"`
	Language            string      `json:"language" yaml:"language"`
	ClientLibraryType   string      `json:"client_library_type" yaml:"client-library-type"`
	ClientDocumentation string      `json:"client_documentation" yaml:"client-documentation"`
	ReleaseLevel        string      `json:"release_level" yaml:"release-level"`
	LibraryType         libraryType `json:"library_type" yaml:"library-type"`
}

type libraryType string

const (
	gapicAutoLibraryType   libraryType = "GAPIC_AUTO"
	gapicManualLibraryType libraryType = "GAPIC_MANUAL"
	coreLibraryType        libraryType = "CORE"
	agentLibraryType       libraryType = "AGENT"
	otherLibraryType       libraryType = "OTHER"
)

// generateRepoMetadata generates a .repo-metadata.json file for a given API.
func generateRepoMetadata(ctx context.Context, cfg *Config, lib *request.Library, api *request.API, moduleConfig *config.ModuleConfig, bazelConfig *bazel.Config) error {
	apiServiceDir := filepath.Join(cfg.SourceDir, api.Path)
	yamlPath := filepath.Join(apiServiceDir, bazelConfig.ServiceYAML())

	yamlFile, err := os.Open(yamlPath)
	if err != nil {
		return fmt.Errorf("librariangen: failed to open service YAML file %s: %w", yamlPath, err)
	}
	defer yamlFile.Close()

	yamlConfig := struct {
		Title    string `yaml:"title"`
		NameFull string `yaml:"name"`
	}{}
	if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
		return fmt.Errorf("librariangen: failed to decode service YAML: %w", err)
	}

	// Construct import path and relative path for docURL and releaseLevel.
	// This is a simplified version of libraryInfo from the legacy postprocessor.
	li := &struct {
		ImportPath   string
		RelPath      string
		ReleaseLevel string
	}{
		ImportPath: bazelConfig.GAPICImportPath(),
	}
	if i := strings.Index(li.ImportPath, ";"); i != -1 {
		li.ImportPath = li.ImportPath[:i]
	}
	li.RelPath = filepath.Join(lib.ID, strings.TrimPrefix(li.ImportPath, "cloud.google.com/go/"+lib.ID))

	docURL, err := docURL(moduleConfig.GetModulePath(), li.ImportPath)
	if err != nil {
		return fmt.Errorf("librariangen: unable to build docs URL: %w", err)
	}

	releaseLevel, err := releaseLevel(cfg.OutputDir, li, bazelConfig)
	if err != nil {
		return fmt.Errorf("librariangen: unable to calculate release level for %v: %w", api.Path, err)
	}

	apiShortname, err := apiShortname(yamlConfig.NameFull)
	if err != nil {
		return fmt.Errorf("librariangen: unable to determine api_shortname from %v: %w", yamlConfig.NameFull, err)
	}

	entry := ManifestEntry{
		APIShortname:        apiShortname,
		DistributionName:    li.ImportPath,
		Description:         yamlConfig.Title,
		Language:            "go",
		ClientLibraryType:   "generated",
		ClientDocumentation: docURL,
		ReleaseLevel:        releaseLevel,
		LibraryType:         gapicAutoLibraryType,
	}

	// Determine output path: e.g., accessapproval/apiv1/.repo-metadata.json
	parts := strings.Split(api.Path, "/")
	version := parts[len(parts)-1]
	outputPath := filepath.Join(cfg.OutputDir, "cloud.google.com", "go", lib.ID, "api"+version, ".repo-metadata.json")

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("librariangen: error creating directory for %s: %w", outputPath, err)
	}

	jsonData, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("librariangen: error marshalling data for API %s: %w", api.Path, err)
	}

	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("librariangen: error writing file %s: %w", outputPath, err)
	}
	slog.Debug("librariangen: generated .repo-metadata.json", "path", outputPath)
	return nil
}

// Name is of form secretmanager.googleapis.com api_shortname
// should be prefix secretmanager.
func apiShortname(nameFull string) (string, error) {
	nameParts := strings.Split(nameFull, ".")
	if len(nameParts) > 0 {
		return nameParts[0], nil
	}
	return "", nil
}

func docURL(modulePath, importPath string) (string, error) {
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, modulePath), "/")
	return "https://cloud.google.com/go/docs/reference/" + modulePath + "/latest/" + pkgPath, nil
}

func releaseLevel(cloudDir string, li *struct {
	ImportPath   string
	RelPath      string
	ReleaseLevel string
}, bazelConfig *bazel.Config) (string, error) {
	// Prioritize Bazel config if available
	if bazelConfig.ReleaseLevel() == "ga" {
		return "stable", nil
	} else if bazelConfig.ReleaseLevel() == "beta" {
		return "preview", nil
	}

	// Fallback to import path and doc.go scan
	if li.ReleaseLevel != "" {
		return li.ReleaseLevel, nil
	}
	i := strings.LastIndex(li.ImportPath, "/")
	lastElm := li.ImportPath[i+1:]
	if strings.Contains(lastElm, "alpha") {
		return "preview", nil
	} else if strings.Contains(lastElm, "beta") {
		return "preview", nil
	}

	// Determine by scanning doc.go for our beta disclaimer
	docFile := filepath.Join(cloudDir, li.RelPath, "doc.go")
	f, err := os.Open(docFile)
	if err != nil {
		// If doc.go doesn't exist, assume stable for now.
		// This might need refinement if there are cases where doc.go is missing
		// but the API is still preview.
		if errors.Is(err, os.ErrNotExist) {
			return "stable", nil
		}
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lineCnt int
	for scanner.Scan() && lineCnt < 50 {
		line := scanner.Text()
		if strings.Contains(line, betaIndicator) {
			return "preview", nil
		}
	}
	return "stable", nil
}



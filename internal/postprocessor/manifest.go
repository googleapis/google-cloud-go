// Copyright 2023 Google LLC
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

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
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

func (p *postProcessor) readManifest() (map[string]ManifestEvent, error) {
	log.Println("reading gapic manifest")
	// Read existing manifest to preserve entries from skipped modules.
	entries := map[string]ManifestEvent{}
	manifestPath := filepath.Join(p.googleCloudDir, "internal", ".repo-metadata-full.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// UpdateManifest updates the manifest file with info about all of the confs.
func (p *postProcessor) UpdateManifest(modules string) error {
	entries := make(map[string]ManifestEntry)
	manifestPath := filepath.Join(p.googleCloudDir, "internal", ".repo-metadata-full.json")
	if modules == "" {
		log.Println("updating gapic manifest")
		var err error
		entries, err = p.generateManifestEntries()
		if err != nil {
			return err
		}
	} else {
		log.Println("updating gapic manifest for", modules)
		b, err := os.ReadFile(manifestPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if err := json.Unmarshal(b, &entries); err != nil {
				return err
			}
		}
		modList := strings.Split(modules, ",")
		for _, mod := range modList {
			for inputDir, li := range p.config.GoogleapisToImportPath {
				if !strings.Contains(li.ImportPath, mod) {
					continue
				}
				if li.ServiceConfig == "" {
					continue
				}
				yamlPath := filepath.Join(p.googleapisDir, inputDir, li.ServiceConfig)
				yamlFile, err := os.Open(yamlPath)
				if err != nil {
					return err
				}
				yamlConfig := struct {
					Title    string `yaml:"title"` // We only need the title and name.
					NameFull string `yaml:"name"`  // We only need the title and name.
				}{}
				if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
					return fmt.Errorf("decode: %v", err)
				}
				docURL, err := docURL(p.googleCloudDir, li.ImportPath, li.RelPath)
				if err != nil {
					return fmt.Errorf("unable to build docs URL: %v", err)
				}

				releaseLevel, err := releaseLevel(p.googleCloudDir, li)
				if err != nil {
					return fmt.Errorf("unable to calculate release level for %v: %v", inputDir, err)
				}

				apiShortname, err := apiShortname(yamlConfig.NameFull)
				if err != nil {
					return fmt.Errorf("unable to determine api_shortname from %v: %v", yamlConfig.NameFull, err)
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
				entries[li.ImportPath] = entry
			}
		}
	}

	// Sort and write the manifest file.
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	f, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := &bytes.Buffer{}
	buf.WriteString("{\n")

	for i, key := range keys {
		entry := entries[key]
		entryBytes, err := json.MarshalIndent(entry, "  ", "  ")
		if err != nil {
			return err
		}

		// Indent the entry itself
		indentedEntry := strings.ReplaceAll(string(entryBytes), "\n", "\n  ")

		buf.WriteString(fmt.Sprintf("  %q: %s", key, indentedEntry))
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}

	buf.WriteString("}\n")
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

// generateManifestEntries gathers info about all of the confs.
func (p *postProcessor) generateManifestEntries() (map[string]ManifestEntry, error) {
	// Read existing manifest to preserve entries from skipped modules.
	existingEntries := map[string]ManifestEntry{}
	manifestPath := filepath.Join(p.googleCloudDir, "internal", ".repo-metadata-full.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &existingEntries); err != nil {
			return nil, err
		}
	}

	entries := map[string]ManifestEntry{} // Key is the package name.
	for _, manual := range p.config.ManualClientInfo {
		entries[manual.DistributionName] = *manual
	}
	for inputDir, li := range p.config.GoogleapisToImportPath {
		if li.ServiceConfig == "" {
			continue
		}
		yamlPath := filepath.Join(p.googleapisDir, inputDir, li.ServiceConfig)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		yamlConfig := struct {
			Title    string `yaml:"title"` // We only need the title and name.
			NameFull string `yaml:"name"`  // We only need the title and name.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		docURL, err := docURL(p.googleCloudDir, li.ImportPath, li.RelPath)
		if err != nil {
			return nil, fmt.Errorf("unable to build docs URL: %v", err)
		}

		releaseLevel, err := releaseLevel(p.googleCloudDir, li)
		if err != nil {
			return nil, fmt.Errorf("unable to calculate release level for %v: %v", inputDir, err)
		}

		apiShortname, err := apiShortname(yamlConfig.NameFull)
		if err != nil {
			return nil, fmt.Errorf("unable to determine api_shortname from %v: %v", yamlConfig.NameFull, err)
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
		entries[li.ImportPath] = entry
	}

	// Add back entries from skipped modules that were in the old manifest.
	for distName, entry := range existingEntries {
		isSkipped := false
		for _, skippedPath := range p.config.SkipModuleScanPaths {
			prefix := "cloud.google.com/go/" + skippedPath
			// Add a trailing slash to prefix to avoid matching substrings, e.g.
			// `.../go/a` matching `.../go/abc`. Except when the distName is
			// exactly the prefix without a slash.
			if strings.HasPrefix(distName, prefix+"/") || distName == prefix {
				isSkipped = true
				break
			}
		}
		if isSkipped {
			if _, ok := entries[distName]; !ok {
				entries[distName] = entry
			}
		}
	}

	// Remove base module entry
	delete(entries, "")
	return entries, nil
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

func docURL(cloudDir, importPath, relPath string) (string, error) {
	dir := filepath.Join(cloudDir, relPath)
	mod, err := gocmd.CurrentMod(dir)
	if err != nil {
		return "", err
	}
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
	return "https://cloud.google.com/go/docs/reference/" + mod + "/latest/" + pkgPath, nil
}

func releaseLevel(cloudDir string, li *libraryInfo) (string, error) {
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

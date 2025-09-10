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

// Package release implements the release-init command for librariangen.
package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/module"
)

// Config holds the configuration for the release-init command.
type Config struct {
	LibrarianDir string
	RepoDir      string
	OutputDir    string
}

// Init is the entrypoint for the release-init command.
func Init(ctx context.Context, cfg *Config) error {
	slog.Info("librariangen: release.Init: starting", "config", cfg)
	reqPath := filepath.Join(cfg.LibrarianDir, "release-init-request.json")
	b, err := os.ReadFile(reqPath)
	if err != nil {
		return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to read request: %w", err))
	}

	var req Request
	if err := json.Unmarshal(b, &req); err != nil {
		return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to unmarshal request: %w", err))
	}

	repoConfig, err := config.LoadRepoConfig(cfg.LibrarianDir)
	if err != nil {
		return fmt.Errorf("librariangen: failed to load repo config: %w", err)
	}

	for _, lib := range req.Libraries {
		if !lib.ReleaseTriggered {
			continue
		}
		moduleConfig := repoConfig.GetModuleConfig(lib.ID)

		moduleDir := filepath.Join(cfg.OutputDir, lib.ID)
		slog.Info("librariangen: processing library for release", "id", lib.ID, "version", lib.Version)
		if err := updateChangelog(cfg, lib, time.Now().UTC()); err != nil {
			return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to update changelog for %s: %w", lib.ID, err))
		}
		if err := module.GenerateInternalVersionFile(moduleDir, lib.Version); err != nil {
			return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to update version for %s: %w", lib.ID, err))
		}
		if err := updateSnippetsMetadata(lib, cfg.RepoDir, cfg.OutputDir, moduleConfig); err != nil {
			return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to update snippet version for %s: %w", lib.ID, err))
		}
	}
	slog.Info("librariangen: release.Init: finished successfully")
	return nil
}

var changelogSections = []struct {
	Type    string
	Section string
}{
	{Type: "feat", Section: "Features"},
	{Type: "fix", Section: "Bug Fixes"},
	{Type: "perf", Section: "Performance Improvements"},
	{Type: "revert", Section: "Reverts"},
	{Type: "docs", Section: "Documentation"},
}

func updateChangelog(cfg *Config, lib *Library, t time.Time) error {
	relativeChangelogPath := filepath.Join(lib.ID, "CHANGES.md")
	slog.Info("librariangen: updating changelog", "path", relativeChangelogPath)

	srcPath := filepath.Join(cfg.RepoDir, relativeChangelogPath)
	oldContent, err := os.ReadFile(srcPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("librariangen: reading changelog: %w", err)
	}

	versionString := fmt.Sprintf("### %s", lib.Version)
	if bytes.Contains(oldContent, []byte(versionString)) {
		slog.Info("librariangen: changelog already up-to-date", "path", relativeChangelogPath, "version", lib.Version)
		return nil
	}

	var newEntry bytes.Buffer

	date := t.Format("2006-01-02")
	fmt.Fprintf(&newEntry, "%s (%s)\n\n", versionString, date)

	changesByType := make(map[string]map[string]bool)
	for _, change := range lib.Changes {
		if changesByType[change.Type] == nil {
			changesByType[change.Type] = make(map[string]bool)
		}
		changesByType[change.Type][change.Subject] = true
	}

	for _, section := range changelogSections {
		subjects := changesByType[section.Type]
		if len(subjects) == 0 {
			continue
		}
		fmt.Fprintf(&newEntry, "#### %s\n\n", section.Section)
		for subj := range subjects {
			fmt.Fprintf(&newEntry, "* %s\n", subj)
		}
		newEntry.WriteString("\n")
	}

	// Find the insertion point after the "# Changes" title and any blank lines.
	insertionPoint := 0
	titleMarker := []byte("# Changes")
	if i := bytes.Index(oldContent, titleMarker); i != -1 {
		// Start searching after the title.
		searchStart := i + len(titleMarker)
		// Find the first non-whitespace character after the title.
		nonWhitespaceIdx := bytes.IndexFunc(oldContent[searchStart:], func(r rune) bool {
			return !bytes.ContainsRune([]byte{' ', '\t', '\n', '\r'}, r)
		})
		if nonWhitespaceIdx != -1 {
			insertionPoint = searchStart + nonWhitespaceIdx
		} else {
			// The file only contains the title and whitespace, so append.
			insertionPoint = len(oldContent)
		}
	} else if len(oldContent) > 0 {
		// The file has content but no title, so prepend.
		insertionPoint = 0
	}

	// Ensure there's a blank line between the new entry and the old content.
	if insertionPoint > 0 && insertionPoint < len(oldContent) && oldContent[insertionPoint-1] != '\n' {
		newEntry.WriteByte('\n')
	}
	if insertionPoint == len(oldContent) && len(oldContent) > 0 && oldContent[len(oldContent)-1] != '\n' {
		// Add a newline before appending if the file doesn't end with one.
		oldContent = append(oldContent, '\n')
		insertionPoint = len(oldContent)
	}
	if insertionPoint == len(oldContent) && len(oldContent) > 0 && oldContent[len(oldContent)-1] == '\n' && (len(oldContent) < 2 || oldContent[len(oldContent)-2] != '\n') {
		// Add a blank line if there isn't one already.
		oldContent = append(oldContent, '\n')
		insertionPoint = len(oldContent)
	}

	var newContent []byte
	newContent = append(newContent, oldContent[:insertionPoint]...)
	newContent = append(newContent, newEntry.Bytes()...)
	newContent = append(newContent, oldContent[insertionPoint:]...)

	destPath := filepath.Join(cfg.OutputDir, relativeChangelogPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("librariangen: creating directory for changelog: %w", err)
	}
	return os.WriteFile(destPath, newContent, 0644)
}

func writeErrorResponse(dir string, err error) error {
	slog.Error("release.Init: failed", "error", err)
	resp := Response{Error: err.Error()}
	b, marshErr := json.MarshalIndent(resp, "", "  ")
	if marshErr != nil {
		slog.Error("failed to marshal error response", "error", marshErr)
		return err
	}
	respPath := filepath.Join(dir, "release-init-response.json")
	if writeErr := os.WriteFile(respPath, b, 0644); writeErr != nil {
		slog.Error("failed to write error response", "error", writeErr)
	}
	return err
}

// Request is the structure of the release-init-request.json file.
type Request struct {
	Libraries []*Library `json:"libraries"`
}

// Library represents a single library in the release request.
type Library struct {
	ID               string    `json:"id"`
	Version          string    `json:"version"`
	Changes          []*Change `json:"changes"`
	APIs             []*API    `json:"apis"`
	SourceRoots      []string  `json:"source_roots"`
	ReleaseTriggered bool      `json:"release_triggered"`
}

// Change represents a single commit change for a library.
type Change struct {
	Type             string `json:"type"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	PiperCLNumber    string `json:"piper_cl_number"`
	SourceCommitHash string `json:"source_commit_hash"`
}

// API represents an API definition for a library.
type API struct {
	Path string `json:"path"`
}

// Response is the structure of the release-init-response.json file.
type Response struct {
	Error string `json:"error,omitempty"`
}

// updateSnippetsMetadata updates all snippet files to populate the $VERSION placeholder, copying them from
// the repo directory to the output directory.
// TODO(https://github.com/googleapis/librarian/issues/2023): move this to module.go (and remove the code
// in postprocessor.go) when we have a common Library representation etc.
func updateSnippetsMetadata(lib *Library, repoDir string, outputDir string, moduleConfig *config.ModuleConfig) error {
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
		read, err := os.ReadFile(filepath.Join(repoDir, path))
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

		if newContent != "" {
			destPath := filepath.Join(outputDir, path)
			slog.Info("librariangen: updating version in snippets metadata file", "destPath", path, "old", oldVersion, "new", version)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("librariangen: creating directory for snippet file: %w", err)
			}
			err = os.WriteFile(destPath, []byte(newContent), 0644)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

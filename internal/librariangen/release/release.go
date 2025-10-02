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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/config"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/module"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

var now = time.Now

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

		var moduleDir string
		if isRootRepoModule(lib) {
			moduleDir = cfg.OutputDir
		} else {
			moduleDir = filepath.Join(cfg.OutputDir, lib.ID)
		}
		slog.Info("librariangen: processing library for release", "id", lib.ID, "version", lib.Version)
		if err := updateChangelog(cfg, lib, now().UTC()); err != nil {
			return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to update changelog for %s: %w", lib.ID, err))
		}
		if err := module.GenerateInternalVersionFile(moduleDir, lib.Version); err != nil {
			return writeErrorResponse(cfg.LibrarianDir, fmt.Errorf("librariangen: failed to update version for %s: %w", lib.ID, err))
		}
		if err := module.UpdateSnippetsMetadata(lib, cfg.RepoDir, cfg.OutputDir, moduleConfig); err != nil {
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

func updateChangelog(cfg *Config, lib *request.Library, t time.Time) error {
	var relativeChangelogPath string
	if isRootRepoModule(lib) {
		relativeChangelogPath = "CHANGES.md"
	} else {
		relativeChangelogPath = filepath.Join(lib.ID, "CHANGES.md")
	}
	slog.Info("librariangen: updating changelog", "path", relativeChangelogPath)

	srcPath := filepath.Join(cfg.RepoDir, relativeChangelogPath)
	oldContent, err := os.ReadFile(srcPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("librariangen: reading changelog: %w", err)
	}

	versionString := fmt.Sprintf("## [%s]", lib.Version)
	if bytes.Contains(oldContent, []byte(versionString)) {
		slog.Info("librariangen: changelog already up-to-date", "path", relativeChangelogPath, "version", lib.Version)
		return nil
	}

	var newEntry bytes.Buffer

	tag := strings.NewReplacer("{id}", lib.ID, "{version}", lib.Version).Replace(lib.TagFormat)
	encodedTag := strings.ReplaceAll(tag, "/", "%2F")
	releaseURL := "https://github.com/googleapis/google-cloud-go/releases/tag/" + encodedTag
	date := t.Format("2006-01-02")
	fmt.Fprintf(&newEntry, "## [%s](%s) (%s)\n\n", lib.Version, releaseURL, date)

	changesByType := make(map[string]map[string]*request.Change)
	for _, change := range lib.Changes {
		if changesByType[change.Type] == nil {
			changesByType[change.Type] = make(map[string]*request.Change)
		}
		changesByType[change.Type][change.Subject] = change
	}

	for _, section := range changelogSections {
		subjectsMap := changesByType[section.Type]
		if len(subjectsMap) == 0 {
			continue
		}
		fmt.Fprintf(&newEntry, "### %s\n\n", section.Section)

		var subjects []string
		for subj := range subjectsMap {
			subjects = append(subjects, subj)
		}
		sort.Strings(subjects)

		for _, subj := range subjects {
			change := subjectsMap[subj]
			var commitLink string
			if change.SourceCommitHash != "" {
				shortHash := change.SourceCommitHash
				if len(shortHash) > 7 {
					shortHash = shortHash[:7]
				}
				commitURL := fmt.Sprintf("https://github.com/googleapis/google-cloud-go/commit/%s", change.SourceCommitHash)
				commitLink = fmt.Sprintf("([%s](%s))", shortHash, commitURL)
			}

			fmt.Fprintf(&newEntry, "* %s %s\n", change.Subject, commitLink)

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

// isRootRepoModule returns whether or not the given library is
// effectively stored in the root of the repository. This is the case
// for repositories which only have a single module, indicated by
// a value in Library.SourcePaths of ".", and also by the ID
// "root-module" in google-cloud-code. Libraries which contain
// a source path of "." will usually have that as the only entry, but
// that isn't validated.
//
// This is expected to be used for repos such as gapic-generator-go,
// but does *not* apply to gax-go as the "root" for that repo is
// the v1 code; the v2 code is under a "v2" directory so we just
// use a library ID of "v2".
//
// The vast majority of modules in google-cloud-go have a single
// source path for the production code, and another for the
// generated snippets.
//
// The use of a special ID of "root-module" for the module of
// google-cloud-go containing "civil", "rpcreplay" etc is slightly
// hacky, but avoids creating special-purpose configuration which
// is realistically only ever going to be used by a single module.
// For example, we could add a "module-root" field in repo-config.yaml
// and set that to an empty string for whole-repo libraries and the
// google-cloud-go main module. The single line of code below seems
// simpler.
func isRootRepoModule(lib *request.Library) bool {
	return slices.Contains(lib.SourcePaths, ".") || lib.ID == "root-module"
}

// Request is the structure of the release-init-request.json file.
type Request struct {
	Libraries []*request.Library `json:"libraries"`
}

// Response is the structure of the release-init-response.json file.
type Response struct {
	Error string `json:"error,omitempty"`
}

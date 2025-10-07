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

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cleanupLegacyConfigs is the entrypoint for removing OwlBot and release-please
// configuration for a module being migrated to Librarian.
func cleanupLegacyConfigs(repoRoot, moduleName string) error {
	if err := cleanupOwlBotYaml(repoRoot, moduleName); err != nil {
		return fmt.Errorf("cleaning up .OwlBot.yaml: %w", err)
	}
	if err := cleanupPostProcessorConfig(repoRoot, moduleName); err != nil {
		return fmt.Errorf("cleaning up postprocessor config: %w", err)
	}

	// release-please JSON files to clean up.
	jsonFiles := []string{
		"release-please-config-individual.json",
		"release-please-config-yoshi-submodules.json",
		".release-please-manifest-individual.json",
		".release-please-manifest-submodules.json",
	}
	for _, f := range jsonFiles {
		if err := cleanupJSONFile(filepath.Join(repoRoot, f), moduleName); err != nil {
			return fmt.Errorf("cleaning up %s: %w", f, err)
		}
	}
	return nil
}

// cleanupJSONFile removes all module entries from a given JSON file.
func cleanupJSONFile(path, moduleName string) error {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(fileBytes), "\n")
	var newLines []string
	inPackages := false
	inBlock := false
	braceCount := 0
	for _, line := range lines {
		if strings.Contains(line, "\"packages\":") {
			inPackages = true
		}
		if inPackages && strings.Contains(line, "\""+moduleName+"\":") {
			inBlock = true
		}

		if inBlock {
			braceCount += strings.Count(line, "{")
			braceCount -= strings.Count(line, "}")
			if braceCount == 0 {
				inBlock = false
			}
			continue
		}

		// Handle manifest files and non-package blocks.
		if strings.Contains(line, "\""+moduleName+"\":") {
			// Check if it's the last line in a block without a trailing comma.
			trimmedLine := strings.TrimSpace(line)
			if !strings.HasSuffix(trimmedLine, ",") {
				if len(newLines) > 0 {
					lastLine := strings.TrimSpace(newLines[len(newLines)-1])
					if strings.HasSuffix(lastLine, ",") {
						newLines[len(newLines)-1] = strings.TrimSuffix(newLines[len(newLines)-1], ",")
					}
				}
			}
			continue
		}

		newLines = append(newLines, line)
	}

	output := strings.Join(newLines, "\n")
	if bytes.Equal([]byte(output), fileBytes) {
		return nil
	}

	return os.WriteFile(path, []byte(output), 0644)
}

// cleanupPostProcessorConfig removes the module and service-config entries for
// a given module from the internal/postprocessor/config.yaml file. It also adds
// the module to the skip-module-scan-paths list.
// NOTE: This function does not remove modules from manual-clients.
func cleanupPostProcessorConfig(repoRoot, moduleName string) error {
	configPath := filepath.Join(repoRoot, "internal", "postprocessor", "config.yaml")
	fileBytes, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(fileBytes), "\n")
	var newLines []string

	// First pass: remove from modules list.
	inModules := false
	for _, line := range lines {
		if strings.HasPrefix(line, "modules:") {
			inModules = true
		} else if !strings.HasPrefix(line, " ") { // top-level key
			inModules = false
		}
		if inModules && strings.TrimSpace(line) == "- "+moduleName {
			continue
		}
		newLines = append(newLines, line)
	}

	// Second pass: remove from service-configs.
	lines = newLines
	newLines = []string{}
	inServiceConfigs := false
	var serviceConfigBlock []string
	isTargetServiceConfig := false
	for _, line := range lines {
		if strings.HasPrefix(line, "service-configs:") {
			inServiceConfigs = true
			newLines = append(newLines, line)
			continue
		}
		if !strings.HasPrefix(line, " ") { // top-level key
			if inServiceConfigs {
				if !isTargetServiceConfig {
					newLines = append(newLines, serviceConfigBlock...)
				}
				isTargetServiceConfig = false
				serviceConfigBlock = nil
			}
			inServiceConfigs = false
		}

		if !inServiceConfigs {
			newLines = append(newLines, line)
			continue
		}

		// In service-configs section
		if strings.HasPrefix(line, "  - input-directory:") {
			if !isTargetServiceConfig {
				newLines = append(newLines, serviceConfigBlock...)
			}
			serviceConfigBlock = []string{line}
			isTargetServiceConfig = false
		} else {
			serviceConfigBlock = append(serviceConfigBlock, line)
		}

		if strings.Contains(line, "import-path:") {
			path := strings.TrimSpace(strings.Split(line, ":")[1])
			if strings.HasPrefix(path, "cloud.google.com/go/"+moduleName+"/") {
				isTargetServiceConfig = true
			}
		}
	}
	// flush last block
	if inServiceConfigs && !isTargetServiceConfig {
		newLines = append(newLines, serviceConfigBlock...)
	}

	// Third pass: remove from skip-module-scan-paths if present.
	lines = newLines
	newLines = []string{}
	inSkipScanPaths := false
	for _, line := range lines {
		if strings.HasPrefix(line, "skip-module-scan-paths:") {
			inSkipScanPaths = true
		} else if inSkipScanPaths && !strings.HasPrefix(line, " ") { // top-level key
			inSkipScanPaths = false
		}
		if inSkipScanPaths && strings.TrimSpace(line) == "- "+moduleName {
			continue
		}
		newLines = append(newLines, line)
	}

	// Fourth pass: add to skip-module-scan-paths.
	lines = newLines
	newLines = []string{}
	skipScanPathsIndex := -1
	librarianReleasedIndex := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "skip-module-scan-paths:") {
			skipScanPathsIndex = i
		}
		if skipScanPathsIndex != -1 && strings.TrimSpace(line) == "# Librarian released modules" {
			librarianReleasedIndex = i
			break // Found it, no need to continue.
		}
	}

	if skipScanPathsIndex == -1 || librarianReleasedIndex == -1 {
		return fmt.Errorf("'skip-module-scan-paths:' or '# Librarian released modules' not found in postprocessor config")
	}

	// Reconstruct the file with the new line added.
	newLines = append(newLines, lines[:librarianReleasedIndex+1]...)
	newLines = append(newLines, "  - "+moduleName)
	newLines = append(newLines, lines[librarianReleasedIndex+1:]...)

	output := strings.Join(newLines, "\n")
	if bytes.Equal([]byte(output), fileBytes) {
		return nil
	}

	return os.WriteFile(configPath, []byte(output), 0644)
}

// cleanupOwlBotYaml removes entries for a given module from the
// .github/.OwlBot.yaml file.
func cleanupOwlBotYaml(repoRoot, moduleName string) error {
	owlBotPath := filepath.Join(repoRoot, ".github", ".OwlBot.yaml")
	fileBytes, err := os.ReadFile(owlBotPath)
	if err != nil {
		return err
	}

	// Also check for a cleanup name, which may be different from the module name.
	// For example, the cloudtasks module has the proto path .../cloud/tasks.
	// We can derive this name from the postprocessor config.
	ppc, err := loadPostProcessorConfig(filepath.Join(repoRoot, "internal/postprocessor/config.yaml"))
	if err != nil {
		return fmt.Errorf("loading postprocessor config: %w", err)
	}
	importPrefix := "cloud.google.com/go/" + moduleName + "/"
	modulePathFragment := "/" + moduleName + "/"

	lines := strings.Split(string(fileBytes), "\n")
	var newLines []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// If it's a source line, compare it with all of the import paths for service configs for this module.
		if strings.Contains(line, "source:") {
			foundSource := false
			for _, sc := range ppc.ServiceConfigs {
				if strings.HasPrefix(sc.ImportPath, importPrefix) && strings.Contains(line, "/"+sc.InputDirectory+"/") {
					if i+1 < len(lines) {
						i++ // Remove both source and dest lines.
					}
					foundSource = true
					break
				}
			}
			if foundSource {
				continue
			}
		}

		if strings.Contains(line, modulePathFragment) {
			// Remove any non-source line containing the module name.
			continue
		}
		newLines = append(newLines, line)
	}

	output := strings.Join(newLines, "\n")
	if bytes.Equal([]byte(output), fileBytes) {
		return nil
	}
	return os.WriteFile(owlBotPath, []byte(output), 0644)
}

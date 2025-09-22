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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// cleanupLegacyConfigs is the main entrypoint for removing legacy configuration
// for a newly migrated module.
func cleanupLegacyConfigs(repoRoot, moduleName string) error {
	if err := cleanupOwlBotYaml(repoRoot, moduleName); err != nil {
		return fmt.Errorf("cleaning up .OwlBot.yaml: %w", err)
	}
	if err := cleanupPostProcessorConfig(repoRoot, moduleName); err != nil {
		return fmt.Errorf("cleaning up postprocessor config: %w", err)
	}

	// JSON files to clean up.
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

// cleanupJSONFile removes a module entry from a given JSON file. It handles
// both manifest files (toplevel keys) and release-please config files (keys
// under a "packages" object).
func cleanupJSONFile(path, moduleName string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		// Not all files will exist in all contexts, so skip if not found.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return err
	}

	// release-please-config files have a "packages" key.
	if packages, ok := data["packages"].(map[string]interface{}); ok {
		delete(packages, moduleName)
	} else {
		// Manifest files have the module name as a top-level key.
		delete(data, moduleName)
	}

	outBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, outBytes, 0644)
}

// cleanupPostProcessorConfig removes the module and service-config entries for
// a given module from the internal/postprocessor/config.yaml file.
func cleanupPostProcessorConfig(repoRoot, moduleName string) error {
	configPath := filepath.Join(repoRoot, "internal", "postprocessor", "config.yaml")
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	// We can't use postProcessorConfig from config.go because it's a partial
	// view of the file. We need the full structure to write it back correctly.
	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal(bytes, &fullConfig); err != nil {
		return err
	}

	// Clean up the modules list.
	modulesKey := "modules"
	modules, ok := fullConfig[modulesKey].([]interface{})
	if !ok {
		return fmt.Errorf("key %q not found or not a slice", modulesKey)
	}
	var newModules []string
	for _, m := range modules {
		mod, ok := m.(string)
		if !ok {
			continue
		}
		if mod != moduleName {
			newModules = append(newModules, mod)
		}
	}
	fullConfig[modulesKey] = newModules

	// Clean up the service-configs list.
	serviceConfigsKey := "service-configs"
	serviceConfigs, ok := fullConfig[serviceConfigsKey].([]interface{})
	if !ok {
		return fmt.Errorf("key %q not found or not a slice", serviceConfigsKey)
	}
	var newServiceConfigs []interface{}
	importPrefix := "cloud.google.com/go/" + moduleName
	for _, sc := range serviceConfigs {
		serviceConfig, ok := sc.(map[string]interface{})
		if !ok {
			continue
		}
		importPath, ok := serviceConfig["import-path"].(string)
		if !ok {
			continue
		}
		if !strings.HasPrefix(importPath, importPrefix) {
			newServiceConfigs = append(newServiceConfigs, sc)
		}
	}
	fullConfig[serviceConfigsKey] = newServiceConfigs

	outBytes, err := yaml.Marshal(fullConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, outBytes, 0644)
}

// cleanupOwlBotYaml removes the deep-remove-regex entries for a given module
// from the .github/.OwlBot.yaml file.
func cleanupOwlBotYaml(repoRoot, moduleName string) error {
	owlBotPath := filepath.Join(repoRoot, ".github", ".OwlBot.yaml")
	bytes, err := os.ReadFile(owlBotPath)
	if err != nil {
		return err
	}

	var owlBotConfig map[string]interface{}
	if err := yaml.Unmarshal(bytes, &owlBotConfig); err != nil {
		return err
	}

	key := "deep-remove-regex"
	regexes, ok := owlBotConfig[key].([]interface{})
	if !ok {
		return fmt.Errorf("key %q not found or not a slice", key)
	}

	var newRegexes []string
	prefix1 := "/" + moduleName + "/"
	prefix2 := "/internal/generated/snippets/" + moduleName + "/"

	for _, r := range regexes {
		regex, ok := r.(string)
		if !ok {
			continue
		}
		if !strings.HasPrefix(regex, prefix1) && !strings.HasPrefix(regex, prefix2) {
			newRegexes = append(newRegexes, regex)
		}
	}

	owlBotConfig[key] = newRegexes
	outBytes, err := yaml.Marshal(owlBotConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(owlBotPath, outBytes, 0644)
}

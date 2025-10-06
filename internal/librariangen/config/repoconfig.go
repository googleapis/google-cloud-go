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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// GeneratorInputDir is the name of the directory containing the repo configuration
// file, relative to the .librarian directory.
const GeneratorInputDir string = "generator-input"

// RepoConfigFile is the name of the repo configuration file, relative to
// GeneratorInputDir.
const RepoConfigFile string = "repo-config.yaml"

// RepoConfig is the configuration for all modules in the repository.
type RepoConfig struct {
	// Modules is the list of all the modules in the repository which need overrides.
	Modules []*ModuleConfig `yaml:"modules"`
}

// ModuleConfig is the configuration for a single module.
type ModuleConfig struct {
	// Name is the top-level name of the module, matching the top-level
	// subdirectory.
	Name string `yaml:"name"`
	// ModulePathVersion is the major version for the overall module, e.g. "v2"
	// to create a module path of cloud.google.com/go/{Name}/v2
	ModulePathVersion string `yaml:"module_path_version"`
	// APIs is the list of APIs within this module (that need overrides).
	APIs []*APIConfig `yaml:"apis"`
}

// APIConfig provides per-API configuration to override defaults,
// e.g. for snippet metadata locations.
type APIConfig struct {
	// Path is the Path within googleapis, e.g. "google/cloud/functions/v2"
	Path string `yaml:"path"`
	// ProtoPackage is the protobuf package, when it doesn't match the Path
	// (after replacing slash with period).
	ProtoPackage string `yaml:"proto_package"`
	// ClientDirectory is the directory containing the client code, relative to the module root.
	// (This is currently only used to find snippet metadata files.)
	ClientDirectory string `yaml:"client_directory"`
	// DisableGAPIC is a flag to disable GAPIC generation for an API, overriding
	// settings from the BUILD.bazel file.
	DisableGAPIC bool `yaml:"disable_gapic"`
	// ModuleName is the name of the module this API config belongs to.
	// This is only exported for ease of testing, and is not expected to be
	// present in the YAML file. It is populated when the APIConfig is returned
	// from GetAPIConfig().
	ModuleName string
}

// LoadRepoConfig loads the repository configuration with module-specific overrides,
// from a file derived from the .librarian directory (specified as librarianDir).
// The absence of the file is not an error; it's equivalent to an empty file being present.
func LoadRepoConfig(librarianDir string) (*RepoConfig, error) {
	var config RepoConfig
	b, err := os.ReadFile(filepath.Join(librarianDir, GeneratorInputDir, RepoConfigFile))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// GetModuleConfig returns the configuration for the named module
// (top-level directory). If no module-specific configuration is found,
// an empty configuration (with the right name) is returned.
func (rc *RepoConfig) GetModuleConfig(name string) *ModuleConfig {
	for _, moduleConfig := range rc.Modules {
		if moduleConfig.Name == name {
			return moduleConfig
		}
	}
	return &ModuleConfig{
		Name: name,
	}
}

// GetAPIConfig returns the configuration for the API identified by
// its path within googleapis (e.g. "google/cloud/functions/v2").
// If no API-specific configuration is found, an empty configuration
// (with the right name) is returned.
func (mc *ModuleConfig) GetAPIConfig(path string) *APIConfig {
	for _, apiConfig := range mc.APIs {
		if apiConfig.Path == path {
			apiConfig.ModuleName = mc.Name
			return apiConfig
		}
	}
	return &APIConfig{
		Path:       path,
		ModuleName: mc.Name,
	}
}

// GetModulePath returns the module path for the module, applying
// any configured version.
func (mc *ModuleConfig) GetModulePath() string {
	prefix := "cloud.google.com/go/" + mc.Name
	if mc.ModulePathVersion != "" {
		return prefix + "/" + mc.ModulePathVersion
	}

	// No override: assume implicit v1.
	return prefix
}

// GetProtoPackage returns the protobuf package for the API config,
// which is derived from the path unless overridden.
func (ac *APIConfig) GetProtoPackage() string {
	if ac.ProtoPackage != "" {
		return ac.ProtoPackage
	}

	// No override: derive the value.
	return strings.ReplaceAll(ac.Path, "/", ".")
}

// GetClientDirectory returns the directory for the clients of this
// API, relative to the module root.
func (ac *APIConfig) GetClientDirectory() (string, error) {
	if ac.ClientDirectory != "" {
		return ac.ClientDirectory, nil
	}

	// No override: derive the value.
	startOfModuleName := strings.Index(ac.Path, ac.ModuleName+"/")
	if startOfModuleName == -1 {
		return "", fmt.Errorf("librariangen: unexpected API path format: %s", ac.Path)
	}

	// google/spanner/v1 => ["google", "spanner", "v1"]
	// google/spanner/admin/instance/v1 => ["google", "spanner", "admin", "instance", "v1"]
	parts := strings.Split(ac.Path, "/")
	moduleIndex := slices.Index(parts, ac.ModuleName)
	if moduleIndex == -1 {
		return "", fmt.Errorf("librariangen: module name '%s' not found in API path '%s'", ac.ModuleName, ac.Path)
	}

	// Remove everything up to and include the module name.
	// google/spanner/v1 => ["v1"]
	// google/spanner/admin/instance/v1 => ["admin", "instance", "v1"]
	parts = parts[moduleIndex+1:]
	parts[len(parts)-1] = "api" + parts[len(parts)-1]
	return strings.Join(parts, "/"), nil
}

// HasDisableGAPIC returns a value saying whether GAPIC generation is explicitly
// disabled for this module.
func (ac *APIConfig) HasDisableGAPIC() bool {
	return ac.DisableGAPIC
}

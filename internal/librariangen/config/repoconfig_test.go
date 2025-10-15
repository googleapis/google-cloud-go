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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLoadRepoConfig_success(t *testing.T) {
	tempDir := t.TempDir()

	const configFileContent = `
modules:
  - name: maps
    apis:
      - path: google/maps/fleetengine/v1
        proto_package: maps.fleetengine.v1
      - path: google/maps/fleetengine/delivery/v1
        proto_package: maps.fleetengine.delivery.v1
  - name: monitoring
    apis:
      - path: google/monitoring/v3
        client_directory: apiv3/v2
  - name: recaptchaenterprise
    module_path_version: v2
`

	generatorInputDir := filepath.Join(tempDir, GeneratorInputDir)
	if err := os.Mkdir(generatorInputDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(generatorInputDir, RepoConfigFile)
	if err := os.WriteFile(configPath, []byte(configFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	want := &RepoConfig{
		Modules: []*ModuleConfig{
			{
				Name: "maps",
				APIs: []*APIConfig{
					{
						Path:         "google/maps/fleetengine/v1",
						ProtoPackage: "maps.fleetengine.v1",
					},
					{
						Path:         "google/maps/fleetengine/delivery/v1",
						ProtoPackage: "maps.fleetengine.delivery.v1",
					},
				},
			},
			{
				Name: "monitoring",
				APIs: []*APIConfig{
					{
						Path:            "google/monitoring/v3",
						ClientDirectory: "apiv3/v2",
					},
				},
			},
			{
				Name:              "recaptchaenterprise",
				ModulePathVersion: "v2",
			},
		},
	}

	got, err := LoadRepoConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadRepoConfig() failed: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LoadRepoConfig() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadRepoConfig_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	got, err := LoadRepoConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadRepoConfig() failed: %v", err)
	}
	want := &RepoConfig{}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LoadRepoConfig() mismatch (-want +got):\n%s", diff)
	}
}

func TestGetModuleConfig(t *testing.T) {
	config := &RepoConfig{
		Modules: []*ModuleConfig{
			{
				Name:              "spanner",
				ModulePathVersion: "v2",
			},
			{Name: "functions"},
		},
	}

	for _, test := range []struct {
		name       string
		moduleName string
		want       *ModuleConfig
	}{
		{
			name:       "present in config",
			moduleName: "spanner",
			want: &ModuleConfig{
				Name:              "spanner",
				ModulePathVersion: "v2",
			},
		},
		{
			name:       "absent in config",
			moduleName: "other",
			want: &ModuleConfig{
				Name: "other",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := config.GetModuleConfig(test.moduleName)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("GetModuleConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetAPIConfig(t *testing.T) {
	config := &ModuleConfig{
		Name: "spanner",
		APIs: []*APIConfig{
			{
				Path:         "google/spanner/v1",
				ProtoPackage: "spanner.v1",
			},
			{
				Path: "google/spanner/admin/instance/v1",
			},
		},
	}

	for _, test := range []struct {
		name    string
		apiPath string
		want    *APIConfig
	}{
		{
			name:    "present in config",
			apiPath: "google/spanner/v1",
			want: &APIConfig{
				Path:         "google/spanner/v1",
				ProtoPackage: "spanner.v1",
				ModuleName:   "spanner",
			},
		},
		{
			name:    "absent in config",
			apiPath: "google/spanner/other/v1",
			want: &APIConfig{
				Path:       "google/spanner/other/v1",
				ModuleName: "spanner",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := config.GetAPIConfig(test.apiPath)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("GetAPIConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetModulePath(t *testing.T) {
	for _, test := range []struct {
		name   string
		config *ModuleConfig
		want   string
	}{
		{
			name: "version specified",
			config: &ModuleConfig{
				Name:              "spanner",
				ModulePathVersion: "v2",
			},
			want: "cloud.google.com/go/spanner/v2",
		},
		{
			name: "no version specified",
			config: &ModuleConfig{
				Name: "spanner",
			},
			want: "cloud.google.com/go/spanner",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := test.config.GetModulePath()
			if got != test.want {
				t.Errorf("GetModulePath() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGetProtoPackage(t *testing.T) {
	for _, test := range []struct {
		name   string
		config *APIConfig
		want   string
	}{
		{
			name: "override present",
			config: &APIConfig{
				Path:         "google/spanner/v1",
				ProtoPackage: "override.package",
			},
			want: "override.package",
		},
		{
			name: "no override",
			config: &APIConfig{
				Path: "google/spanner/v1",
			},
			want: "google.spanner.v1",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := test.config.GetProtoPackage()
			if got != test.want {
				t.Errorf("GetProtoPackage() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGetClientDirectory(t *testing.T) {
	for _, test := range []struct {
		name      string
		config    *APIConfig
		want      string
		wantError bool
	}{
		{
			name: "override present",
			config: &APIConfig{
				ClientDirectory: "override/directory",
			},
			want: "override/directory",
		},
		{
			name: "simple path",
			config: &APIConfig{
				Path:       "google/spanner/v1",
				ModuleName: "spanner",
			},
			want: "apiv1",
		},
		{
			name: "complex path",
			config: &APIConfig{
				Path:       "google/spanner/admin/instance/v1",
				ModuleName: "spanner",
			},
			want: "admin/instance/apiv1",
		},
		{
			name: "module not in path",
			config: &APIConfig{
				Path:       "google/spanner/v1",
				ModuleName: "wrongmodule",
			},
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.config.GetClientDirectory()
			if (err != nil) != test.wantError {
				t.Fatalf("GetClientDirectory() error = %v, wantError %v", err, test.wantError)
			}
			if got != test.want {
				t.Errorf("GetClientDirectory() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHasDisableGAPIC(t *testing.T) {
	for _, test := range []struct {
		name   string
		config *APIConfig
		want   bool
	}{
		{
			name: "true",
			config: &APIConfig{
				DisableGAPIC: true,
			},
			want: true,
		},
		{
			name:   "default",
			config: &APIConfig{},
			want:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.config.HasDisableGAPIC(); got != test.want {
				t.Errorf("HasDisableGAPIC() = %v, want %v", got, test.want)
			}
		})
	}
}

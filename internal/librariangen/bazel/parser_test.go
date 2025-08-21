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

package bazel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	content := `
go_grpc_library(
    name = "asset_go_proto",
    importpath = "cloud.google.com/go/asset/apiv1/assetpb",
    protos = [":asset_proto"],
)

go_gapic_library(
    name = "asset_go_gapic",
    srcs = [":asset_proto_with_info"],
    grpc_service_config = "cloudasset_grpc_service_config.json",
    importpath = "cloud.google.com/go/asset/apiv1;asset",
    metadata = True,
    release_level = "ga",
    rest_numeric_enums = True,
    service_yaml = "cloudasset_v1.yaml",
    transport = "grpc+rest",
    diregapic = False,
)
`
	tmpDir := t.TempDir()
	buildPath := filepath.Join(tmpDir, "BUILD.bazel")
	if err := os.WriteFile(buildPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	got, err := Parse(tmpDir)
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if want := "cloud.google.com/go/asset/apiv1;asset"; got.GAPICImportPath() != want {
		t.Errorf("GAPICImportPath() = %q; want %q", got.GAPICImportPath(), want)
	}
	if want := "cloudasset_v1.yaml"; got.ServiceYAML() != want {
		t.Errorf("ServiceYAML() = %q; want %q", got.ServiceYAML(), want)
	}
	if want := "cloudasset_grpc_service_config.json"; got.GRPCServiceConfig() != want {
		t.Errorf("GRPCServiceConfig() = %q; want %q", got.GRPCServiceConfig(), want)
	}
	if want := "grpc+rest"; got.Transport() != want {
		t.Errorf("Transport() = %q; want %q", got.Transport(), want)
	}
	if want := "ga"; got.ReleaseLevel() != want {
		t.Errorf("ReleaseLevel() = %q; want %q", got.ReleaseLevel(), want)
	}
	if !got.HasMetadata() {
		t.Error("HasMetadata() = false; want true")
	}
	if got.HasDiregapic() {
		t.Error("HasDiregapic() = true; want false")
	}
	if !got.HasRESTNumericEnums() {
		t.Error("HasRESTNumericEnums() = false; want true")
	}
}

func TestParse_misconfiguration(t *testing.T) {
	content := `
go_grpc_library()

go_proto_library()
`
	tmpDir := t.TempDir()
	buildPath := filepath.Join(tmpDir, "BUILD.bazel")
	if err := os.WriteFile(buildPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if _, err := Parse(tmpDir); err == nil {
		t.Error("Parse() succeeded; want error")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg: &Config{
				gapicImportPath:   "a",
				serviceYAML:       "b",
				grpcServiceConfig: "c",
				transport:         "d",
			},
			wantErr: false,
		},
		{
			name:    "missing gapicImportPath",
			cfg:     &Config{serviceYAML: "b", grpcServiceConfig: "c", transport: "d"},
			wantErr: true,
		},
		{
			name:    "missing serviceYAML",
			cfg:     &Config{gapicImportPath: "a", grpcServiceConfig: "c", transport: "d"},
			wantErr: true,
		},
		{
			name:    "missing grpcServiceConfig",
			cfg:     &Config{gapicImportPath: "a", serviceYAML: "b", transport: "d"},
			wantErr: true,
		},
		{
			name:    "missing transport",
			cfg:     &Config{gapicImportPath: "a", serviceYAML: "b", grpcServiceConfig: "c"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

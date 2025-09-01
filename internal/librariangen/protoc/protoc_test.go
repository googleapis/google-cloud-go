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

package protoc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
	"github.com/google/go-cmp/cmp"
)

// mockConfigProvider is a mock implementation of the ConfigProvider interface for testing.
type mockConfigProvider struct {
	gapicImportPath   string
	serviceYAML       string
	grpcServiceConfig string
	transport         string
	releaseLevel      string
	metadata          bool
	diregapic         bool
	restNumericEnums  bool
	hasGoGRPC         bool
	hasGAPIC          bool
	hasLegacyGRPC     bool
}

func (m *mockConfigProvider) GAPICImportPath() string   { return m.gapicImportPath }
func (m *mockConfigProvider) ServiceYAML() string       { return m.serviceYAML }
func (m *mockConfigProvider) GRPCServiceConfig() string { return m.grpcServiceConfig }
func (m *mockConfigProvider) Transport() string         { return m.transport }
func (m *mockConfigProvider) ReleaseLevel() string      { return m.releaseLevel }
func (m *mockConfigProvider) HasMetadata() bool         { return m.metadata }
func (m *mockConfigProvider) HasDiregapic() bool        { return m.diregapic }
func (m *mockConfigProvider) HasRESTNumericEnums() bool { return m.restNumericEnums }
func (m *mockConfigProvider) HasGoGRPC() bool           { return m.hasGoGRPC }
func (m *mockConfigProvider) HasGAPIC() bool            { return m.hasGAPIC }
func (m *mockConfigProvider) HasLegacyGRPC() bool       { return m.hasLegacyGRPC }

func TestBuild(t *testing.T) {
	// The testdata directory is a curated version of a valid protoc
	// import path that contains all the necessary proto definitions.
	sourceDir, err := filepath.Abs("../testdata/generate/source")
	if err != nil {
		t.Fatalf("failed to get absolute path for sourceDir: %v", err)
	}
	tests := []struct {
		name          string
		apiPath       string
		apiServiceDir string
		reqID         string
		config        mockConfigProvider
		want          []string
	}{
		{
			name:    "go_grpc_library rule",
			apiPath: "google/cloud/workflows/v1",
			reqID:   "workflows",
			config: mockConfigProvider{
				gapicImportPath:   "cloud.google.com/go/workflows/apiv1;workflows",
				transport:         "grpc",
				grpcServiceConfig: "workflows_grpc_service_config.json",
				serviceYAML:       "workflows_v1.yaml",
				releaseLevel:      "ga",
				metadata:          true,
				restNumericEnums:  true,
				hasGoGRPC:         true,
				hasGAPIC:          true,
			},
			want: []string{
				"protoc",
				"--experimental_allow_proto3_optional",
				"--go_out=/output",
				"--go-grpc_out=/output",
				"--go-grpc_opt=require_unimplemented_servers=false",
				"--go_gapic_out=/output",
				"--go_gapic_opt=go-gapic-package=cloud.google.com/go/workflows/apiv1;workflows",
				"--go_gapic_opt=api-service-config=" + filepath.Join(sourceDir, "google/cloud/workflows/v1/workflows_v1.yaml"),
				"--go_gapic_opt=grpc-service-config=" + filepath.Join(sourceDir, "google/cloud/workflows/v1/workflows_grpc_service_config.json"),
				"--go_gapic_opt=transport=grpc",
				"--go_gapic_opt=release-level=ga",
				"--go_gapic_opt=metadata",
				"--go_gapic_opt=rest-numeric-enums",
				"-I=" + sourceDir,
				filepath.Join(sourceDir, "google/cloud/workflows/v1/workflows.proto"),
			},
		},
		{
			name:    "go_proto_library rule with legacy gRPC",
			apiPath: "google/cloud/secretmanager/v1beta2",
			reqID:   "secretmanager",
			config: mockConfigProvider{
				gapicImportPath:   "cloud.google.com/go/secretmanager/apiv1beta2;secretmanager",
				transport:         "grpc",
				grpcServiceConfig: "secretmanager_grpc_service_config.json",
				serviceYAML:       "secretmanager_v1beta2.yaml",
				releaseLevel:      "ga",
				metadata:          true,
				restNumericEnums:  true,
				hasGoGRPC:         false,
				hasGAPIC:          true,
				hasLegacyGRPC:     true,
			},
			want: []string{
				"protoc",
				"--experimental_allow_proto3_optional",
				"--go_v1_out=/output",
				"--go_v1_opt=plugins=grpc",
				"--go_gapic_out=/output",
				"--go_gapic_opt=go-gapic-package=cloud.google.com/go/secretmanager/apiv1beta2;secretmanager",
				"--go_gapic_opt=api-service-config=" + filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager_v1beta2.yaml"),
				"--go_gapic_opt=grpc-service-config=" + filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager_grpc_service_config.json"),
				"--go_gapic_opt=transport=grpc",
				"--go_gapic_opt=release-level=ga",
				"--go_gapic_opt=metadata",
				"--go_gapic_opt=rest-numeric-enums",
				"-I=" + sourceDir,
				filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager.proto"),
			},
		},
		{
			name:    "go_proto_library rule without legacy gRPC",
			apiPath: "google/cloud/secretmanager/v1beta2",
			reqID:   "secretmanager",
			config: mockConfigProvider{
				gapicImportPath:   "cloud.google.com/go/secretmanager/apiv1beta2;secretmanager",
				transport:         "grpc",
				grpcServiceConfig: "secretmanager_grpc_service_config.json",
				serviceYAML:       "secretmanager_v1beta2.yaml",
				releaseLevel:      "ga",
				metadata:          true,
				restNumericEnums:  true,
				hasGoGRPC:         false,
				hasGAPIC:          true,
			},
			want: []string{
				"protoc",
				"--experimental_allow_proto3_optional",
				"--go_v1_out=/output",
				"--go_gapic_out=/output",
				"--go_gapic_opt=go-gapic-package=cloud.google.com/go/secretmanager/apiv1beta2;secretmanager",
				"--go_gapic_opt=api-service-config=" + filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager_v1beta2.yaml"),
				"--go_gapic_opt=grpc-service-config=" + filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager_grpc_service_config.json"),
				"--go_gapic_opt=transport=grpc",
				"--go_gapic_opt=release-level=ga",
				"--go_gapic_opt=metadata",
				"--go_gapic_opt=rest-numeric-enums",
				"-I=" + sourceDir,
				filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager.proto"),
			},
		},
		{
			// Note: we don't have a separate test directory with a proto-only library;
			// the config is used to say "don't generate GAPIC".
			name:    "proto-only",
			apiPath: "google/cloud/secretmanager/v1beta2",
			reqID:   "secretmanager",
			config: mockConfigProvider{
				hasGoGRPC: false,
				hasGAPIC:  false,
			},
			want: []string{
				"protoc",
				"--experimental_allow_proto3_optional",
				"--go_v1_out=/output",
				"-I=" + sourceDir,
				filepath.Join(sourceDir, "google/cloud/secretmanager/v1beta2/secretmanager.proto"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &request.Request{
				ID: tt.reqID,
			}
			api := &request.API{
				Path: tt.apiPath,
			}

			got, err := Build(req, api, filepath.Join(sourceDir, tt.apiPath), &tt.config, sourceDir, "/output")
			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Build() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRun_Integration(t *testing.T) {
	// Perform a "health check" on the required protoc plugins. Instead of just
	// checking for their existence (which could lead to a hanging test if the
	// binary is broken), we run them with a --version flag and a timeout.
	plugins := []string{"protoc-gen-go", "protoc-gen-go_gapic"}
	for _, plugin := range plugins {
		if _, err := exec.LookPath(plugin); err != nil {
			t.Skipf("%s not found in PATH, skipping integration test", plugin)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, plugin, "--version").Run(); err != nil {
			t.Skipf("%s is not responsive (--version failed or timed out), skipping integration test", plugin)
		}
	}
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH, skipping integration test")
	}

	// Setup temporary directory for the output.
	outputDir := t.TempDir()

	// The testdata/generate/source directory is a miniature version of a valid protoc
	// import path that contains all the necessary proto definitions.
	sourceDir, err := filepath.Abs("../testdata/generate/source")
	if err != nil {
		t.Fatalf("failed to get absolute path for sourceDir: %v", err)
	}
	protoPath := filepath.Join(sourceDir, "google/cloud/workflows/v1/workflows.proto")

	args := []string{
		"protoc",
		"--go_out=" + outputDir,
		"-I=" + sourceDir,
		protoPath,
	}

	if err := execv.Run(context.Background(), args, outputDir); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	// Check that a .pb.go file was created somewhere in the output directory.
	// We don't check the exact path because it's determined by the go_package
	// option in the proto file, and we don't want the test to be too brittle.
	var found bool
	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".pb.go") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk output directory: %v", err)
	}

	if !found {
		t.Error("Run() did not create any .pb.go files")
	}
}

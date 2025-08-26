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
	"fmt"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

// ConfigProvider is an interface that describes the configuration needed
// by the Build function. This allows the protoc package to be decoupled
// from the source of the configuration (e.g., Bazel files, JSON, etc.).
type ConfigProvider interface {
	GAPICImportPath() string
	ServiceYAML() string
	GRPCServiceConfig() string
	Transport() string
	ReleaseLevel() string
	HasMetadata() bool
	HasDiregapic() bool
	HasRESTNumericEnums() bool
	HasGoGRPC() bool
}

// Build constructs the full protoc command arguments for a given API.
func Build(lib *request.Request, api *request.API, apiServiceDir string, config ConfigProvider, sourceDir, outputDir string) ([]string, error) {
	// Gather all .proto files in the API's source directory.
	entries, err := os.ReadDir(apiServiceDir)
	if err != nil {
		return nil, fmt.Errorf("librariangen: failed to read API source directory %s: %w", apiServiceDir, err)
	}

	var protoFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".proto" {
			protoFiles = append(protoFiles, filepath.Join(apiServiceDir, entry.Name()))
		}
	}

	if len(protoFiles) == 0 {
		return nil, fmt.Errorf("librariangen: no .proto files found in %s", apiServiceDir)
	}

	// Construct the protoc command arguments.
	var gapicOpts []string
	gapicOpts = append(gapicOpts, "go-gapic-package="+config.GAPICImportPath())
	if config.ServiceYAML() != "" {
		gapicOpts = append(gapicOpts, fmt.Sprintf("api-service-config=%s", filepath.Join(apiServiceDir, config.ServiceYAML())))
	}
	if config.GRPCServiceConfig() != "" {
		gapicOpts = append(gapicOpts, fmt.Sprintf("grpc-service-config=%s", filepath.Join(apiServiceDir, config.GRPCServiceConfig())))
	}
	if config.Transport() != "" {
		gapicOpts = append(gapicOpts, fmt.Sprintf("transport=%s", config.Transport()))
	}
	if config.ReleaseLevel() != "" {
		gapicOpts = append(gapicOpts, fmt.Sprintf("release-level=%s", config.ReleaseLevel()))
	}
	if config.HasMetadata() {
		gapicOpts = append(gapicOpts, "metadata")
	}
	if config.HasDiregapic() {
		gapicOpts = append(gapicOpts, "diregapic")
	}
	if config.HasRESTNumericEnums() {
		gapicOpts = append(gapicOpts, "rest-numeric-enums")
	}

	args := []string{
		"protoc",
		"--experimental_allow_proto3_optional",
	}
	// All generated files are written to the /output directory.
	// Which plugin(s) we use depends on whether the Bazel rule was go_grpc_library
	// or go_proto_library:
	// - If we're using go_rpc, we use the newer go plugin and the go-grpc plugin
	// - Otherwise, use the "old" plugin (built explicitly in the Dockerfile)
	if config.HasGoGRPC() {
		args = append(args, "--go_out="+outputDir, "--go-grpc_out="+outputDir, "--go-grpc_opt=require_unimplemented_servers=false")
	} else {
		args = append(args, "--go_v1_out="+outputDir, "--go_v1_opt=plugins=grpc")
	}
	args = append(args, "--go_gapic_out="+outputDir)

	for _, opt := range gapicOpts {
		args = append(args, "--go_gapic_opt="+opt)
	}
	args = append(args,
		// The -I flag specifies the import path for protoc. All protos
		// and their dependencies must be findable from this path.
		// The /source mount contains the complete googleapis repository.
		"-I="+sourceDir,
	)

	args = append(args, protoFiles...)

	return args, nil
}

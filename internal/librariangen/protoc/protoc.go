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
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		return nil, fmt.Errorf("failed to read API source directory %s: %w", apiServiceDir, err)
	}

	var protoFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".proto" {
			protoFiles = append(protoFiles, filepath.Join(apiServiceDir, entry.Name()))
		}
	}

	if len(protoFiles) == 0 {
		return nil, fmt.Errorf("no .proto files found in %s", apiServiceDir)
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
		// All generated files are written to the /output directory.
		"--go_out=" + outputDir,
	}
	if config.HasGoGRPC() {
		args = append(args, "--go-grpc_out="+outputDir)
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

// Run executes a command and logs its output.
func Run(ctx context.Context, args []string, outputDir string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Dir = outputDir // Run commands from the output directory.
	slog.Debug("running command", "command", strings.Join(cmd.Args, " "), "dir", cmd.Dir)

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		slog.Debug("command output", "output", string(output))
	}
	if err != nil {
		return fmt.Errorf("command failed with error: %w", err)
	}
	return nil
}

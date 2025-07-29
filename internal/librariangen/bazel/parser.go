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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bazelbuild/buildtools/build"
)

// Config holds configuration extracted from the Go rules in a googleapis BUILD.bazel file.
// The fields are from the Go rules in a API version BUILD.bazel file.
// E.g., googleapis/google/cloud/asset/v1/BUILD.bazel
// Note that not all fields are present in every Bazel rule usage.
type Config struct {
	// The fields below are all from the go_gapic_library rule.
	grpcServiceConfig string
	gapicImportPath   string
	metadata          bool
	releaseLevel      string
	restNumericEnums  bool
	serviceYAML       string
	transport         string
	diregapic         bool

	// Meta configuration
	// TODO(quartzmo): Remove this field once the googleapis migration from go_proto_library
	// to go_grpc_library is complete.
	hasGoGRPC bool
}

// GAPICImportPath is importpath in the go_gapic_library rule.
// The Go package name is typically appended to the end, separated by a `;`.
// E.g., cloud.google.com/go/asset/apiv1;asset
func (c *Config) GAPICImportPath() string { return c.gapicImportPath }

// ModulePath returns the module path from the GAPIC import path.
// E.g., "cloud.google.com/go/chronicle/apiv1;chronicle" -> "cloud.google.com/go/chronicle/apiv1"
func (c *Config) ModulePath() string {
	if idx := strings.Index(c.gapicImportPath, ";"); idx != -1 {
		return c.gapicImportPath[:idx]
	}
	return c.gapicImportPath
}

// ServiceYAML is the client config file in the API version directory in googleapis.
// This is a required input to the GAPIC generator.
// E.g., googleapis/google/cloud/asset/v1/cloudasset_v1.yaml
func (c *Config) ServiceYAML() string { return c.serviceYAML }

// GRPCServiceConfig is name of the gRPC service config JSON file.
// E.g., cloudasset_grpc_service_config.json
func (c *Config) GRPCServiceConfig() string { return c.grpcServiceConfig }

// Transport is typically one of "grpc", "rest" or "grpc+rest".
func (c *Config) Transport() string { return c.transport }

// ReleaseLevel is typically one of "beta", "" (same as beta) or "ga".
// If "ga", gapic-generator-go does not print a warning in the package docs.
func (c *Config) ReleaseLevel() string { return c.releaseLevel }

// HasMetadata indicates whether a gapic_metadata.json should be generated.
// This is typically true.
func (c *Config) HasMetadata() bool { return c.metadata }

// HasDiregapic indicates whether generation uses DIREGAPIC (Discovery REST GAPICs).
// This is typically false or not present. Used for the GCE (compute) client.
func (c *Config) HasDiregapic() bool { return c.diregapic }

// HasRESTNumericEnums indicates whether the generated Go REST client should support
// numeric enums. This is typically true.
func (c *Config) HasRESTNumericEnums() bool { return c.restNumericEnums }

// HasGoGRPC is meta-configuration that indicates if a go_grpc_library rule is used
// instead of a go_proto_library in the BUILD.bazel file. This is not part of the
// BUILD.bazel configuration passed to the GAPIC generator. If true, --go-grpc_out
// is passed to the protoc command. Will be removed once the googleapis migration
// from go_proto_library to go_grpc_library is complete and --go-grpc_out is always
// used. This is trending toward typically true.
func (c *Config) HasGoGRPC() bool { return c.hasGoGRPC }

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.gapicImportPath == "" {
		return fmt.Errorf("gapicImportPath is not set")
	}
	if c.serviceYAML == "" {
		return fmt.Errorf("serviceYAML is not set")
	}
	if c.grpcServiceConfig == "" {
		return fmt.Errorf("grpcServiceConfig is not set")
	}
	if c.transport == "" {
		return fmt.Errorf("transport is not set")
	}
	return nil
}

// Parse reads a BUILD.bazel file from the given directory and extracts the
// relevant configuration from the go_gapic_library and go_proto_library rules.
func Parse(dir string) (*Config, error) {
	c := &Config{}
	fp := filepath.Join(dir, "BUILD.bazel")
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to read BUILD.bazel file %s: %w", fp, err)
	}
	f, err := build.Parse(fp, data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse BUILD.bazel file %s: %w", fp, err)
	}

	// GAPIC build target
	for _, rule := range f.Rules("go_gapic_library") {
		if v := rule.AttrString("grpc_service_config"); v != "" {
			c.grpcServiceConfig = v
		}
		if v := rule.AttrString("importpath"); v != "" {
			c.gapicImportPath = v
		}
		if v := rule.AttrLiteral("metadata"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				c.metadata = b
			} else {
				slog.Warn("failed to parse metadata", "error", err, "input", v)
			}
		}
		if v := rule.AttrString("release_level"); v != "" {
			c.releaseLevel = v
		}
		if v := rule.AttrLiteral("rest_numeric_enums"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				c.restNumericEnums = b
			} else {
				slog.Warn("failed to parse rest_numeric_enums", "error", err, "input", v)
			}
		}
		if v := rule.AttrString("service_yaml"); v != "" {
			c.serviceYAML = v
		}
		if v := rule.AttrString("transport"); v != "" {
			c.transport = v
		}
		if v := rule.AttrLiteral("diregapic"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				c.diregapic = b
			} else {
				slog.Warn("failed to parse diregapic", "error", err, "input", v)
			}
		}
	}

	// We are currently migrating go_proto_library to go_grpc_library.
	// Only one is expect to be present
	for range f.Rules("go_grpc_library") {
		c.hasGoGRPC = true
	}
	for range f.Rules("go_proto_library") {
		if c.hasGoGRPC {
			return nil, fmt.Errorf("misconfiguration in BUILD.bazel file, only one of go_grpc_library and go_proto_library rules should be present: %s", fp)
		}
	}
	return c, nil
}

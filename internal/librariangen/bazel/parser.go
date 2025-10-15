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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	// See https://github.com/googleapis/librarian/issues/1021.
	hasGoGRPC bool

	// Whether this library has a GAPIC rule at all.
	hasGAPIC bool

	// Whether the go_proto_library rule uses @io_bazel_rules_go//proto:go_grpc
	hasLegacyGRPC bool
}

// HasGAPIC indicates whether the GAPIC generator should be run.
// This is typically true. If this is false, none of the other GAPIC-related
// fields should be used.
func (c *Config) HasGAPIC() bool { return c.hasGAPIC }

// DisableGAPIC overrides any previous configuration, disabling GAPIC generation.
func (c *Config) DisableGAPIC() {
	c.hasGAPIC = false
}

// GAPICImportPath is importpath in the go_gapic_library rule.
// The Go package name is typically appended to the end, separated by a `;`.
// E.g., cloud.google.com/go/asset/apiv1;asset
func (c *Config) GAPICImportPath() string { return c.gapicImportPath }

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

// HasLegacyGRPC indicates whether a go_proto_library rule uses
// @io_bazel_rules_go//proto:go_grpc to generate gRPC code. If so,
// the "plugins=grpc" option is passed to the legacy Go plugin.
func (c *Config) HasLegacyGRPC() bool { return c.hasLegacyGRPC }

// Validate ensures that the configuration is valid.
func (c *Config) Validate() error {
	if c.hasGAPIC {
		if c.gapicImportPath == "" {
			return errors.New("librariangen: gapicImportPath is not set")
		}
		if c.serviceYAML == "" {
			return errors.New("librariangen: serviceYAML is not set")
		}
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
		return nil, fmt.Errorf("librariangen: failed to read BUILD.bazel file %s: %w", fp, err)
	}
	content := string(data)

	// First, find the go_gapic_library block.
	re := regexp.MustCompile(`go_gapic_library\((?s:.)*?\)`)
	gapicLibraryBlock := re.FindString(content)
	if gapicLibraryBlock != "" {
		// GAPIC build target
		c.hasGAPIC = true
		c.grpcServiceConfig = findString(gapicLibraryBlock, "grpc_service_config")
		c.gapicImportPath = findString(gapicLibraryBlock, "importpath")
		c.releaseLevel = findString(gapicLibraryBlock, "release_level")
		// If the service config is actually a bazel target instead of a file, just assume there's a file with the same name.
		c.serviceYAML = strings.TrimPrefix(findString(gapicLibraryBlock, "service_yaml"), ":")
		c.transport = findString(gapicLibraryBlock, "transport")
		if c.metadata, err = findBool(gapicLibraryBlock, "metadata"); err != nil {
			return nil, fmt.Errorf("librariangen: failed to parse BUILD.bazel file %s: %w", fp, err)
		}
		if c.restNumericEnums, err = findBool(gapicLibraryBlock, "rest_numeric_enums"); err != nil {
			return nil, fmt.Errorf("librariangen: failed to parse BUILD.bazel file %s: %w", fp, err)
		}
		if c.diregapic, err = findBool(gapicLibraryBlock, "diregapic"); err != nil {
			return nil, fmt.Errorf("librariangen: failed to parse BUILD.bazel file %s: %w", fp, err)
		}
	}

	// We are currently migrating go_proto_library to go_grpc_library.
	// Only one is expect to be present
	if strings.Contains(content, "go_grpc_library") {
		c.hasGoGRPC = true
	}
	goProtoLibraryPattern := regexp.MustCompile(`go_proto_library\((?s:.)*?\)`)
	goProtoLibraryBlock := goProtoLibraryPattern.FindString(content)
	if goProtoLibraryBlock != "" {
		if c.hasGoGRPC {
			return nil, fmt.Errorf("librariangen: misconfiguration in BUILD.bazel file, only one of go_grpc_library and go_proto_library rules should be present: %s", fp)
		}
		c.hasLegacyGRPC = strings.Contains(goProtoLibraryBlock, "@io_bazel_rules_go//proto:go_grpc")
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("librariangen: invalid bazel config in %s: %w", dir, err)
	}
	slog.Debug("librariangen: bazel config loaded", "conf", fmt.Sprintf("%+v", c))
	return c, nil
}

func findString(content, name string) string {
	re := regexp.MustCompile(fmt.Sprintf(`%s\s*=\s*"([^"]+)"`, name))
	if match := re.FindStringSubmatch(content); len(match) > 1 {
		return match[1]
	}
	slog.Debug("librariangen: failed to find string attr in BUILD.bazel", "name", name)
	return ""
}

func findBool(content, name string) (bool, error) {
	re := regexp.MustCompile(fmt.Sprintf(`%s\s*=\s*(\w+)`, name))
	if match := re.FindStringSubmatch(content); len(match) > 1 {
		if b, err := strconv.ParseBool(match[1]); err == nil {
			return b, nil
		}
		return false, fmt.Errorf("librariangen: failed to parse bool attr in BUILD.bazel: %q, got: %q", name, match[1])
	}
	slog.Debug("librariangen: failed to find bool attr in BUILD.bazel", "name", name)
	return false, nil
}

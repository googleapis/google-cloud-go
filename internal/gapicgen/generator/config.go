// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

import (
	"strings"

	"cloud.google.com/go/internal/aliasfix"
)

// MicrogenConfig represents a single microgen target.
type MicrogenConfig struct {
	// InputDirectoryPath is the path to the input (.proto, etc) files, relative
	// to googleapisDir.
	InputDirectoryPath string

	// ImportPath is the path that this library should be imported as.
	ImportPath string

	// Pkg is the name that should be used in the package declaration.
	Pkg string

	// GRPCServiceConfigPath is the path to the grpc service config for this
	// target, relative to googleapisDir/inputDirectoryPath.
	GRPCServiceConfigPath string

	// ApiServiceConfigPath is the path to the gapic service config for this
	// target, relative to googleapisDir/inputDirectoryPath.
	ApiServiceConfigPath string

	// ReleaseLevel is the release level of this target. Values incl ga,
	// beta, alpha.
	ReleaseLevel string

	// stopGeneration is used to stop generating a given client. This might be
	// useful if a client needs to be deprecated, but retained in the repo
	// metadata.
	stopGeneration bool

	// DisableMetadata is used to toggle generation of the gapic_metadata.json
	// file for the client library.
	DisableMetadata bool

	// Transports is a list of Transports to generate a client for. Acceptable
	// values are 'grpc' and 'rest'. Default is ["grpc", "rest"].
	Transports []string

	// stubsDir indicates that the protobuf/gRPC stubs should be generated
	// in the GAPIC module by replacing the go_package option with the value of
	// ImportPath plus the specified suffix separated by a "/", and using the
	// same Pkg value.
	stubsDir string

	// NumericEnumsDisabled indicates, for REST GAPICs, if requests should *not*
	// be generated to send the $alt=json;enum-encoding=int system parameter
	// with every API call. This should only be disabled for services that are
	// *not* up-to-date enough to support such a system parameter.
	NumericEnumsDisabled bool
}

// genprotoImportPath returns the genproto import path for a given config.
func (m *MicrogenConfig) genprotoImportPath() string {
	return "google.golang.org/genproto/googleapis/" + strings.TrimPrefix(m.InputDirectoryPath, "google/")
}

// getStubsDir gets the stubs dir specified in config or returns the
// directory path if the config is either in progress or completed a migration.
func (m *MicrogenConfig) getStubsDir() string {
	if m.stubsDir != "" {
		return m.stubsDir
	}
	if pkg, ok := aliasfix.GenprotoPkgMigration[m.genprotoImportPath()]; ok && pkg.Status != aliasfix.StatusNotMigrated {
		ss := strings.Split(pkg.ImportPath, "/")
		return ss[len(ss)-1]
	}
	return ""
}

// isMigrated is a convenience wrapper for calling the function of the same
// name.
func (m *MicrogenConfig) isMigrated() bool {
	return isMigrated(m.genprotoImportPath())
}

// StopGeneration is used to stop generating a given client. This might be
// useful if a client needs to be deprecated, but retained in the repo
// metadata.
func (m *MicrogenConfig) StopGeneration() bool {
	return m.stopGeneration
}

// isMigrated returns true if the specified genproto import path has been
// migrated.
func isMigrated(importPath string) bool {
	if pkg, ok := aliasfix.GenprotoPkgMigration[importPath]; ok && pkg.Status == aliasfix.StatusMigrated {
		return true
	}
	return false
}

var MicrogenGapicConfigs = []*MicrogenConfig{
	{
		InputDirectoryPath:    "google/devtools/cloudbuild/v1",
		Pkg:                   "cloudbuild",
		ImportPath:            "cloud.google.com/go/cloudbuild/apiv1/v2",
		GRPCServiceConfigPath: "cloudbuild_grpc_service_config.json",
		ApiServiceConfigPath:  "cloudbuild_v1.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/cloud/tasks/v2",
		Pkg:                   "cloudtasks",
		ImportPath:            "cloud.google.com/go/cloudtasks/apiv2",
		GRPCServiceConfigPath: "cloudtasks_grpc_service_config.json",
		ApiServiceConfigPath:  "cloudtasks_v2.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/cloud/tasks/v2beta2",
		Pkg:                   "cloudtasks",
		ImportPath:            "cloud.google.com/go/cloudtasks/apiv2beta2",
		GRPCServiceConfigPath: "cloudtasks_grpc_service_config.json",
		ApiServiceConfigPath:  "cloudtasks_v2beta2.yaml",
		ReleaseLevel:          "beta",
	},
	{
		InputDirectoryPath:    "google/cloud/tasks/v2beta3",
		Pkg:                   "cloudtasks",
		ImportPath:            "cloud.google.com/go/cloudtasks/apiv2beta3",
		GRPCServiceConfigPath: "cloudtasks_grpc_service_config.json",
		ApiServiceConfigPath:  "cloudtasks_v2beta3.yaml",
		ReleaseLevel:          "beta",
	},
	{
		InputDirectoryPath:    "google/devtools/clouddebugger/v2",
		Pkg:                   "debugger",
		ImportPath:            "cloud.google.com/go/debugger/apiv2",
		GRPCServiceConfigPath: "clouddebugger_grpc_service_config.json",
		ApiServiceConfigPath:  "clouddebugger_v2.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/devtools/clouderrorreporting/v1beta1",
		Pkg:                   "errorreporting",
		ImportPath:            "cloud.google.com/go/errorreporting/apiv1beta1",
		GRPCServiceConfigPath: "errorreporting_grpc_service_config.json",
		ApiServiceConfigPath:  "clouderrorreporting_v1beta1.yaml",
		ReleaseLevel:          "beta",
	},
	{
		InputDirectoryPath:    "google/monitoring/v3",
		Pkg:                   "monitoring",
		ImportPath:            "cloud.google.com/go/monitoring/apiv3/v2",
		GRPCServiceConfigPath: "monitoring_grpc_service_config.json",
		ApiServiceConfigPath:  "monitoring.yaml",
		ReleaseLevel:          "ga",
		NumericEnumsDisabled:  true,
		Transports:            []string{"grpc"},
	},
	{
		InputDirectoryPath:    "google/cloud/recaptchaenterprise/v1",
		Pkg:                   "recaptchaenterprise",
		ImportPath:            "cloud.google.com/go/recaptchaenterprise/v2/apiv1",
		GRPCServiceConfigPath: "recaptchaenterprise_grpc_service_config.json",
		ApiServiceConfigPath:  "recaptchaenterprise_v1.yaml",
		ReleaseLevel:          "ga",
		NumericEnumsDisabled:  false,
		Transports:            []string{"grpc"},
	},
	{
		InputDirectoryPath:    "google/cloud/recaptchaenterprise/v1beta1",
		Pkg:                   "recaptchaenterprise",
		ImportPath:            "cloud.google.com/go/recaptchaenterprise/v2/apiv1beta1",
		GRPCServiceConfigPath: "recaptchaenterprise_grpc_service_config.json",
		ApiServiceConfigPath:  "recaptchaenterprise_v1beta1.yaml",
		ReleaseLevel:          "beta",
	},
	{
		InputDirectoryPath:    "google/cloud/resourcemanager/v2",
		Pkg:                   "resourcemanager",
		ImportPath:            "cloud.google.com/go/resourcemanager/apiv2",
		GRPCServiceConfigPath: "",
		ApiServiceConfigPath:  "cloudresourcemanager_v2.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/storage/v2",
		Pkg:                   "storage",
		ImportPath:            "cloud.google.com/go/storage/internal/apiv2",
		GRPCServiceConfigPath: "",
		ApiServiceConfigPath:  "storage_v2.yaml",
		ReleaseLevel:          "alpha",
		NumericEnumsDisabled:  true,
		Transports:            []string{"grpc"},
	},
	{
		InputDirectoryPath:    "google/devtools/cloudtrace/v1",
		Pkg:                   "trace",
		ImportPath:            "cloud.google.com/go/trace/apiv1",
		GRPCServiceConfigPath: "cloudtrace_grpc_service_config.json",
		ApiServiceConfigPath:  "cloudtrace_v1.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/devtools/cloudtrace/v2",
		Pkg:                   "trace",
		ImportPath:            "cloud.google.com/go/trace/apiv2",
		GRPCServiceConfigPath: "cloudtrace_grpc_service_config.json",
		ApiServiceConfigPath:  "cloudtrace_v2.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/cloud/translate/v3",
		Pkg:                   "translate",
		ImportPath:            "cloud.google.com/go/translate/apiv3",
		GRPCServiceConfigPath: "translate_grpc_service_config.json",
		ApiServiceConfigPath:  "translate_v3.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/cloud/vision/v1",
		Pkg:                   "vision",
		ImportPath:            "cloud.google.com/go/vision/v2/apiv1",
		GRPCServiceConfigPath: "vision_grpc_service_config.json",
		ApiServiceConfigPath:  "vision_v1.yaml",
		ReleaseLevel:          "ga",
	},
	{
		InputDirectoryPath:    "google/cloud/vision/v1p1beta1",
		Pkg:                   "vision",
		ImportPath:            "cloud.google.com/go/vision/v2/apiv1p1beta1",
		GRPCServiceConfigPath: "vision_grpc_service_config.json",
		ApiServiceConfigPath:  "vision_v1p1beta1.yaml",
		ReleaseLevel:          "beta",
	},
}

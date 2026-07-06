/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package database

import (
	"context"

	"cloud.google.com/go/spanner/omni"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const omniInstanceType = "OMNI"

// OmniClientConfig defines the Spanner Omni connection settings used by
// NewDatabaseAdminClientWithConfig.
type OmniClientConfig interface {
	GetInstanceType() string
	GetUsePlainText() bool
	GetCaCertificateFile() string
	GetClientCertificateFile() string
	GetClientKeyFile() string
	GetUsername() string
	GetPassword() string
}

// NewDatabaseAdminClientWithConfig creates a new database admin client with
// connection options derived from config. The config is primarily used for
// Spanner Omni connections.
func NewDatabaseAdminClientWithConfig(ctx context.Context, config OmniClientConfig, opts ...option.ClientOption) (*DatabaseAdminClient, error) {
	if config == nil {
		return nil, status.Error(codes.InvalidArgument, "config cannot be nil")
	}

	if config.GetInstanceType() != omniInstanceType && hasOmniConnectionOptions(config) {
		return nil, status.Errorf(codes.InvalidArgument, "UsePlainText, CaCertificateFile, ClientCertificateFile, and ClientKeyFile can only be set when Type is OMNI")
	}

	if config.GetInstanceType() == omniInstanceType {
		omniOpts, err := omni.ConnectionOptions(config.GetUsePlainText(), config.GetCaCertificateFile(), config.GetClientCertificateFile(), config.GetClientKeyFile())
		if err != nil {
			return nil, err
		}
		opts = append(opts, omniOpts...)

		if config.GetUsername() != "" && config.GetPassword() != "" {
			tsOpts := append([]option.ClientOption(nil), opts...)
			opts = append(opts, option.WithTokenSource(omni.NewTokenSource(config.GetUsername(), config.GetPassword(), tsOpts)))
		} else {
			opts = append(opts, option.WithoutAuthentication())
		}
	}

	return NewDatabaseAdminClient(ctx, opts...)
}

func hasOmniConnectionOptions(config OmniClientConfig) bool {
	return config.GetUsePlainText() ||
		config.GetCaCertificateFile() != "" ||
		config.GetClientCertificateFile() != "" ||
		config.GetClientKeyFile() != ""
}

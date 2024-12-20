// Copyright 2023 Google LLC
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

package grpctransport

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal/compute"
	"google.golang.org/grpc"
	grpcgoogle "google.golang.org/grpc/credentials/google"
)

func isDirectPathEnabled(endpoint string, opts *Options) bool {
	if opts.InternalOptions != nil && !opts.InternalOptions.EnableDirectPath {
		return false
	}
	if !checkDirectPathEndPoint(endpoint) {
		return false
	}
	if b, _ := strconv.ParseBool(os.Getenv(disableDirectPathEnvVar)); b {
		return false
	}
	return true
}

func checkDirectPathEndPoint(endpoint string) bool {
	// Only [dns:///]host[:port] is supported, not other schemes (e.g., "tcp://" or "unix://").
	// Also don't try direct path if the user has chosen an alternate name resolver
	// (i.e., via ":///" prefix).
	if strings.Contains(endpoint, "://") && !strings.HasPrefix(endpoint, "dns:///") {
		return false
	}

	if endpoint == "" {
		return false
	}

	return true
}

func isTokenProviderDirectPathCompatible(tp auth.TokenProvider, o *Options) bool {
	if tp == nil {
		log.Println("isTokenProviderDirectPathCompatible: tp is nil")
		return false
	}
	tok, err := tp.Token(context.Background())
	if err != nil {
		log.Printf("isTokenProviderDirectPathCompatible err: %v", err)
		return false
	}
	if tok == nil {
		log.Println("isTokenProviderDirectPathCompatible: tok is nil")
		return false
	}
	if val := tok.MetadataString("auth.google.tokenSource"); val != "compute-metadata" {
		log.Printf("Metadata String: %v", val)
		return false
	}
	log.Printf("MADE HERE WOHO!!")
	if o.InternalOptions != nil && o.InternalOptions.EnableNonDefaultSAForDirectPath {
		return true
	}
	if tok.MetadataString("auth.google.serviceAccount") != "default" {
		return false
	}
	return true
}

func isDirectPathXdsUsed(o *Options) bool {
	// Method 1: Enable DirectPath xDS by env;
	if b, _ := strconv.ParseBool(os.Getenv(enableDirectPathXdsEnvVar)); b {
		return true
	}
	// Method 2: Enable DirectPath xDS by option;
	if o.InternalOptions != nil && o.InternalOptions.EnableDirectPathXds {
		return true
	}
	return false
}

// configureDirectPath returns some dial options and an endpoint to use if the
// configuration allows the use of direct path. If it does not the provided
// grpcOpts and endpoint are returned.
func configureDirectPath(grpcOpts []grpc.DialOption, opts *Options, endpoint string, creds *auth.Credentials) ([]grpc.DialOption, string) {
	log.Println("Checking if DirectPath can be enabled")
	if isDirectPathEnabled(endpoint, opts) && compute.OnComputeEngine() && isTokenProviderDirectPathCompatible(creds, opts) {
		log.Println("DirectPath can be enabled")
		// Overwrite all of the previously specific DialOptions, DirectPath uses its own set of credentials and certificates.
		grpcOpts = []grpc.DialOption{
			grpc.WithCredentialsBundle(grpcgoogle.NewDefaultCredentialsWithOptions(grpcgoogle.DefaultCredentialsOptions{PerRPCCreds: &grpcCredentialsProvider{creds: creds}}))}
		if timeoutDialerOption != nil {
			grpcOpts = append(grpcOpts, timeoutDialerOption)
		}
		// Check if google-c2p resolver is enabled for DirectPath
		if isDirectPathXdsUsed(opts) {
			// google-c2p resolver target must not have a port number
			if addr, _, err := net.SplitHostPort(endpoint); err == nil {
				endpoint = "google-c2p:///" + addr
			} else {
				endpoint = "google-c2p:///" + endpoint
			}
		} else {
			if !strings.HasPrefix(endpoint, "dns:///") {
				endpoint = "dns:///" + endpoint
			}
			grpcOpts = append(grpcOpts,
				// For now all DirectPath go clients will be using the following lb config, but in future
				// when different services need different configs, then we should change this to a
				// per-service config.
				grpc.WithDisableServiceConfig(),
				grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"grpclb":{"childPolicy":[{"pick_first":{}}]}}]}`))
		}
		// TODO: add support for system parameters (quota project, request reason) via chained interceptor.
	} else {
		log.Println("DirectPath could not be enabled")
	}
	return grpcOpts, endpoint
}

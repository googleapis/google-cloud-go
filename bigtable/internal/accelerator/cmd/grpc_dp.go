//go:build !disable_grpc_modules

// Copyright 2026 Google LLC
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

package main

// The accelerator daemon dials the data plane with DirectAccess enabled (see
// internal/option.DirectAccessOptions) but, unlike the classic client, it does
// not import package bigtable -- so it never pulls in that package's grpc_dp.go,
// which is the only place these two gRPC modules are registered. Without them
// the daemon requests DirectPath, fails to resolve the "google-c2p" target
// ("unknown port") or NACKs the xDS route config ("RLS LB policy not
// registered"), and silently falls back to CloudPath (CFE). Register the same
// pair here, behind the same build tag, so DirectPath is usable in environments
// that support it.
import (
	// Install the google-c2p resolver, which is required for DirectPath.
	_ "google.golang.org/grpc/xds/googledirectpath"
	// Install the RLS load-balancing policy, required by Bigtable's DirectPath
	// xDS route config (google.golang.org/grpc RLS).
	_ "google.golang.org/grpc/balancer/rls"
)

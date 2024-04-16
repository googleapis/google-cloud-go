// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package containeranalysis is an auto-generated package for the
// Container Analysis API.
//
//	NOTE: This package is in beta. It is not stable, and may be subject to changes.
//
// An implementation of the Grafeas API, which stores, and enables querying
// and retrieval of critical metadata about all of your software artifacts.
//
// # Use of Context
//
// The ctx passed to NewClient is used for authentication requests and
// for creating the underlying connection, but is not used for subsequent calls.
// Individual methods on the client use the ctx given to them.
//
// To close the open connection, use the Close() method.
//
// For information about setting deadlines, reusing contexts, and more
// please visit godoc.org/cloud.google.com/go.
package containeranalysis // import "cloud.google.com/go/containeranalysis/apiv1"

import (
	"context"

	"google.golang.org/grpc/metadata"
)

func insertMetadata(ctx context.Context, mds ...metadata.MD) context.Context {
	out, _ := metadata.FromOutgoingContext(ctx)
	out = out.Copy()
	for _, md := range mds {
		for k, v := range md {
			out[k] = append(out[k], v...)
		}
	}
	return metadata.NewOutgoingContext(ctx, out)
}

// DefaultAuthScopes reports the default set of authentication scopes to use with this package.
func DefaultAuthScopes() []string {
	return []string{
		"https://www.googleapis.com/auth/cloud-platform",
	}
}

var versionClient = "20220222"

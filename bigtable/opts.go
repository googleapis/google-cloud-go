/*
Copyright 2015 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

// TODO(dsymonds): Much of this file may migrate to google.golang.org/cloud.

import (
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// A ClientOption configures a Client or AdminClient.
type ClientOption interface {
	clientOption()
}

func dialWithOptions(ctx context.Context, defAddr, scope string, opts ...ClientOption) (*grpc.ClientConn, error) {
	addr := defAddr
	insecure := false
	var creds credentials.Credentials
	gotCreds := false
	for _, opt := range opts {
		switch opt := opt.(type) {
		case withCreds:
			creds = opt.creds
			gotCreds = true
		case withInsecureAddr:
			addr = string(opt)
			insecure = true
		}
	}
	if !gotCreds {
		ts, err := google.DefaultTokenSource(ctx, scope)
		if err != nil {
			return nil, err
		}
		creds = credentials.TokenSource{ts}
	}

	var dopts []grpc.DialOption
	if creds != nil {
		dopts = append(dopts, grpc.WithPerRPCCredentials(creds))
	}
	if !insecure {
		dopts = append(dopts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}
	return grpc.Dial(addr, dopts...)
}

// WithCredentials returns a ClientOption that specifies some non-default gRPC credentials.
// The supplied credentials should work in Scope, ReadonlyScope or AdminScope.
//
// If this option is not used, the Application Default Credentials are used.
func WithCredentials(creds credentials.Credentials) ClientOption { return withCreds{creds} }

type withCreds struct{ creds credentials.Credentials }

func (withCreds) clientOption() {}

// WithInsecureAddr returns a ClientOption that results in dialing an alternate address, without TLS.
func WithInsecureAddr(addr string) ClientOption { return withInsecureAddr(addr) }

type withInsecureAddr string

func (withInsecureAddr) clientOption() {}

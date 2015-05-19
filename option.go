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

package cloud

import (
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type dialOpt struct {
	endpoint string
	scopes   []string

	tokenSource oauth2.TokenSource

	httpClient *http.Client
	grpcClient *grpc.ClientConn
}

// ClientOption is used when construct clients for each cloud service.
type ClientOption interface {
	resolve(*dialOpt)
}

// WithTokenSource returns a ClientOption that specifies an OAuth2 token
// source to be used as the basis for authentication.
func WithTokenSource(s oauth2.TokenSource) ClientOption {
	return withTokenSource{s}
}

type withTokenSource struct{ ts oauth2.TokenSource }

func (w withTokenSource) resolve(o *dialOpt) {
	o.tokenSource = w.ts
}

// WithEndpoint returns a ClientOption that overrides the default endpoint
// to be used for a service.
func WithEndpoint(url string) ClientOption {
	return withEndpoint(url)
}

type withEndpoint string

func (w withEndpoint) resolve(o *dialOpt) {
	o.endpoint = string(w)
}

// WithScopes returns a ClientOption that overrides the default OAuth2 scopes
// to be used for a service.
func WithScopes(scope ...string) ClientOption {
	return withScopes(scope)
}

type withScopes []string

func (w withScopes) resolve(o *dialOpt) {
	o.scopes = []string(w)
}

// WithBaseHTTP returns a ClientOption that specifies the HTTP client to
// use as the basis of communications. This option may only be used with
// services that support HTTP as their communication transport.
func WithBaseHTTP(client *http.Client) ClientOption {
	return withBaseHTTP{client}
}

type withBaseHTTP struct{ client *http.Client }

func (w withBaseHTTP) resolve(o *dialOpt) {
	o.httpClient = w.client
}

// WithBaseGRPC returns a ClientOption that specifies the GRPC client
// connection to use as the basis of communications. This option many only be
// used with services that support HRPC as their communication transport.
func WithBaseGRPC(client *grpc.ClientConn) ClientOption {
	return withBaseGRPC{client}
}

type withBaseGRPC struct{ client *grpc.ClientConn }

func (w withBaseGRPC) resolve(o *dialOpt) {
	o.grpcClient = w.client
}

// DialHTTP returns an HTTP client for use communicating with a Google cloud
// service, configured with the given ClientOptions. Most developers should
// call the relevant NewClient method for the target service rather than
// invoking DialHTTP directly.
func DialHTTP(ctx context.Context, opt ...ClientOption) (*http.Client, error) {
	var o dialOpt
	for _, opt := range opt {
		opt.resolve(&o)
	}
	if o.grpcClient != nil {
		return nil, errors.New("unsupported GRPC base transport specified")
	}
	// TODO(djd): Wrap all http.Client's with appropriate internal version to add
	// UserAgent header and prepend correct endpoint.
	if o.httpClient != nil {
		return o.httpClient, nil
	}
	if o.tokenSource == nil {
		var err error
		o.tokenSource, err = google.DefaultTokenSource(ctx, o.scopes...)
		if err != nil {
			return nil, fmt.Errorf("google.DefaultTokenSource: %v", err)
		}
	}
	return oauth2.NewClient(ctx, o.tokenSource), nil
}

// DialGRPC returns a GRPC connection for use communicating with a Google cloud
// service, configured with the given ClientOptions. Most developers should
// call the relevant NewClient method for the target service rather than
// invoking DialGRPC directly.
func DialGRPC(ctx context.Context, opt ...ClientOption) (*grpc.ClientConn, error) {
	var o dialOpt
	for _, opt := range opt {
		opt.resolve(&o)
	}
	if o.httpClient != nil {
		return nil, errors.New("unsupported HTTP base transport specified")
	}
	if o.grpcClient != nil {
		return o.grpcClient, nil
	}
	if o.tokenSource == nil {
		var err error
		o.tokenSource, err = google.DefaultTokenSource(ctx, o.scopes...)
		if err != nil {
			return nil, fmt.Errorf("google.DefaultTokenSource: %v", err)
		}
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithPerRPCCredentials(credentials.TokenSource{o.tokenSource}),
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
	}
	return grpc.Dial(o.endpoint, grpcOpts...)
}

/*
Copyright 2026 Google LLC

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

package spanner

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type authorityInterceptor struct {
	unaryHeaders metadata.MD
}

func (ai *authorityInterceptor) interceptUnary(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing metadata in unary request")
	}
	ai.unaryHeaders = md
	return handler(ctx, req)
}

func TestCreateEndpointClientPreservesDefaultAuthority(t *testing.T) {
	interceptor := &authorityInterceptor{}
	server, clientOpts, teardown := testutil.NewMockedSpannerInMemTestServer(
		t, grpc.UnaryInterceptor(interceptor.interceptUnary),
	)
	defer teardown()

	database := "projects/p/instances/i/databases/d"
	sc := newSessionClient(
		nil,
		database,
		"",
		nil,
		"",
		false,
		metadata.Pairs(resourcePrefixHeader, database),
		0,
		nil,
		nil,
	)
	sc.baseClientOpts = clientOpts
	sc.endpointAuthority = "spanner.spanner-ns:15000"
	sc.metricsTracerFactory = &builtinMetricsTracerFactory{}

	client, err := sc.createEndpointClient(context.Background(), server.ServerAddress)
	if err != nil {
		t.Fatalf("createEndpointClient() failed: %v", err)
	}
	defer client.Close()

	_, err = client.CreateSession(context.Background(), &spannerpb.CreateSessionRequest{
		Database: database,
		Session:  &spannerpb.Session{},
	})
	if err != nil {
		t.Fatalf("CreateSession() failed: %v", err)
	}

	if got := interceptor.unaryHeaders.Get(":authority"); len(got) != 1 || got[0] != sc.endpointAuthority {
		t.Fatalf("authority mismatch\ngot: %v\nwant: [%s]", got, sc.endpointAuthority)
	}
}

func TestNormalizeAuthorityTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		want   string
	}{
		{name: "plain", target: "spanner.googleapis.com:443", want: "spanner.googleapis.com:443"},
		{name: "dns", target: "dns:///spanner.googleapis.com:443", want: "spanner.googleapis.com:443"},
		{name: "passthrough", target: "passthrough:///10.0.0.1:15000", want: "10.0.0.1:15000"},
		{name: "google-c2p", target: "google-c2p:///spanner.googleapis.com", want: "spanner.googleapis.com"},
		{name: "https", target: "https://spanner.googleapis.com:443", want: "spanner.googleapis.com:443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeAuthorityTarget(tc.target); got != tc.want {
				t.Fatalf("normalizeAuthorityTarget(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

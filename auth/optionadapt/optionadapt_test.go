// Copyright 2025 Google LLC
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

package optionadapt

import (
	"context"
	"testing"

	"cloud.google.com/go/auth/grpctransport"
	"google.golang.org/api/option"
)

const testUniverseDomain = "example.com"

func TestDialPool(t *testing.T) {
	oldDialContextNewAuth := dialContextNewAuth
	var wantNumGRPCDialOpts int
	var universeDomain string
	// Replace package var in order to assert DialContext args.
	dialContextNewAuth = func(ctx context.Context, secure bool, opts *grpctransport.Options) (grpctransport.GRPCClientConnPool, error) {
		if len(opts.GRPCDialOpts) != wantNumGRPCDialOpts {
			t.Fatalf("got: %d, want: %d", len(opts.GRPCDialOpts), wantNumGRPCDialOpts)
		}
		if opts.UniverseDomain != universeDomain {
			t.Fatalf("got: %q, want: %q", opts.UniverseDomain, universeDomain)
		}
		return nil, nil
	}
	defer func() {
		dialContextNewAuth = oldDialContextNewAuth
	}()

	for _, tc := range []struct {
		name                string
		opts                []option.ClientOption
		wantNumGRPCDialOpts int
		wantUniverseDomain  string
	}{
		{
			name: "no options",
			opts: []option.ClientOption{},
		},
		{
			name: "with user agent",
			opts: []option.ClientOption{
				option.WithUserAgent("test"),
			},
			wantNumGRPCDialOpts: 1,
		},
		{
			name: "with universe domain",
			opts: []option.ClientOption{
				option.WithUniverseDomain(testUniverseDomain),
			},
			wantUniverseDomain: testUniverseDomain,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantNumGRPCDialOpts = tc.wantNumGRPCDialOpts
			universeDomain = tc.wantUniverseDomain
			_, err := DialPool(context.Background(), false, 1, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

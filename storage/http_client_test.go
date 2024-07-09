// Copyright 2024 Google LLC
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

package storage

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2/callctx"
)

func TestSetHeadersFromContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	xGoogKey := "X-Goog-Api-Client"

	for _, test := range []struct {
		desc            string
		originalHeaders http.Header
		headersOnCtx    []string // keyval pairs
		wantHeaders     http.Header
	}{
		{
			desc: "all empty values",
		},
		{
			desc: "regular headers",
			originalHeaders: http.Header{
				"Headerkey-A": {"value1", "value2"},
				"Headerkey-B": {"v1", "v2"},
			},
			headersOnCtx: []string{"key-c", "val1", "headerkey-a", "value3"},
			wantHeaders: http.Header{
				"Headerkey-A": {"value3"},
				"Headerkey-B": {"v1", "v2"},
				"Key-C":       {"val1"},
			},
		},
		{
			desc: "x-goog-api-client merging",
			originalHeaders: http.Header{
				"Headerkey-A": {"value1", "value2"},
			},
			headersOnCtx: []string{"key-c", "val1", xGoogKey, "k1/v1 k2/v2", xGoogKey, "k3/v3"},
			wantHeaders: http.Header{
				"Headerkey-A": {"value1", "value2"},
				"Key-C":       {"val1"},
				xGoogKey:      {"k1/v1 k2/v2 k3/v3"},
			},
		},
		{
			desc: "x-goog-api-client merging with values already set",
			originalHeaders: http.Header{
				"Headerkey-A": {"value1", "value2"},
				xGoogKey:      {"k4/v4 k5/v5"},
			},
			headersOnCtx: []string{"key-c", "val1", xGoogKey, "k1/v1 k2/v2", xGoogKey, "k3/v3"},
			wantHeaders: http.Header{
				"Headerkey-A": {"value1", "value2"},
				"Key-C":       {"val1"},
				xGoogKey:      {"k1/v1 k2/v2 k3/v3 k4/v4 k5/v5"},
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := callctx.SetHeaders(ctx, test.headersOnCtx...)
			got := test.originalHeaders.Clone()
			setHeadersFromCtx(ctx, got)

			if len(got) != len(test.wantHeaders) {
				t.Errorf("Headers not set correctly: got: %+v, want: %+v\n", got, test.wantHeaders)
			}

			for k, wantVals := range test.wantHeaders {
				if diff := cmp.Diff(got[k], wantVals, cmpopts.SortSlices(func(a, b string) bool { return len(a) < len(b) })); diff != "" {
					t.Errorf("Header %q not set correctly: got(-),want(+):\n%s", k, diff)
				}
			}
		})
	}
}

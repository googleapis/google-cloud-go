// Copyright 2021 Google LLC
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

package managedwriter

import (
	"context"
	"testing"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestTableParentFromStreamName(t *testing.T) {
	testCases := []struct {
		in   string
		want string
	}{
		{
			"bad",
			"bad",
		},
		{
			"projects/foo/datasets/bar/tables/baz",
			"projects/foo/datasets/bar/tables/baz",
		},
		{
			"projects/foo/datasets/bar/tables/baz/zip/zam/zoomie",
			"projects/foo/datasets/bar/tables/baz",
		},
		{
			"projects/foo/datasets/bar/tables/baz/_default",
			"projects/foo/datasets/bar/tables/baz",
		},
	}

	for _, tc := range testCases {
		got := TableParentFromStreamName(tc.in)
		if got != tc.want {
			t.Errorf("mismatch on %s: got %s want %s", tc.in, got, tc.want)
		}
	}
}

func TestCreatePool_Location(t *testing.T) {
	t.Skip("skipping until new write_location is allowed")
	c := &Client{
		cfg:       &writerClientConfig{},
		ctx:       context.Background(),
		projectID: "myproj",
	}
	pool, err := c.createPool("foo", nil)
	if err != nil {
		t.Fatalf("createPool: %v", err)
	}
	meta, ok := metadata.FromOutgoingContext(pool.ctx)
	if !ok {
		t.Fatalf("no metadata in outgoing context")
	}
	vals, ok := meta["x-goog-request-params"]
	if !ok {
		t.Fatalf("metadata key not present")
	}
	found := false
	for _, v := range vals {
		if v == "write_location=projects/myproj/locations/foo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected location header not found")
	}
}

// TestCreatePool tests the result of calling createPool with different combinations
// of global configuration and per-writer configuration.
func TestCreatePool(t *testing.T) {
	testCases := []struct {
		desc                string
		cfg                 *writerClientConfig
		settings            *streamSettings
		wantMaxBytes        int
		wantMaxRequests     int
		wantCallOptions     int
		wantPoolCallOptions int
	}{
		{
			desc: "cfg, no settings",
			cfg: &writerClientConfig{
				defaultInflightRequests: 12,
				defaultInflightBytes:    2048,
			},
			wantMaxBytes:    2048,
			wantMaxRequests: 12,
		},
		{
			desc: "empty cfg, w/settings",
			cfg:  &writerClientConfig{},
			settings: &streamSettings{
				MaxInflightRequests: 99,
				MaxInflightBytes:    1024,
				appendCallOptions:   []gax.CallOption{gax.WithPath("foo")},
			},
			wantMaxBytes:    1024,
			wantMaxRequests: 99,
			wantCallOptions: 1,
		},
		{
			desc: "both cfg and settings",
			cfg: &writerClientConfig{
				defaultInflightRequests:      123,
				defaultInflightBytes:         456,
				defaultAppendRowsCallOptions: []gax.CallOption{gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(999))},
			},
			settings: &streamSettings{
				MaxInflightRequests: 99,
				MaxInflightBytes:    1024,
			},
			wantMaxBytes:        1024,
			wantMaxRequests:     99,
			wantPoolCallOptions: 1,
		},
		{
			desc: "merge defaults and settings",
			cfg: &writerClientConfig{
				defaultInflightRequests:      123,
				defaultInflightBytes:         456,
				defaultAppendRowsCallOptions: []gax.CallOption{gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(999))},
			},
			settings: &streamSettings{
				MaxInflightBytes:  1024,
				appendCallOptions: []gax.CallOption{gax.WithPath("foo")},
			},
			wantMaxBytes:        1024,
			wantMaxRequests:     123,
			wantCallOptions:     1,
			wantPoolCallOptions: 1,
		},
	}

	for _, tc := range testCases {
		c := &Client{
			cfg: tc.cfg,
			ctx: context.Background(),
		}
		pool, err := c.createPool("", nil)
		if err != nil {
			t.Errorf("case %q: createPool errored unexpectedly: %v", tc.desc, err)
			continue
		}
		writer := &ManagedStream{
			id:             "foo",
			streamSettings: tc.settings,
		}
		if err = pool.addWriter(writer); err != nil {
			t.Errorf("case %q: addWriter: %v", tc.desc, err)
		}
		pw := newPendingWrite(context.Background(), writer, nil, nil, "", "")
		gotConn, err := pool.selectConn(pw)
		if err != nil {
			t.Errorf("case %q: selectConn: %v", tc.desc, err)
		}

		// too many go-cmp overrides needed to quickly diff here, look at the interesting fields explicitly.
		if gotVal := gotConn.fc.maxInsertBytes; gotVal != tc.wantMaxBytes {
			t.Errorf("case %q: flowController maxInsertBytes mismatch, got %d want %d", tc.desc, gotVal, tc.wantMaxBytes)
		}
		if gotVal := gotConn.fc.maxInsertCount; gotVal != tc.wantMaxRequests {
			t.Errorf("case %q: flowController maxInsertCount mismatch, got %d want %d", tc.desc, gotVal, tc.wantMaxRequests)
		}
		if gotVal := len(gotConn.callOptions); gotVal != tc.wantCallOptions {
			t.Errorf("case %q: calloption count mismatch, got %d want %d", tc.desc, gotVal, tc.wantCallOptions)
		}
		if gotVal := len(pool.callOptions); gotVal != tc.wantPoolCallOptions {
			t.Errorf("case %q: POOL calloption count mismatch, got %d want %d", tc.desc, gotVal, tc.wantPoolCallOptions)
		}
	}
}

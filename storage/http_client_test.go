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
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/api/option"
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

// TestXMLReaderRetryInvocationHeaders verifies that retried XML read requests
// carry only the invocation headers for the current attempt. The underlying
// *http.Request is reused across attempts, so headers from earlier attempts
// must not leak into later ones.
func TestXMLReaderRetryInvocationHeaders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const failures = 2
	var (
		mu         sync.Mutex
		gotHeaders []string
	)
	hc, closeServer := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHeaders = append(gotHeaders, r.Header.Get(xGoogHeaderKey))
		n := len(gotHeaders)
		mu.Unlock()
		if n <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(readData)))
		w.Write([]byte(readData))
	})
	defer closeServer()

	client, err := NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	r, err := client.Bucket("b").Object("o").Retryer(
		WithBackoff(gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}),
		WithPolicy(RetryAlways),
	).NewReader(ctx)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer r.Close()
	if _, err := io.ReadAll(r); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gotHeaders) != failures+1 {
		t.Fatalf("got %d requests, want %d", len(gotHeaders), failures+1)
	}
	for i, h := range gotHeaders {
		attempt := i + 1
		if got := strings.Count(h, "gccl-attempt-count/"); got != 1 {
			t.Errorf("attempt %d: %q has %d gccl-attempt-count tokens, want 1", attempt, h, got)
		}
		if want := fmt.Sprintf("gccl-attempt-count/%d", attempt); !strings.Contains(h, want) {
			t.Errorf("attempt %d: %q does not contain %q", attempt, h, want)
		}
		if got := strings.Count(h, "gccl-invocation-id/"); got != 1 {
			t.Errorf("attempt %d: %q has %d gccl-invocation-id tokens, want 1", attempt, h, got)
		}
	}
}

func TestAppendableWriteUnsupported(t *testing.T) {
	ctx := context.Background()
	c, err := newHTTPStorageClient(ctx)
	if err != nil {
		t.Fatalf("failed to create HTTP client: %v", err)
	}

	bucket := "a-bucket"
	object := "an-object"
	attrs := ObjectAttrs{
		Bucket:     bucket,
		Name:       object,
		Generation: defaultGen,
	}

	params := &openWriterParams{
		attrs:    &attrs,
		bucket:   bucket,
		append:   true,
		ctx:      ctx,
		donec:    make(chan struct{}),
		setError: func(_ error) {},        // no-op
		progress: func(_ int64) {},        // no-op
		setObj:   func(_ *ObjectAttrs) {}, // no-op
	}
	_, err = c.OpenWriter(params)
	if err == nil {
		t.Errorf("OpenWriter: got ok; want error")
	}
}

func TestValidateChecksumFromServer(t *testing.T) {
	correctChecksum := 1
	tests := []struct {
		name          string
		wrongChecksum bool
		wantErr       error
	}{
		{
			name:          "correct checksum",
			wrongChecksum: false,
		},
		{
			name:          "wrong checksum",
			wrongChecksum: true,
			wantErr:       fmt.Errorf("storage: object checksum mismatch: computed %q, server %q; the bucket may contain corrupted object", encodeUint32(2), encodeUint32(1)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverChecksumChan := make(chan uint32, 1)
			checksum := correctChecksum
			if test.wrongChecksum {
				checksum = 2
			}
			hiw := &httpInternalWriter{
				serverChecksumChan: serverChecksumChan,
				fullObjectChecksum: uint32(checksum),
			}
			go func() {
				serverChecksumChan <- uint32(correctChecksum)
			}()
			err := hiw.validateChecksumFromServer()
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				return
			}
			if err.Error() != test.wantErr.Error() {
				t.Errorf("Want err: %v, got err: %v", err, test.wantErr)
			}
		})
	}
}

func TestTrackingTransport(t *testing.T) {
	mock := &mockTransport{}
	mock.addResult(&http.Response{Status: "200 OK"}, nil)
	tt := &trackingTransport{
		base:     mock,
		features: uint32(1 << featurePCU),
	}

	ctx := context.Background()
	ctx = addFeatureAttributes(ctx, featureMultistreamInMRD)
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)

	_, err := tt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	gotHeader := mock.gotReq.Header.Get(featureTrackerHeaderName)
	wantFeatures := uint32(1<<featurePCU) | uint32(1<<featureMultistreamInMRD)
	wantHeader := encodeUint32(wantFeatures)

	if gotHeader != wantHeader {
		t.Errorf("Header %s = %q; want %q", featureTrackerHeaderName, gotHeader, wantHeader)
	}

	// Verify original request was not modified.
	if req.Header.Get(featureTrackerHeaderName) != "" {
		t.Errorf("Original request header was modified")
	}
}

type mockCloseIdler struct {
	mockTransport
	closedIdle bool
}

func (m *mockCloseIdler) CloseIdleConnections() {
	m.closedIdle = true
}

func TestTrackingTransport_CloseIdleConnections(t *testing.T) {
	mock := &mockCloseIdler{}
	tt := &trackingTransport{
		base: mock,
	}

	tt.CloseIdleConnections()
	if !mock.closedIdle {
		t.Errorf("CloseIdleConnections was not called on base transport")
	}
}

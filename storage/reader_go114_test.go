// Copyright 2020 Google LLC
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

// +build go1.14

// This test is only for Go1.14 and above because we need to use
// net/http/httptest.Server.EnableHTTP2, which was introduced in Go1.14.

package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"google.golang.org/api/option"
)

// alwaysToTargetURLRoundTripper ensures that every single request
// is routed to a target destination. Some requests within the storage
// client by-pass using the provided HTTP client, hence this enforcemenet.
type alwaysToTargetURLRoundTripper struct {
	destURL *url.URL
	hc      *http.Client
}

func (adrt *alwaysToTargetURLRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Host = adrt.destURL.Host
	// Cloud Storage has full control over the response headers for their
	// HTTP server but unfortunately we don't, so we have to prune
	// the Range header to mimick GCS ignoring Range header:
	// https://cloud.google.com/storage/docs/transcoding#range
	delete(req.Header, "Range")
	return adrt.hc.Do(req)
}

func TestContentEncodingGzipWithReader(t *testing.T) {
	original := bytes.Repeat([]byte("a"), 4<<10)
	mockGCS := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b/bucket/o/object":
			fmt.Fprintf(w, `{
                            "bucket": "bucket", "name": "name", "contentEncoding": "gzip",
                            "contentLength": 43,
                            "contentType": "text/plain","timeCreated": "2020-04-10T16:08:58-07:00",
                            "updated": "2020-04-14T16:08:58-07:00"
                        }`)
			return

		default:
			// Serve back the file.
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Etag", `"c50e3e41c9bc9df34e84c94ce073f928"`)
			w.Header().Set("X-Goog-Generation", "1587012235914578")
			w.Header().Set("X-Goog-MetaGeneration", "2")
			w.Header().Set("X-Goog-Stored-Content-Encoding", "gzip")
			w.Header().Set("vary", "Accept-Encoding")
			w.Header().Set("x-goog-stored-content-length", "43")
			w.Header().Set("x-goog-hash", "crc32c=pYIWwQ==")
			w.Header().Set("x-goog-hash", "md5=xQ4+Qcm8nfNOhMlM4HP5KA==")
			w.Header().Set("x-goog-storage-class", "STANDARD")
			gz := gzip.NewWriter(w)
			gz.Write(original)
			gz.Close()
		}
	}))
	mockGCS.EnableHTTP2 = true
	mockGCS.StartTLS()
	defer mockGCS.Close()

	ctx := context.Background()
	hc := mockGCS.Client()
	ux, _ := url.Parse(mockGCS.URL)
	hc.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	wrt := &alwaysToTargetURLRoundTripper{
		destURL: ux,
		hc:      hc,
	}

	whc := &http.Client{Transport: wrt}
	client, err := NewClient(ctx, option.WithEndpoint(mockGCS.URL), option.WithoutAuthentication(), option.WithHTTPClient(whc))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// 2. Different flavours of the read should all return the body.
	readerCreators := []struct {
		name   string
		create func(ctx context.Context, obj *ObjectHandle) (*Reader, error)
	}{
		{
			"NewReader", func(cxt context.Context, obj *ObjectHandle) (*Reader, error) {
				return obj.NewReader(ctx)
			},
		},
		{
			"NewRangeReader(0, -1)",
			func(ctx context.Context, obj *ObjectHandle) (*Reader, error) {
				return obj.NewRangeReader(ctx, 0, -1)
			},
		},
		{
			"NewRangeReader(1kB, 2kB)",
			func(ctx context.Context, obj *ObjectHandle) (*Reader, error) {
				return obj.NewRangeReader(ctx, 1<<10, 2<<10)
			},
		},
		{
			"NewRangeReader(2kB, -1)",
			func(ctx context.Context, obj *ObjectHandle) (*Reader, error) {
				return obj.NewRangeReader(ctx, 2<<10, -1)
			},
		},
		{
			"NewRangeReader(2kB, 3kB)",
			func(ctx context.Context, obj *ObjectHandle) (*Reader, error) {
				return obj.NewRangeReader(ctx, 2<<10, 3<<10)
			},
		},
	}

	for _, tt := range readerCreators {
		t.Run(tt.name, func(t *testing.T) {
			obj := client.Bucket("bucket").Object("object")
			_, err := obj.Attrs(ctx)
			if err != nil {
				t.Fatal(err)
			}
			rd, err := tt.create(ctx, obj)
			if err != nil {
				t.Fatal(err)
			}
			defer rd.Close()

			got, err := ioutil.ReadAll(rd)
			if err != nil {
				t.Fatal(err)
			}
			if g, w := got, original; !bytes.Equal(g, w) {
				t.Fatalf("Response mismatch\nGot:\n%q\n\nWant:\n%q", g, w)
			}
		})
	}
}

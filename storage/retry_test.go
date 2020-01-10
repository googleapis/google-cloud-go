// Copyright 2019 Google LLC
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

package storage_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestIndefiniteRetries(t *testing.T) {
	t.Skip("https://github.com/googleapis/google-cloud-go/issues/1641")

	if testing.Short() {
		t.Skip("A long running test for retries")
	}

	uploadRoute := "/upload"

	var resumableUploadIDs atomic.Value
	resumableUploadIDs.Store(make(map[string]time.Time))

	lookupUploadID := func(resumableUploadID string) (time.Time, bool) {
		t, ok := resumableUploadIDs.Load().(map[string]time.Time)[resumableUploadID]
		return t, ok
	}

	memoizeUploadID := func(resumableUploadID string) {
		resumableUploadIDs.Load().(map[string]time.Time)[resumableUploadID] = time.Now().UTC()
	}

	cst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resumableUploadID := r.URL.Query().Get("upload_id")
		path := r.URL.Path

		switch {
		case path == "/b": // Bucket creation
			w.Write([]byte(`{"kind":"storage#bucket","id":"bucket","name":"bucket"}`))
			return

		case strings.HasPrefix(path, "/b/") && strings.HasSuffix(path, "/o"):
			if resumableUploadID == "" {
				uploadID := time.Now().Format(time.RFC3339Nano)
				w.Header().Set("X-GUploader-UploadID", uploadID)
				// Now for the resumable upload URL.
				w.Header().Set("Location", fmt.Sprintf("http://%s?upload_id=%s", r.Host+uploadRoute, uploadID))
			} else {
				w.Write([]byte(`{"kind":"storage#object","bucket":"bucket","name":"bucket"}`))
			}

			return

		case path == uploadRoute:
			start, _, _, completedUpload, spamThem := parseContentRange(r.Header)

			if resumableUploadID != "" {
				_, ok := lookupUploadID(resumableUploadID)
				if !ok {
					if start == "0" {
						// First time that we are encountering this upload
						// and it is at byte 0, so memoize the uploadID.
						memoizeUploadID(resumableUploadID)
					} else {
						// If the start and end range are non-zero this is the exact
						// error in https://github.com/googleapis/google-cloud-go/issues/1507
						// mismatched_content_start (Invalid request. According to the Content-Range header,
						// the upload offset is 1082130432 byte(s), which exceeds already uploaded size of 0 byte(s).)
						errStr := fmt.Sprintf("mismatched_content_start (Invalid request. According to the Content-Range header,"+
							"the upload offset is %s byte(s), which exceeds already uploaded size of 0 byte(s).)\n%s", start, r.Header["Content-Range"])
						http.Error(w, errStr, http.StatusServiceUnavailable)
						return
					}
				}
			}

			if spamThem {
				// Reproduce https://github.com/googleapis/google-cloud-go/issues/1507
				// by sending then a retryable error on the last byte.
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}

			if completedUpload {
				// Completed the upload.
				return
			}

			// Consume the body since we can accept this body.
			_, _ = ioutil.ReadAll(r.Body)

			w.Header().Set("X-Http-Status-Code-Override", "308")
			return

		default:
			http.Error(w, "Unimplemented", http.StatusNotFound)
			return
		}
	}))
	defer cst.Close()

	hc := &http.Client{
		Transport: &oauth2.Transport{
			Source: new(tokenSupplier),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := []option.ClientOption{option.WithHTTPClient(hc), option.WithEndpoint(cst.URL)}

	sc, err := storage.NewClient(ctx, opts...)
	if err != nil {
		t.Fatalf("Failed to create storage client: %v", err)
	}
	defer sc.Close()

	obj := sc.Bucket("issue-1507").Object("object")
	w := obj.NewWriter(ctx)

	maxFileSize := 1 << 20
	chunkSize := maxFileSize / 4

	w.ChunkSize = chunkSize

	for i := 0; i < maxFileSize; {
		nowStr := time.Now().Format(time.RFC3339Nano)
		n, _ := fmt.Fprintf(w, "%s%s", nowStr, strings.Repeat("a", w.ChunkSize))
		i += n
	}

	closeDone := make(chan error, 1)
	go func() {
		// Invoking w.Close() to ensure that this triggers completion of the upload.
		closeDone <- w.Close()
	}()

	// Given that the ExponentialBackoff is 30 seconds from a start of 100ms,
	// let's wait for a maximum of 5 minutes to account for (2**n) increments
	// between [100ms, 30s].
	maxWait := 5 * time.Minute
	select {
	case <-time.After(maxWait):
		t.Fatalf("Test took longer than %s to return", maxWait)
	case err := <-closeDone:
		ge, ok := err.(*googleapi.Error)
		if !ok {
			t.Fatalf("Got error (%v) of type %T, expected *googleapi.Error", err, err)
		}
		if ge.Code != http.StatusTooManyRequests {
			t.Fatalf("Got unexpected error: %#v\nWant statusCode of %d", ge, http.StatusTooManyRequests)
		}
	}
}

type tokenSupplier int

func (ts *tokenSupplier) Token() (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken:  "access-token",
		TokenType:    "Bearer",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}, nil
}

func parseContentRange(hdr http.Header) (start, end, max string, completed, spamThem bool) {
	cRange := strings.TrimPrefix(hdr.Get("Content-Range"), "bytes ")
	rangeSplits := strings.Split(cRange, "/")
	prelude := rangeSplits[0]
	max = rangeSplits[1]
	if len(prelude) == 0 || prelude == "*" {
		// Completed the upload.
		completed = true
		return
	}

	startEndSplit := strings.Split(prelude, "-")
	start, end = startEndSplit[0], startEndSplit[1]

	if max != "*" { // They've uploaded the last byte.
		// Reproduce https://github.com/googleapis/google-cloud-go/issues/1507
		// by sending then a retryable error on the last byte.
		spamThem = true
	}

	return
}

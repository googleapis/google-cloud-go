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

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const wantEntries = 5

// responses is used for faking HTTP responses. Format each value with a timestamp based on whether get() should
// fetch another page.
var responses = []string{
	`{"Path":"golang.org/x/net","Version":"v0.0.0-20180627171509-e514e69ffb8b","Timestamp":"2019-04-10T20:30:42.413493Z"}
	 {"Path":"golang.org/x/exp/notary","Version":"v0.0.0-20190409044807-56b785ea58b2","Timestamp":"2019-04-10T20:37:01.409908Z"}
	 {"Path":"golang.org/x/crypto","Version":"v0.0.0-20181025213731-e84da0312774","Timestamp":"2019-04-10T20:40:43.408586Z"}
	 {"Path":"golang.org/x/net","Version":"v0.0.0-20181213202711-891ebc4b82d6","Timestamp":"2019-04-10T20:40:54.12906Z"}
	 {"Path":"cloud.google.com/go","Version":"v1.0.0","Timestamp":"%s"}`,

	`{"Path":"cloud.google.com/go/storage","Version":"v1.0.0","Timestamp":"2020-04-10T20:40:43.408586Z"}
	 {"Path":"cloud.google.com/go/bigquery","Version":"v1.0.0","Timestamp":"2020-04-10T20:40:43.408586Z"}
	 {"Path":"golang.org/x/crypto","Version":"v0.0.0-20181025213731-e84da0312774","Timestamp":"2020-04-10T20:40:43.408586Z"}
	 {"Path":"cloud.google.com/go/spanner","Version":"v1.0.0","Timestamp":"2020-04-10T20:40:43.408586Z"}
	 {"Path":"cloud.google.com/go","Version":"v1.0.0","Timestamp":"%s"}`,
}

func TestGet(t *testing.T) {
	numCalls := 0
	lastTime := time.Now().Add(-time.Minute).Format(time.RFC3339)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		numCalls++
		if numCalls == 1 {
			resp := []byte(fmt.Sprintf(responses[0], time.Now().Add(-time.Hour).Format(time.RFC3339)))
			w.Write(resp)
			return
		}
		if numCalls == 2 {
			resp := []byte(fmt.Sprintf(responses[1], lastTime))
			w.Write(resp)
			return
		}
		t.Fatalf("Unexpected call #%d to server", numCalls)
	}))

	ic := indexClient{indexURL: s.URL}

	entries, last, err := ic.get(context.Background(), []string{"cloud.google.com"}, time.Time{})
	if err != nil {
		t.Fatalf("get got err: %v", err)
	}
	if got, want := len(entries), wantEntries; got != want {
		t.Errorf("get got %d entries, want %d", got, want)
	}

	want, err := time.Parse(time.RFC3339, lastTime)
	if err != nil {
		t.Fatalf("Failed to parse time: %v", err)
	}
	if !last.Equal(want) {
		t.Errorf("get got last time %v, want %v", last, lastTime)
	}
}

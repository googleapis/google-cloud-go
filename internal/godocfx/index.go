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
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		log.Fatalf("Failed to parse URL: %q", s)
	}
	return u
}

// indexClient is used to access index.golang.org.
type indexClient struct {
	indexURL string
}

// indexEntry represents a line in the output of index.golang.org/index.
type indexEntry struct {
	Path      string
	Version   string
	Timestamp time.Time
}

// get returns modules with the given prefix that were published since the given timestamp.
//
// get uses index.golang.org/index?since=timestamp to find new module versions.
//
// get returns the entries and the timestamp of the last seen entry, which may be later
// than the timestamp of the last returned entry.
func (ic indexClient) get(ctx context.Context, prefixes []string, since time.Time) ([]indexEntry, time.Time, error) {
	fiveMinAgo := time.Now().Add(-5 * time.Minute).UTC() // When to stop processing.
	entries := []indexEntry{}
	log.Printf("Fetching index.golang.org entries since %s", since.Format(time.RFC3339))
	pages := 0
	lastSeen := since
	for !lastSeen.After(fiveMinAgo) {
		log.Printf("last seen: %v", lastSeen.Format(time.RFC3339))
		pages++
		var cur []indexEntry
		var err error
		cur, lastSeen, err = ic.getPage(prefixes, lastSeen)
		if err != nil {
			return nil, time.Time{}, err
		}
		entries = append(entries, cur...)
	}
	log.Printf("Parsed %d index.golang.org pages from %s to %s", pages, since.Format(time.RFC3339), lastSeen.Format(time.RFC3339))

	return entries, lastSeen, nil
}

// get fetches a single chronological page of modules from
// index.golang.org/index.
func (ic indexClient) getPage(prefixes []string, since time.Time) ([]indexEntry, time.Time, error) {
	entries := []indexEntry{}

	u := mustParse(ic.indexURL)
	q := u.Query()
	q.Set("since", since.Format(time.RFC3339))
	u.RawQuery = q.Encode()
	log.Printf("Fetching %s", u.String())
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, time.Time{}, err
	}

	s := bufio.NewScanner(resp.Body)
	last := time.Time{}
	for s.Scan() {
		e := indexEntry{}
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			return nil, time.Time{}, err
		}
		last = e.Timestamp // Always update the last timestamp.
		if !hasPrefix(e.Path, prefixes) ||
			strings.Contains(e.Path, "internal") ||
			strings.Contains(e.Path, "third_party") ||
			strings.Contains(e.Version, "-") { // Filter out pseudo-versions.
			continue
		}
		entries = append(entries, e)
	}
	return entries, last, nil
}

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

// +build go1.15

package main

import (
	"context"
	"testing"
	"time"
)

const wantEntries = 5

type fakeIC struct{}

func (f fakeIC) get(prefix string, since time.Time) (entries []indexEntry, last time.Time, err error) {
	e := indexEntry{Timestamp: since.Add(24 * time.Hour)}
	return []indexEntry{e}, e.Timestamp, nil
}

type fakeTS struct {
	getCalled, putCalled bool
}

func (f *fakeTS) get(context.Context) (time.Time, error) {
	f.getCalled = true
	t := time.Now().Add(-wantEntries * 24 * time.Hour).UTC()
	return t, nil
}

func (f *fakeTS) put(context.Context, time.Time) error {
	f.putCalled = true
	return nil
}

func TestNewModules(t *testing.T) {
	ic := fakeIC{}
	ts := &fakeTS{}
	entries, err := newModules(context.Background(), ic, ts, "cloud.google.com")
	if err != nil {
		t.Fatalf("newModules got err: %v", err)
	}
	if got, want := len(entries), wantEntries; got != want {
		t.Errorf("newModules got %d entries, want %d", got, want)
	}
	if !ts.getCalled {
		t.Errorf("fakeTS.get was never called")
	}
	if !ts.putCalled {
		t.Errorf("fakeTS.put was never called")
	}
}

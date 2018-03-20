// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build go1.8

package datastore

import (
	"testing"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/net/context"
)

func TestOCTracing(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}

	te := &testExporter{}
	trace.RegisterExporter(te)
	defer trace.UnregisterExporter(te)
	trace.SetDefaultSampler(trace.AlwaysSample())

	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type SomeValue struct {
		S string
	}
	_, err := client.Put(ctx, IncompleteKey("SomeKey", nil), &SomeValue{"foo"})
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}

	time.Sleep(time.Second)

	if len(te.c) == 0 {
		t.Fatal("Expected some trace to be created, but none was")
	}
}

type testExporter struct {
	c []*trace.SpanData
}

func (te *testExporter) ExportSpan(s *trace.SpanData) {
	te.c = append(te.c, s)
}

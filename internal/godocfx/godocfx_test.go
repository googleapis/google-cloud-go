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
	"testing"

	_ "cloud.google.com/go/storage" // Implicitly required by test.
)

func TestParse(t *testing.T) {
	testPath := "cloud.google.com/go/storage"
	pages, toc, module, err := parse(testPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := len(toc), 1; got != want {
		t.Fatalf("Parse got len(toc) = %d, want %d", got, want)
	}
	if got, want := len(pages), 1; got != want {
		t.Errorf("Parse got len(pages) = %d, want %d", got, want)
	}
	if got := module.Path; got != testPath {
		t.Fatalf("Parse got module = %q, want %q", got, testPath)
	}

	page := pages[testPath]

	// Check invariants for every item.
	for _, item := range page.Items {
		if got := item.UID; got == "" {
			t.Errorf("Parse found missing UID: %v", item)
		}

		if got, want := item.Langs, []string{"go"}; len(got) != 1 || got[0] != want[0] {
			t.Errorf("Parse %v got langs = %v, want %v", item.UID, got, want)
		}
	}

	// Check there is at least one type, const, variable, and function.
	// Note: no method because they aren't printed for Namespaces yet.
	wants := []string{"type", "const", "variable", "function"}
	for _, want := range wants {
		found := false
		for _, c := range page.Items {
			if c.Type == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Parse got no %q, want at least one", want)
		}
	}
}

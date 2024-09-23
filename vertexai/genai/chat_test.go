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

package genai

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCopySanitizedModelContent(t *testing.T) {
	var tests = []struct {
		partsIn   []Part
		wantParts []Part
	}{
		{[]Part{Text("foobar"), Text("")},
			[]Part{Text("foobar")}},
		{[]Part{Text(""), Text("foobar"), Text(""), Text("a b c")},
			[]Part{Text("foobar"), Text("a b c")}},
		{[]Part{Text(""), Text("foobar"), Text(""), Text(""), Blob{MIMEType: "png"}},
			[]Part{Text("foobar"), Blob{MIMEType: "png"}}},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%v", tt.partsIn)
		t.Run(testname, func(t *testing.T) {
			got := copySanitizedModelContent(&Content{Role: "model", Parts: tt.partsIn})
			if !cmp.Equal(got.Parts, tt.wantParts) {
				t.Errorf("got %v, want %v", got.Parts, tt.wantParts)
			}
		})
	}
}

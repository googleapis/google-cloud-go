// Copyright 2023 Google LLC
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

package support

import (
	"reflect"
	"strconv"
	"testing"
)

func TestTransformMapValues(t *testing.T) {
	var from map[string]int
	got := TransformMapValues(from, strconv.Itoa)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	from = map[string]int{"one": 1, "two": 2}
	got = TransformMapValues(from, strconv.Itoa)
	want := map[string]string{"one": "1", "two": "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

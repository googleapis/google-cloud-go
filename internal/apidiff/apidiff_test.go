// Copyright 2021 Google LLC
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

// +build linux darwin

package main

import "testing"

func TestCheckAllowBreakingChange(t *testing.T) {
	for _, tst := range []struct {
		name, msg string
		want      bool
	}{
		{
			name: "disallow - no indicator",
			msg:  "feat: add foo",
			want: false,
		},
		{
			name: "allow - bang indicator",
			msg:  "feat!: remove foo",
			want: true,
		},
		{
			name: "allow - bang indicator pre-scope",
			msg:  "feat!(scope): remove foo",
			want: true,
		},
		{
			name: "allow - tag indicator",
			msg:  "BREAKING CHANGE: remove foo",
			want: true,
		},
		{
			name: "allow - multiline bang indicator",
			msg: `feat: add foo
			feat!: remove bar
			chore: update dep`,
			want: true,
		},
		{
			name: "allow - multiline tag indicator",
			msg: `feat: add foo
			BREAKING CHANGE: remove bar
			chore: update dep`,
			want: true,
		},
	} {
		if got := checkAllowBreakingChange(tst.msg); got != tst.want {
			t.Errorf("%s: got %v want %v", tst.name, got, tst.want)
		}
	}
}

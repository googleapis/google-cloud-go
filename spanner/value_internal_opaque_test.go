// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build spanner_opaque

package spanner

import "strings"

func brokenRowErrorEqual(got, want error, row *Row) bool {
	if len(row.vals) == 1 && row.vals[0] != nil && internalValueKindOf(row.vals[0]) == internalValueUnset {
		return got == nil
	}
	if got == nil || want == nil {
		return got == want
	}
	wantText := strings.NewReplacer(
		"*structpb.Value_NullValue", "null",
		"*structpb.Value_NumberValue", "number",
		"*structpb.Value_StringValue", "string",
		"*structpb.Value_BoolValue", "bool",
		"*structpb.Value_StructValue", "struct",
		"*structpb.Value_ListValue", "list",
	).Replace(want.Error())
	return got.Error() == wantText
}

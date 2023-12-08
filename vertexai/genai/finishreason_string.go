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

package genai

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var x [1]struct{}
	_ = x[FinishReasonUnspecified-0]
	_ = x[FinishReasonStop-1]
	_ = x[FinishReasonMaxTokens-2]
	_ = x[FinishReasonSafety-3]
	_ = x[FinishReasonRecitation-4]
	_ = x[FinishReasonOther-5]
}

const _FinishReasonName = "FinishReasonUnspecifiedFinishReasonStopFinishReasonMaxTokensFinishReasonSafetyFinishReasonRecitationFinishReasonOther"

var _FinishReasonIndex = [...]uint8{0, 23, 39, 60, 78, 100, 117}

func (i FinishReason) String() string {
	if i < 0 || i >= FinishReason(len(_FinishReasonIndex)-1) {
		return "FinishReason(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _FinishReasonName[_FinishReasonIndex[i]:_FinishReasonIndex[i+1]]
}

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

package sentencepiece

import "strings"

// normalize performs unicode normalization.
//
// SentencePiece has a feature to perform configurable unicode normalization on
// the input text and has some options for adding dummy whitespace prefixes or
// trimming whitespace. However, the model we're working with has a very simple
// normalizer that does none of this. These options can be added in the future
// if needed.
func normalize(text string) string {
	return replaceSpacesBySeparator(text)
}

const whitespaceSeparator = "▁"

// replaceSpacesBySeparator replaces spaces by the whitespace separator used by
// the model.
func replaceSpacesBySeparator(text string) string {
	return strings.ReplaceAll(text, " ", whitespaceSeparator)
}

// replaceSeparatorsBySpace replaces the whitespace separator used by
// the model back with spaces.
func replaceSeparatorsBySpace(text string) string {
	return strings.ReplaceAll(text, whitespaceSeparator, " ")
}

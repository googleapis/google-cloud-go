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

import "fmt"

// Token represents a single token from the input text. ID is a unique token
// identifier that the model uses in its internal representation. Text is
// the piece of text this token represents.
type Token struct {
	ID   int
	Text string
}

func (t Token) String() string {
	return fmt.Sprintf("Token{ID: %v, Text: %q}", t.ID, t.Text)
}

// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package tokenizer_test

import (
	"fmt"
	"log"

	"cloud.google.com/go/vertexai/genai"
	"cloud.google.com/go/vertexai/genai/tokenizer"
)

func ExampleTokenizer_CountTokens() {
	tok, err := tokenizer.New("gemini-1.5-flash")
	if err != nil {
		log.Fatal(err)
	}

	ntoks, err := tok.CountTokens(genai.Text("a prompt"), genai.Text("another prompt"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("total token count:", ntoks.TotalTokens)

	// Output: total token count: 4
}

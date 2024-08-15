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

package sentencepiece_test

import (
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/vertexai/internal/sentencepiece"
)

func ExampleEncode() {
	protoFile := os.Getenv("MODELPATH")
	if protoFile == "" {
		log.Println("Need MODELPATH env var to run example")
		return
	}

	enc, err := sentencepiece.NewEncoderFromPath(protoFile)
	if err != nil {
		log.Fatal(err)
	}

	text := "Encoding produces tokens that LLMs can learn and understand"
	tokens := enc.Encode(text)

	for _, token := range tokens {
		fmt.Println(token)
	}
}

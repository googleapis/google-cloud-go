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

// Command gensnippets writes all of the GoDoc examples to the given
// output directory.
//
// Every module in the current directory is processed. You can optionally
// pass a directory to process instead.
package main

import (
	"flag"
	"log"

	"cloud.google.com/go/internal/gapicgen/gensnippets"
)

func main() {
	outDir := flag.String("out", "internal/generated/snippets", "Output directory (default internal/generated/snippets)")

	flag.Parse()

	rootDir := "."
	if flag.NArg() > 0 {
		rootDir = flag.Arg(0)
	}
	// TODO(tbp): route proper api short names
	if err := gensnippets.Generate(rootDir, *outDir, map[string]string{}); err != nil {
		log.Fatal(err)
	}
}

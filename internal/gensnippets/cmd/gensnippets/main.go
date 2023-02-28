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

	"cloud.google.com/go/internal/gapicgen/generator"
	"cloud.google.com/go/internal/gensnippets"
)

func main() {
	outDir := flag.String("out", "internal/generated/snippets", "Output directory (default internal/generated/snippets)")
	googleapisDir := flag.String("googleapis-dir", "", "Root directory of googleapis/googleapis")

	flag.Parse()

	rootDir := "."
	if flag.NArg() > 0 {
		rootDir = flag.Arg(0)
	}
	apiShortnames, err := generator.ParseAPIShortnames(*googleapisDir, generator.MicrogenGapicConfigs, generator.ManualEntries)
	if err != nil {
		log.Fatalf("unable to parse shortnames: %v", err)
	}

	if err := gensnippets.Generate(rootDir, *outDir, apiShortnames); err != nil {
		log.Fatal(err)
	}
}

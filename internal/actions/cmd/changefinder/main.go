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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/internal/actions/changes"
	"cloud.google.com/go/internal/actions/logg"
)

var (
	dir       = flag.String("dir", "", "the root directory to evaluate")
	format    = flag.String("format", "plain", "output format, one of [plain|github], defaults to 'plain'")
	ghVarName = flag.String("gh-var", "submodules", "github format's variable name to set output for, defaults to 'submodules'.")
	base      = flag.String("base", "origin/main", "the base ref to compare to, defaults to 'origin/main'")
)

func main() {
	flag.BoolVar(&logg.Quiet, "q", false, "quiet mode, minimal logging")
	flag.Parse()
	rootDir, err := os.Getwd()
	if err != nil {
		logg.Fatal(err)
	}
	if *dir != "" {
		rootDir = *dir
	}
	logg.Printf("Root dir: %q", rootDir)

	updatedSubmoduleDirs, err := changes.GatherChangedModuleDirs(rootDir, *base)
	if err != nil {
		logg.Fatal(err)
	}

	output(updatedSubmoduleDirs)
}

func output(s []string) {
	switch *format {
	case "github":
		b, err := json.Marshal(s)
		if err != nil {
			logg.Fatalf("unable to marshal submodules: %v", err)
		}
		fmt.Printf("::set-output name=%s::%s", *ghVarName, b)
	case "plain":
		fallthrough
	default:
		fmt.Println(strings.Join(s, "\n"))
	}
}

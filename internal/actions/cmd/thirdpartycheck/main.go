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

package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	"cloud.google.com/go/internal/actions/logg"
	"golang.org/x/mod/modfile"
)

var (
	dir = flag.String("dir", "", "the root directory to evaluate")
	mod = flag.String("mod", "", "path to go.mod file relative to root directory")

	// List of allowlist module prefixes.
	allowlist = []string{
		// First party deps.
		"cloud.google.com/go",
		"github.com/GoogleCloudPlatform/",
		"github.com/google/",
		"github.com/googleapis/",
		"github.com/golang/",
		"google.golang.org/",
		"golang.org/",

		// Third party deps (allowed).
		"go.opencensus.io",
		"go.opentelemetry.io/",
		"gopkg.in/yaml",
		"github.com/go-git/go-git",
		"github.com/apache/arrow/go",
		"github.com/cloudprober/cloudprober", // https://github.com/googleapis/google-cloud-go/issues/9377

		// Third party deps (temporary exception(s)).
		"go.einride.tech/aip", // https://github.com/googleapis/google-cloud-go/issues/9338
		"rsc.io/binaryregexp", // https://github.com/googleapis/google-cloud-go/issues/9376
	}
)

func main() {
	flag.BoolVar(&logg.Quiet, "q", false, "quiet mode, minimal logging")
	flag.Parse()
	if *mod == "" {
		logg.Fatalf("missing required flag: -mod")
	}

	rootDir, err := os.Getwd()
	if err != nil {
		logg.Fatal(err)
	}
	if *dir != "" {
		rootDir = *dir
	}

	modPath := path.Join(rootDir, *mod)
	findings, err := check(modPath)
	if err != nil {
		logg.Fatal(err)
	}

	if len(findings) != 0 {
		fmt.Println("found disallowed module(s) in direct dependencies:")
		fmt.Println()
		fmt.Printf("\t%s\n", strings.Join(findings, "\n\t"))
		fmt.Println()
		os.Exit(1)
	}
}

// check reads & parses the specified go.mod, then validates that
// all direct dependencies are part of the allowlist.
func check(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	m, err := modfile.ParseLax(*mod, data, nil)
	if err != nil {
		return nil, err
	}

	var disallowed []string
	for _, r := range m.Require {
		if r.Indirect {
			continue
		}
		var allowed bool
		for _, a := range allowlist {
			if strings.HasPrefix(r.Mod.Path, a) {
				allowed = true
				break
			}
		}
		if !allowed {
			disallowed = append(disallowed, r.Mod.Path)
		}
	}
	return disallowed, nil
}

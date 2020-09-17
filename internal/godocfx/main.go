// Copyright 2020 Google LLC
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

// +build go1.15

/*Command godocfx generates DocFX YAML for Go code.

Usage:

    godocfx [flags] path

    cd module && godocfx ./...
    godocfx cloud.google.com/go/...
    godocfx -print cloud.google.com/go/storage/...
    godocfx -out custom/output/dir cloud.google.com/go/...
    godocfx -rm cloud.google.com/go/...

See:
* https://dotnet.github.io/docfx/spec/metadata_format_spec.html
* https://github.com/googleapis/doc-templates
* https://github.com/googleapis/doc-pipeline

TODO:
  * Cross link referenced packages.
*/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v2"
)

func main() {
	print := flag.Bool("print", false, "Print instead of save (default false)")
	rm := flag.Bool("rm", false, "Delete out directory before generating")
	outDir := flag.String("out", "obj/api", "Output directory (default obj/api)")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatalf("%s missing required argument: module path", os.Args[0])
	}

	pages, toc, module, err := parse(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	if *print {
		if err := yaml.NewEncoder(os.Stdout).Encode(pages); err != nil {
			log.Fatal(err)
		}
		fmt.Println("----- toc.yaml")
		if err := yaml.NewEncoder(os.Stdout).Encode(toc); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *rm {
		os.RemoveAll(*outDir)
	}

	if err := write(*outDir, pages, toc, module); err != nil {
		log.Fatalf("write: %v", err)
	}
}

func write(outDir string, pages map[string]*page, toc tableOfContents, module *packages.Module) error {
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return fmt.Errorf("os.MkdirAll: %v", err)
	}
	for path, p := range pages {
		// Make the module root page the index.
		if path == module.Path {
			path = "index"
		}
		// Trim the module path from all other paths.
		path = strings.TrimPrefix(path, module.Path+"/")
		path = filepath.Join(outDir, path+".yml")
		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return fmt.Errorf("os.MkdirAll: %v", err)
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintln(f, "### YamlMime:UniversalReference")
		if err := yaml.NewEncoder(f).Encode(p); err != nil {
			return err
		}

		path = filepath.Join(outDir, "toc.yml")
		f, err = os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintln(f, "### YamlMime:TableOfContent")
		if err := yaml.NewEncoder(f).Encode(toc); err != nil {
			return err
		}
	}

	// Write the docuploader docs.metadata file. Not for DocFX.
	// See https://github.com/googleapis/docuploader/issues/11.
	// Example:
	/*
		update_time {
		  seconds: 1600048103
		  nanos: 183052000
		}
		name: "cloud.google.com/go"
		version: "v0.65.0"
		language: "go"
	*/
	path := filepath.Join(outDir, "docs.metadata")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	now := time.Now().UTC()
	fmt.Fprintf(f, `update_time {
  seconds: %d
  nanos: %d
}
name: %q
version: %q
language: "go"`, now.Unix(), now.Nanosecond(), module.Path, module.Version)
	return nil
}

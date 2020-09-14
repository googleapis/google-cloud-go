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

	"gopkg.in/yaml.v2"
)

func main() {
	print := flag.Bool("print", false, "Print instead of save (default false)")
	rm := flag.Bool("rm", false, "Delete out directory before generating")
	outDir := flag.String("out", "obj/api", "Output directory (default obj/api)")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatalf("%s expected 1 arg, got %d", os.Args[0], flag.NArg())
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
	if err := os.MkdirAll(*outDir, os.ModePerm); err != nil {
		log.Fatalf("os.MkdirAll: %v", err)
	}
	for path, p := range pages {
		// Make the module root page the index.
		if path == module.Path {
			path = "index"
		}
		// Trim the module path from all other paths.
		path = strings.TrimPrefix(path, module.Path+"/")
		path = filepath.Join(*outDir, path+".yml")
		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			log.Fatalf("os.MkdirAll: %v", err)
		}
		f, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		fmt.Fprintln(f, "### YamlMime:UniversalReference")
		if err := yaml.NewEncoder(f).Encode(p); err != nil {
			log.Fatal(err)
		}

		path = filepath.Join(*outDir, "toc.yml")
		f, err = os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		fmt.Fprintln(f, "### YamlMime:TableOfContent")
		if err := yaml.NewEncoder(f).Encode(toc); err != nil {
			log.Fatal(err)
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
	path := filepath.Join(*outDir, "docs.metadata")
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now().UTC()
	fmt.Fprintf(f, `update_time {
  seconds: %d
  nanos: %d
}
name: %q
version: %q
language: "go"`, now.Second(), now.Nanosecond(), module.Path, module.Version)
}

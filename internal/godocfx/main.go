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

// +build linux,go1.15

/*Command godocfx generates DocFX YAML for Go code.

Usage:

    godocfx [flags] path

    # New modules with the given prefix. Delete any previous output.
    godocfx -rm -project my-project -new-modules cloud.google.com/go
    # Process a single module @latest.
    godocfx cloud.google.com/go
    # Process and print, instead of save.
    godocfx -print cloud.google.com/go/storage@latest
    # Change output directory.
    godocfx -out custom/output/dir cloud.google.com/go

See:
* https://dotnet.github.io/docfx/spec/metadata_format_spec.html
* https://github.com/googleapis/doc-templates
* https://github.com/googleapis/doc-pipeline

TODO:
  * Cross link referenced packages.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

func main() {
	print := flag.Bool("print", false, "Print instead of save (default false)")
	rm := flag.Bool("rm", false, "Delete out directory before generating")
	outDir := flag.String("out", "obj/api", "Output directory (default obj/api)")
	projectID := flag.String("project", "", "Project ID to use. Required when using -new-modules.")
	newMods := flag.Bool("new-modules", false, "Process all new modules with the given prefix. Uses timestamp in Datastore. Stores results in $out/$mod.")

	log.SetPrefix("[godocfx] ")

	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatalf("%s missing required argument: module path/prefix", os.Args[0])
	}

	mod := flag.Arg(0)
	var mods []indexEntry
	if *newMods {
		if *projectID == "" {
			log.Fatal("Must set -project when using -new-modules")
		}
		var err error
		mods, err = newModules(context.Background(), indexClient{}, &dsTimeSaver{projectID: *projectID}, mod)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		modPath := mod
		version := "latest"
		if strings.Contains(mod, "@") {
			parts := strings.Split(mod, "@")
			if len(parts) != 2 {
				log.Fatal("module arg expected only one '@'")
			}
			modPath = parts[0]
			version = parts[1]
		}
		modPath = strings.TrimSuffix(modPath, "/...") // No /... needed.
		mods = []indexEntry{
			{
				Path:    modPath,
				Version: version,
			},
		}
	}

	if *rm {
		os.RemoveAll(*outDir)
	}
	if len(mods) == 0 {
		log.Println("No new modules to process")
		return
	}
	// Create a temp module so we can get the exact version asked for.
	tempDir, err := ioutil.TempDir("", "godocfx-*")
	if err != nil {
		log.Fatalf("ioutil.TempDir: %v", err)
	}
	runCmd(tempDir, "go", "mod", "init", "cloud.google.com/go/lets-build-some-docs")
	for _, m := range mods {
		log.Printf("Processing %s@%s", m.Path, m.Version)

		path := *outDir
		// If we have more than one module, we need a more specific out path.
		if len(mods) > 1 {
			path = filepath.Join(path, fmt.Sprintf("%s@%s", m.Path, m.Version))
		}
		if err := process(m, tempDir, path, *print); err != nil {
			log.Printf("Failed to process %v", m)
		}
		log.Printf("Done with %s@%s", m.Path, m.Version)
	}
}

func runCmd(dir, name string, args ...string) error {
	log.Printf("> [%s] %s %s", dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Start: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Wait: %s", err)
	}
	return nil
}

func process(mod indexEntry, tempDir, outDir string, print bool) error {
	// Be sure to get the module and run the module loader in the tempDir.
	if err := runCmd(tempDir, "go", "mod", "tidy"); err != nil {
		return err
	}
	if err := runCmd(tempDir, "go", "get", mod.Path+"@"+mod.Version); err != nil {
		return err
	}

	optionalExtraFiles := []string{
		"README.md",
	}
	r, err := parse(mod.Path+"/...", tempDir, optionalExtraFiles)
	if err != nil {
		return fmt.Errorf("parse: %v", err)
	}

	if print {
		if err := yaml.NewEncoder(os.Stdout).Encode(r.pages); err != nil {
			return fmt.Errorf("Encode: %v", err)
		}
		fmt.Println("----- toc.yaml")
		if err := yaml.NewEncoder(os.Stdout).Encode(r.toc); err != nil {
			return fmt.Errorf("Encode: %v", err)
		}
		return nil
	}

	if err := write(outDir, r); err != nil {
		log.Fatalf("write: %v", err)
	}
	return nil
}

func write(outDir string, r *result) error {
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return fmt.Errorf("os.MkdirAll: %v", err)
	}
	for path, p := range r.pages {
		// Make the module root page the index.
		if path == r.module.Path {
			path = "index"
		}
		// Trim the module path from all other paths.
		path = strings.TrimPrefix(path, r.module.Path+"/")
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
		if err := yaml.NewEncoder(f).Encode(r.toc); err != nil {
			return err
		}
	}

	for _, ef := range r.extraFiles {
		src, err := os.Open(filepath.Join(r.module.Dir, ef.srcRelativePath))
		if err != nil {
			return err
		}
		dst, err := os.Create(filepath.Join(outDir, ef.dstRelativePath))
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			return nil
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
language: "go"`, now.Unix(), now.Nanosecond(), r.module.Path, r.module.Version)
	return nil
}

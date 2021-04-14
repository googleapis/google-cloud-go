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
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/printer"
	"go/token"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/internal/godocfx/pkgload"
	"cloud.google.com/go/third_party/go/doc"
)

func main() {
	outDir := flag.String("out", "internal/generated/snippets", "Output directory (default internal/generated/snippets)")

	flag.Parse()

	rootDir := "."
	if flag.NArg() > 0 {
		rootDir = flag.Arg(0)
	}

	// Find all modules in the current directory.
	dirs := []string{}
	filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "internal" {
			return filepath.SkipDir
		}
		if d.Name() == "go.mod" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})

	log.Printf("Processing %v directories: %q\n", len(dirs), dirs)

	trimPrefix := "cloud.google.com/go"
	for _, dir := range dirs {
		// Load does not look at nested modules.
		pis, err := pkgload.Load("./...", dir, nil)
		if err != nil {
			log.Fatalf("failed to load packages: %v", err)
		}
		for _, pi := range pis {
			if err := processExamples(pi.Doc, pi.Fset, trimPrefix, *outDir); err != nil {
				log.Fatalf("failed to process examples: %v", err)
			}
		}
	}

	if len(dirs) > 0 {
		cmd := exec.Command("goimports", "-w", ".")
		cmd.Dir = *outDir
		if err := cmd.Run(); err != nil {
			log.Fatalf("failed to run goimports: %v", err)
		}
	}
}

func processExamples(pkg *doc.Package, fset *token.FileSet, trimPrefix, outDir string) error {
	trimmed := strings.TrimPrefix(pkg.ImportPath, trimPrefix)
	outDir = filepath.Join(outDir, trimmed)

	regionTag := "generated" + strings.ReplaceAll(trimmed, "/", "_")

	// Note: variables and constants don't have examples.

	for _, f := range pkg.Funcs {
		dir := filepath.Join(outDir, f.Name)
		if err := writeExamples(dir, f.Examples, fset, regionTag); err != nil {
			return err
		}
	}

	for _, t := range pkg.Types {
		dir := filepath.Join(outDir, t.Name)
		if err := writeExamples(dir, t.Examples, fset, regionTag); err != nil {
			return err
		}
		for _, f := range t.Funcs {
			fDir := filepath.Join(dir, f.Name)
			if err := writeExamples(fDir, f.Examples, fset, regionTag); err != nil {
				return err
			}
		}
		for _, m := range t.Methods {
			mDir := filepath.Join(dir, m.Name)
			if err := writeExamples(mDir, m.Examples, fset, regionTag); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeExamples(outDir string, exs []*doc.Example, fset *token.FileSet, regionTag string) error {
	if len(exs) == 0 {
		// Nothing to do.
		return nil
	}
	for _, ex := range exs {
		dir := outDir
		if len(exs) > 1 {
			// More than one example, so we need to disambiguate.
			dir = filepath.Join(outDir, ex.Suffix)
		}
		filename := filepath.Join(dir, "main.go")

		buf := &bytes.Buffer{}
		var node interface{} = &printer.CommentedNode{
			Node:     ex.Code,
			Comments: ex.Comments,
		}
		if ex.Play != nil {
			node = ex.Play
		}
		if err := format.Node(buf, fset, node); err != nil {
			return err
		}
		s := buf.String()
		if strings.HasPrefix(s, "{\n") && strings.HasSuffix(s, "\n}") {
			lines := strings.Split(s, "\n")
			builder := strings.Builder{}
			for _, line := range lines[1 : len(lines)-1] {
				builder.WriteString(strings.TrimPrefix(line, "\t"))
				builder.WriteString("\n")
			}
			s = builder.String()
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(header()); err != nil {
			return err
		}
		tag := regionTag + "_" + ex.Name
		// Include an extra newline to keep separate from the package declaration.
		if _, err := fmt.Fprintf(f, "// [START %v]\n\n", tag); err != nil {
			return err
		}
		if _, err := f.WriteString(s); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(f, "// [END %v]\n", tag); err != nil {
			return err
		}
	}
	return nil
}

func header() string {
	return fmt.Sprintf(licenseHeader, time.Now().Year())
}

const licenseHeader string = `// Copyright %v Google LLC
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

`

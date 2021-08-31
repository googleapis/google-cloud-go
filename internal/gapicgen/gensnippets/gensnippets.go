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

// Package gensnippets processes GoDoc examples.
package gensnippets

import (
	"bytes"
	"fmt"
	"go/format"
	"go/printer"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/internal/godocfx/pkgload"
	"cloud.google.com/go/third_party/go/doc"
	"golang.org/x/sys/execabs"
	"google.golang.org/genproto/googleapis/gapic/metadata"
	"google.golang.org/protobuf/encoding/protojson"
)

// Generate reads all modules in rootDir and outputs their examples in outDir.
func Generate(rootDir, outDir string, apiShortnames map[string]string) error {
	if rootDir == "" {
		rootDir = "."
	}
	if outDir == "" {
		outDir = "internal/generated/snippets"
	}

	// Find all modules in rootDir.
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

	log.Printf("Processing examples in %v directories: %q\n", len(dirs), dirs)

	trimPrefix := "cloud.google.com/go"
	errs := []error{}
	for _, dir := range dirs {
		// Load does not look at nested modules.
		pis, err := pkgload.Load("./...", dir, nil)
		if err != nil {
			return fmt.Errorf("failed to load packages: %v", err)
		}
		for _, pi := range pis {
			if eErrs := processExamples(pi.Doc, pi.Fset, trimPrefix, rootDir, outDir, apiShortnames); len(eErrs) > 0 {
				errs = append(errs, fmt.Errorf("%v", eErrs))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("example errors: %v", errs)
	}

	if len(dirs) > 0 {
		cmd := execabs.Command("goimports", "-w", ".")
		cmd.Dir = outDir
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run goimports: %v", err)
		}
	}

	return nil
}

var skip = map[string]bool{
	"cloud.google.com/go":                          true, // No product for root package.
	"cloud.google.com/go/civil":                    true, // General time/date package.
	"cloud.google.com/go/cloudbuild/apiv1":         true, // Has v2.
	"cloud.google.com/go/cmd/go-cloud-debug-agent": true, // Command line tool.
	"cloud.google.com/go/container":                true, // Deprecated.
	"cloud.google.com/go/containeranalysis/apiv1":  true, // Accidental beta at wrong path?
	"cloud.google.com/go/grafeas/apiv1":            true, // With containeranalysis.
	"cloud.google.com/go/httpreplay":               true, // Helper.
	"cloud.google.com/go/httpreplay/cmd/httpr":     true, // Helper.
	"cloud.google.com/go/longrunning":              true, // Helper.
	"cloud.google.com/go/monitoring/apiv3":         true, // Has v2.
	"cloud.google.com/go/translate":                true, // Has newer version.
}

func processExamples(pkg *doc.Package, fset *token.FileSet, trimPrefix, rootDir, outDir string, apiShortnames map[string]string) []error {
	if skip[pkg.ImportPath] {
		return nil
	}
	trimmed := strings.TrimPrefix(pkg.ImportPath, trimPrefix)
	regionTags, err := computeRegionTags(rootDir, trimmed, apiShortnames)
	if err != nil {
		return []error{err}
	}
	if len(regionTags) == 0 {
		// Nothing to do.
		return nil
	}
	outDir = filepath.Join(outDir, trimmed)

	// Note: only process methods because they correspond to RPCs.

	var errs []error
	for _, t := range pkg.Types {
		for _, m := range t.Methods {
			if len(m.Examples) == 0 {
				// Nothing to do for this method.
				continue
			}
			dir := filepath.Join(outDir, t.Name, m.Name)
			regionTag, ok := regionTags[t.Name][m.Name]
			if !ok {
				errs = append(errs, fmt.Errorf("could not find region tag for %s %s.%s", pkg.ImportPath, t.Name, m.Name))
				continue
			}
			if err := writeExamples(dir, m.Examples, fset, regionTag); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// computeRegionTags gets the region tags for the given path, keyed by client name and method name.
func computeRegionTags(rootDir, path string, apiShortnames map[string]string) (regionTags map[string]map[string]string, err error) {
	metadataPath := filepath.Join(rootDir, path, "gapic_metadata.json")
	f, err := os.ReadFile(metadataPath)
	if err != nil {
		// If there is no gapic_metadata.json file, don't generate snippets.
		// This isn't an error, though, because some packages aren't GAPICs and
		// shouldn't get snippets in the first place.
		return nil, nil
	}
	m := metadata.GapicMetadata{}
	if err := protojson.Unmarshal(f, &m); err != nil {
		return nil, err
	}
	shortname, ok := apiShortnames[m.GetLibraryPackage()]
	if !ok {
		return nil, fmt.Errorf("could not find shortname for %q", m.GetLibraryPackage())
	}
	protoParts := strings.Split(m.GetProtoPackage(), ".")
	apiVersion := protoParts[len(protoParts)-1]

	regionTags = map[string]map[string]string{}
	for sName, s := range m.GetServices() {
		for _, c := range s.GetClients() {
			for rpc, methods := range c.GetRpcs() {
				if len(methods.GetMethods()) != 1 {
					return nil, fmt.Errorf("%s %s %s found %d methods", m.GetLibraryPackage(), sName, c.GetLibraryClient(), len(methods.GetMethods()))
				}
				if methods.GetMethods()[0] != rpc {
					return nil, fmt.Errorf("%s %s %s %q does not match %q", m.GetLibraryPackage(), sName, c.GetLibraryClient(), methods.GetMethods()[0], rpc)
				}

				// Every Go method is synchronous.
				regionTag := fmt.Sprintf("%s_%s_generated_%s_%s_sync", shortname, apiVersion, sName, rpc)

				if regionTags[c.GetLibraryClient()] == nil {
					regionTags[c.GetLibraryClient()] = map[string]string{}
				}
				regionTags[c.GetLibraryClient()][rpc] = regionTag
			}
		}
	}
	return regionTags, nil
}

func writeExamples(outDir string, exs []*doc.Example, fset *token.FileSet, regionTag string) error {
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

		tag := regionTag
		if len(ex.Suffix) > 0 {
			tag += "_" + ex.Suffix
		}

		// Include an extra newline to keep separate from the package declaration.
		if _, err := fmt.Fprintf(f, "// [START %v]\n\n", tag); err != nil {
			return err
		}
		if _, err := f.WriteString(s); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(f, "\n// [END %v]\n", tag); err != nil {
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

// Code generated by cloud.google.com/go/internal/gapicgen/gensnippets. DO NOT EDIT.

`

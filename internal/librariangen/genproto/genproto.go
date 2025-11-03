// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package genproto

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/execv"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/protoc"
	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/request"
)

var (
	execvRun = execv.Run
)

var goPkgOptRe = regexp.MustCompile(`(?m)^option go_package = (.*);`)

// RepoType is the value that should appear in repo-config.yaml to use the functions in this package.
const RepoType string = "go-genproto"

// Generate generates all relevant protos from sourceDir into outputDir,
// determining which golang packages are in scope based on the configured exclusions.
func Generate(ctx context.Context, sourceDir string, outputDir string, generateReq *request.Library) error {
	packages, err := derivePackages(generateReq)
	if err != nil {
		return err
	}
	// Figure out what we're generating: a map from a full package name
	// such as "google.golang.org/genproto/googleapis/type/date_range" to the proto files
	// to generate for that package.
	protos, err := protosForPackages(sourceDir, generateReq, packages)
	if err != nil {
		return err
	}
	// Now call protoc on the protos for each package.
	slog.Info("generating protos")
	args := protoc.BuildGenProto(sourceDir, outputDir, protos)
	if err := execvRun(ctx, args, outputDir); err != nil {
		return fmt.Errorf("librariangen: protoc failed: %w", err)
	}

	// Move the output to the right place.
	generatedPath := filepath.Join(outputDir, "google.golang.org", "genproto", "googleapis")
	wantedPath := filepath.Join(outputDir, "googleapis")
	if err := os.Rename(generatedPath, wantedPath); err != nil {
		return err
	}

	// Run goimports -w on the output root.
	if err := goimports(ctx, outputDir); err != nil {
		return fmt.Errorf("librariangen: failed to run 'goimports': %w", err)
	}
	return nil
}

// goimports runs the goimports tool on a directory to format Go files and
// manage imports.
func goimports(ctx context.Context, dir string) error {
	slog.Info("librariangen: running goimports", "directory", dir)
	// The `.` argument will make goimports process all go files in the directory
	// and its subdirectories. The -w flag writes results back to source files.
	args := []string{"goimports", "-w", "."}
	return execvRun(ctx, args, dir)
}

func derivePackages(generateReq *request.Library) ([]string, error) {
	packages := []string{}
	const exclusionPrefix = "^googleapis"
	const exclusionSuffix = "/[^/]*\\.go$"
	for _, exclusion := range generateReq.RemoveRegex {
		exclusion, hasPrefix := strings.CutPrefix(exclusion, exclusionPrefix)
		if !hasPrefix {
			return nil, fmt.Errorf("librariangen: exclusion does not have expected prefix: %s", exclusion)
		}
		exclusion, hasSuffix := strings.CutSuffix(exclusion, exclusionSuffix)
		if !hasSuffix {
			return nil, fmt.Errorf("librariangen: exclusion does not have expected suffix: %s", exclusion)
		}
		packages = append(packages, "google.golang.org/genproto/googleapis"+exclusion)
	}
	return packages, nil
}

/*
func Build(ctx context.Context, cfg *build.Config, repoConfig *config.RepoConfig, buildReq *request.Library) error {
	// TODO: implement...
	return nil
}
*/

// parseGoPkg parses the import path declared in the given file's `go_package`
// option. If the option is missing, parseGoPkg returns empty string.
func parseGoPkg(content []byte) (string, error) {
	var pkgName string
	if match := goPkgOptRe.FindSubmatch(content); len(match) > 0 {
		pn, err := strconv.Unquote(string(match[1]))
		if err != nil {
			return "", err
		}
		pkgName = pn
	}
	if p := strings.IndexRune(pkgName, ';'); p > 0 {
		pkgName = pkgName[:p]
	}
	return pkgName, nil
}

// goPkg reports the import path declared in the given file's `go_package`
// option. If the option is missing, goPkg returns empty string.
func goPkg(fileName string) (string, error) {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return "", err
	}
	return parseGoPkg(content)
}

func protosForPackages(sourceDir string, generateReq *request.Library, packages []string) ([]string, error) {
	protos := []string{}
	for _, api := range generateReq.APIs {
		protoPath := filepath.Join(sourceDir, api.Path)
		entries, err := os.ReadDir(protoPath)
		if err != nil {
			return nil, fmt.Errorf("librariangen: failed to read API source directory %s: %w", protoPath, err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".proto") {
				continue
			}
			if strings.HasSuffix(entry.Name(), "compute_small.proto") {
				continue
			}
			path := filepath.Join(protoPath, entry.Name())
			pkg, err := goPkg(path)
			if err != nil {
				return nil, err
			}
			if !slices.Contains(packages, pkg) {
				continue
			}
			protos = append(protos, path)
		}
	}
	return protos, nil
}

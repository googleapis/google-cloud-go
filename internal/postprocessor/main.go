// Copyright 2022 Google LLC
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

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/gapicgen/generator"
	"cloud.google.com/go/internal/gapicgen/git"
	"cloud.google.com/go/internal/gensnippets"
	"cloud.google.com/go/internal/postprocessor/execv"
	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

func main() {
	var srcPrefix string
	var dstPrefix string
	var scope bool
	flag.StringVar(&srcPrefix, "src", "owl-bot-staging/src/", "Path to owl-bot-staging-directory")
	flag.StringVar(&dstPrefix, "dst", ".", "Path to clients")
	flag.BoolVar(&scope, "testing", false, "Test only accessaproval client")
	flag.Parse()

	ctx := context.Background()

	log.Println("srcPrefix set to", srcPrefix)
	log.Println("dstPrefix set to", dstPrefix)

	if err := run(ctx, srcPrefix, dstPrefix, scope); err != nil {
		log.Fatal(err)
	}

	// TODO: delete owl-bot-staging file
	log.Println("End of postprocessor script.")
}

func run(ctx context.Context, srcPrefix, dstPrefix string, testing bool) error {
	log.Println("in run(). srcPrefix is", srcPrefix, ". dstPrefix is", dstPrefix)
	filepath.WalkDir(srcPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if testing && !strings.Contains(path, "accessapproval") {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		dstPath := filepath.Join(dstPrefix, strings.TrimPrefix(path, srcPrefix))
		if err := copyFiles(path, dstPath); err != nil {
			return err
		}

		return nil
	})

	// If testing, only run for accessapproval lib
	if !testing {
		if err := gocmd.ModTidyAll(dstPrefix); err != nil {
			return err
		}

		if err := gocmd.Vet(dstPrefix); err != nil {
			return err
		}
	} else {
		if err := gocmd.ModTidy(filepath.Join(dstPrefix, "accessapproval")); err != nil {
			return err
		}

		if err := gocmd.Vet(filepath.Join(dstPrefix, "accessapproval")); err != nil {
			return err
		}
	}

	if err := SnippetsGenCoordinator(ctx, dstPrefix, testing); err != nil {
		return err
	}

	return nil
}

func copyFiles(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	return nil
}

func SnippetsGenCoordinator(ctx context.Context, dstPrefix string, testing bool) error {
	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("working out %s\n", tmpDir)

	googleapisDir := filepath.Join(tmpDir, "googleapis")
	gocloudDir := dstPrefix

	// Clone repository for use in parsing API shortnames.
	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir)
	})

	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	s := SnippetConfs{googleapisDir, gocloudDir}

	if err := s.regenSnippets(testing); err != nil {
		return err
	}

	return nil
}

func ParseAPIShortnames(googleapisDir string, confs []*generator.MicrogenConfig, manualEntries []generator.ManifestEntry) (map[string]string, error) {
	shortnames := map[string]string{}
	for _, conf := range confs {
		yamlPath := filepath.Join(googleapisDir, conf.InputDirectoryPath, conf.ApiServiceConfigPath)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		config := struct {
			Name string `yaml:"name"`
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&config); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		shortname := strings.TrimSuffix(config.Name, ".googleapis.com")
		shortnames[conf.ImportPath] = shortname
	}

	// Do our best for manuals.
	for _, manual := range manualEntries {
		p := strings.TrimPrefix(manual.DistributionName, "cloud.google.com/go/")
		if strings.Contains(p, "/") {
			p = p[0:strings.Index(p, "/")]
		}
		shortnames[manual.DistributionName] = p
	}
	return shortnames, nil
}

type SnippetConfs struct {
	googleapisDir  string
	googleCloudDir string
}

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (s *SnippetConfs) regenSnippets(testing bool) error {
	log.Println("regenerating snippets")

	snippetDir := filepath.Join(s.googleCloudDir, "internal", "generated", "snippets")
	apiShortnames, err := ParseAPIShortnames(s.googleapisDir, generator.MicrogenGapicConfigs, generator.ManualEntries)

	if err != nil {
		log.Println("error in ParseAPIShortnames.")
		return err
	}
	if err := gensnippets.Generate(s.googleCloudDir, snippetDir, apiShortnames, testing); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := replaceAllForSnippets(s.googleCloudDir, snippetDir, testing); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}

	return nil
}

func replaceAllForSnippets(googleCloudDir, snippetDir string, testing bool) error {
	return execv.ForEachMod(googleCloudDir, func(dir string) error {
		if testing {
			if !strings.Contains(dir, "accessapproval") {
				return nil
			}
		}

		if dir == snippetDir {
			return nil
		}

		mod, err := gocmd.ListModName(dir)
		if err != nil {
			return err
		}

		// Replace it. Use a relative path to avoid issues on different systems.
		rel, err := filepath.Rel(snippetDir, dir)
		if err != nil {
			return err
		}
		c := execv.Command("bash", "-c", `go mod edit -replace "$MODULE=$MODULE_PATH"`)
		c.Dir = snippetDir
		c.Env = []string{
			fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
			fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
			fmt.Sprintf("MODULE=%s", mod),
			fmt.Sprintf("MODULE_PATH=%s", rel),
		}
		return c.Run()
	})
}

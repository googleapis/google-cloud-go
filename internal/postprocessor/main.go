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
	"encoding/json"
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
	body, err := collectPRBody()
	if err != nil {
		log.Fatal(err)
	}
	log.Println(body)

	var stagingDir string
	var clientRoot string
	var googleapisDir string
	var directories string
	flag.StringVar(&stagingDir, "stage-dir", "owl-bot-staging/src/", "Path to owl-bot-staging directory")
	flag.StringVar(&clientRoot, "client-root", "/repo", "Path to clients")
	flag.StringVar(&googleapisDir, "googleapis-dir", "", "Path to googleapis/googleapis repo")
	// The module names are relative to the client root - do not add paths. See README for example.
	flag.StringVar(&directories, "dirs", "", "Comma-separated list of modules to run")
	flag.Parse()

	ctx := context.Background()

	log.Println("stage-dir set to", stagingDir)
	log.Println("client-root set to", clientRoot)
	log.Println("googleapis-dir set to", googleapisDir)

	var modules []string
	if directories != "" {
		dirSlice := strings.Split(directories, ",")
		for _, dir := range dirSlice {
			modules = append(modules, filepath.Join(clientRoot, dir))
		}
	}

	log.Println("modules set to", modules)

	if googleapisDir == "" {
		log.Println("creating temp dir")
		tmpDir, err := ioutil.TempDir("", "update-postprocessor")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		log.Printf("working out %s\n", tmpDir)
		googleapisDir = filepath.Join(tmpDir, "googleapis")

		// Clone repository for use in parsing API shortnames.
		// TODO: if not cloning other repos clean up
		grp, _ := errgroup.WithContext(ctx)
		grp.Go(func() error {
			return git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir)
		})

		if err := grp.Wait(); err != nil {
			log.Fatal(err)
		}
	}

	c := &config{
		googleapisDir:  googleapisDir,
		googleCloudDir: clientRoot,
		stagingDir:     stagingDir,
		modules:        modules,
	}

	if err := c.run(ctx); err != nil {
		log.Fatal(err)
	}

	// TODO: delete owl-bot-staging file
	log.Println("End of postprocessor script.")
}

type config struct {
	googleapisDir  string
	googleCloudDir string
	stagingDir     string
	modules        []string
}

func (c *config) run(ctx context.Context) error {
	filepath.WalkDir(c.stagingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		dstPath := filepath.Join(c.googleCloudDir, strings.TrimPrefix(path, c.stagingDir))
		if err := copyFiles(path, dstPath); err != nil {
			return err
		}
		return nil
	})
	if err := gocmd.ModTidyAll(c.googleCloudDir); err != nil {
		return err
	}
	if err := gocmd.Vet(c.googleCloudDir); err != nil {
		return err
	}
	if err := c.regenSnippets(); err != nil {
		return err
	}
	if _, err := c.manifest(generator.MicrogenGapicConfigs); err != nil {
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

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (c *config) regenSnippets() error {
	log.Println("regenerating snippets")

	snippetDir := filepath.Join(c.googleCloudDir, "internal", "generated", "snippets")
	apiShortnames, err := generator.ParseAPIShortnames(c.googleapisDir, generator.MicrogenGapicConfigs, generator.ManualEntries)

	if err != nil {
		return err
	}
	if err := gensnippets.GenerateSnippetsDirs(c.googleCloudDir, snippetDir, apiShortnames, c.modules); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := c.replaceAllForSnippets(c.googleCloudDir, snippetDir); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}

	return nil
}

func (c *config) replaceAllForSnippets(googleCloudDir, snippetDir string) error {
	return execv.ForEachMod(googleCloudDir, func(dir string) error {
		if c.modules != nil {
			for _, mod := range c.modules {
				if !strings.Contains(dir, mod) {
					return nil
				}
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

// manifest writes a manifest file with info about all of the confs.
func (c *config) manifest(confs []*generator.MicrogenConfig) (map[string]generator.ManifestEntry, error) {
	log.Println("updating gapic manifest")
	entries := map[string]generator.ManifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(c.googleCloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for _, manual := range generator.ManualEntries {
		entries[manual.DistributionName] = manual
	}
	for _, conf := range confs {
		yamlPath := filepath.Join(c.googleapisDir, conf.InputDirectoryPath, conf.ApiServiceConfigPath)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		yamlConfig := struct {
			Title string `yaml:"title"` // We only need the title field.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		docURL, err := docURL(c.googleCloudDir, conf.ImportPath)
		if err != nil {
			return nil, fmt.Errorf("unable to build docs URL: %v", err)
		}
		entry := generator.ManifestEntry{
			DistributionName:  conf.ImportPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           docURL,
			ReleaseLevel:      conf.ReleaseLevel,
			LibraryType:       generator.GapicAutoLibraryType,
		}
		entries[conf.ImportPath] = entry
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return entries, enc.Encode(entries)
}

func docURL(cloudDir, importPath string) (string, error) {
	suffix := strings.TrimPrefix(importPath, "cloud.google.com/go/")
	mod, err := gocmd.CurrentMod(filepath.Join(cloudDir, suffix))
	if err != nil {
		return "", err
	}
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
	return "https://cloud.google.com/go/docs/reference/" + mod + "/latest/" + pkgPath, nil
}

// branch name assigned by OwlBot for mono repos is 'owl-bot-copy'
// (https://github.com/googleapis/repo-automation-bots/blob/57f0cabf9379ba41df0a1894f153236022ada38b/packages/owl-bot/src/copy-code.ts#L247)
// var OWL_BOT_BRANCH_NAME string = "owl-bot-copy"
var OWL_BOT_BRANCH_NAME string = "CommitMessages"

func collectPRBody() (string, error) {

	c := execv.Command("/bin/bash", "-c", `
	git checkout $BRANCH_NAME || true
	git log -1 --pretty=%B
	`)

	// c := execv.Command("git", "log", "-1", "--pretty=%B")
	out, err := c.Output()
	if err != nil {
		return "", err
	}
	outStr := string(out)
	// ss := strings.Split(outStr, "Changes:\n\n")
	// if len(ss) != 2 {
	// 	return "", fmt.Errorf("unable to process commit msg")
	// }
	// commits := strings.Split(ss[1], "\n\n")

	return outStr, nil
}

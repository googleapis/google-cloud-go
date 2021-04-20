// Copyright 2019 Google LLC
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

package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/gapicgen/execv"
	"cloud.google.com/go/internal/gapicgen/gensnippets"
	"gopkg.in/yaml.v2"
)

// GapicGenerator is used to regenerate gapic libraries.
type GapicGenerator struct {
	googleapisDir   string
	protoDir        string
	googleCloudDir  string
	genprotoDir     string
	gapicToGenerate string
}

// NewGapicGenerator creates a GapicGenerator.
func NewGapicGenerator(googleapisDir, protoDir, googleCloudDir, genprotoDir string, gapicToGenerate string) *GapicGenerator {
	return &GapicGenerator{
		googleapisDir:   googleapisDir,
		protoDir:        protoDir,
		googleCloudDir:  googleCloudDir,
		genprotoDir:     genprotoDir,
		gapicToGenerate: gapicToGenerate,
	}
}

// Regen generates gapics.
func (g *GapicGenerator) Regen(ctx context.Context) error {
	log.Println("regenerating gapics")
	for _, c := range microgenGapicConfigs {
		// Skip generation if generating all of the gapics and the associated
		// config has a block on it. Or if generating a single gapic and it does
		// not match the specified import path.
		if (c.stopGeneration && g.gapicToGenerate == "") ||
			(g.gapicToGenerate != "" && g.gapicToGenerate != c.importPath) {
			continue
		}
		if err := g.microgen(c); err != nil {
			return err
		}
	}

	if err := g.copyMicrogenFiles(); err != nil {
		return err
	}

	if err := g.manifest(microgenGapicConfigs); err != nil {
		return err
	}

	if err := g.setVersion(); err != nil {
		return err
	}

	if err := forEachMod(g.googleCloudDir, g.addModReplaceGenproto); err != nil {
		return err
	}

	snippetDir := filepath.Join(g.googleCloudDir, "internal", "generated", "snippets")
	apiShortnames, err := g.parseAPIShortnames(microgenGapicConfigs, manualEntries)
	if err != nil {
		return err
	}
	if err := gensnippets.Generate(g.googleCloudDir, snippetDir, apiShortnames); err != nil {
		return fmt.Errorf("error generating snippets: %v", err)
	}
	if err := replaceAllForSnippets(g.googleCloudDir, snippetDir); err != nil {
		return err
	}
	if err := goModTidy(snippetDir); err != nil {
		return err
	}

	if err := vet(g.googleCloudDir); err != nil {
		return err
	}

	if err := build(g.googleCloudDir); err != nil {
		return err
	}

	if err := forEachMod(g.googleCloudDir, g.dropModReplaceGenproto); err != nil {
		return err
	}

	return nil
}

// forEachMod runs the given function with the directory of
// every non-internal module.
func forEachMod(rootDir string, fn func(dir string) error) error {
	return filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(path, "internal") {
			return filepath.SkipDir
		}
		if d.Name() == "go.mod" {
			if err := fn(filepath.Dir(path)); err != nil {
				return err
			}
		}
		return nil
	})
}

func goModTidy(dir string) error {
	log.Printf("[%s] running go mod tidy", dir)
	c := execv.Command("go", "mod", "tidy")
	c.Dir = dir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

func replaceAllForSnippets(googleCloudDir, snippetDir string) error {
	return forEachMod(googleCloudDir, func(dir string) error {
		if dir == snippetDir {
			return nil
		}

		// Get the module name in this dir.
		modC := execv.Command("go", "list", "-m")
		modC.Dir = dir
		mod, err := modC.Output()
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

// addModReplaceGenproto adds a genproto replace statement that points genproto
// to the local copy. This is necessary since the remote genproto may not have
// changes that are necessary for the in-flight regen.
func (g *GapicGenerator) addModReplaceGenproto(dir string) error {
	log.Printf("[%s] adding temporary genproto replace statement", dir)
	c := execv.Command("bash", "-c", `
set -ex

go mod edit -replace "google.golang.org/genproto=$GENPROTO_DIR"
`)
	c.Dir = dir
	c.Env = []string{
		"GENPROTO_DIR=" + g.genprotoDir,
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// dropModReplaceGenproto drops the genproto replace statement. It is intended
// to be run after addModReplaceGenproto.
func (g *GapicGenerator) dropModReplaceGenproto(dir string) error {
	log.Printf("[%s] removing genproto replace statement", dir)
	c := execv.Command("bash", "-c", `
set -ex

go mod edit -dropreplace "google.golang.org/genproto"
`)
	c.Dir = dir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// setVersion updates the versionClient constant in all .go files. It may create
// .backup files on certain systems (darwin), and so should be followed by a
// clean-up of .backup files.
func (g *GapicGenerator) setVersion() error {
	log.Println("updating client version")
	// TODO(deklerk): Migrate this all to Go instead of using bash.

	c := execv.Command("bash", "-c", `
ver=$(date +%Y%m%d)
git ls-files -mo | while read modified; do
	dir=${modified%/*.*}
	find . -path "*/$dir/doc.go" -exec sed -i.backup -e "s/^const versionClient.*/const versionClient = \"$ver\"/" '{}' +;
done
find . -name '*.backup' -delete
`)
	c.Dir = g.googleCloudDir
	return c.Run()
}

// microgen runs the microgenerator on a single microgen config.
func (g *GapicGenerator) microgen(conf *microgenConfig) error {
	log.Println("microgen generating", conf.pkg)

	var protoFiles []string
	if err := filepath.Walk(g.googleapisDir+"/"+conf.inputDirectoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(info.Name(), ".proto") {
			protoFiles = append(protoFiles, path)
		}
		return nil
	}); err != nil {
		return err
	}

	args := []string{"-I", g.googleapisDir,
		"--experimental_allow_proto3_optional",
		"-I", g.protoDir,
		"--go_gapic_out", g.googleCloudDir,
		"--go_gapic_opt", fmt.Sprintf("go-gapic-package=%s;%s", conf.importPath, conf.pkg),
		"--go_gapic_opt", fmt.Sprintf("gapic-service-config=%s", conf.apiServiceConfigPath),
		"--go_gapic_opt", fmt.Sprintf("release-level=%s", conf.releaseLevel)}

	if conf.gRPCServiceConfigPath != "" {
		args = append(args, "--go_gapic_opt", fmt.Sprintf("grpc-service-config=%s", conf.gRPCServiceConfigPath))
	}
	if !conf.disableMetadata {
		args = append(args, "--go_gapic_opt", "metadata")
	}
	args = append(args, protoFiles...)
	c := execv.Command("protoc", args...)
	c.Dir = g.googleapisDir
	return c.Run()
}

// manifestEntry is used for JSON marshaling in manifest.
type manifestEntry struct {
	DistributionName  string `json:"distribution_name"`
	Description       string `json:"description"`
	Language          string `json:"language"`
	ClientLibraryType string `json:"client_library_type"`
	DocsURL           string `json:"docs_url"`
	ReleaseLevel      string `json:"release_level"`
}

// TODO: consider getting Description from the gapic, if there is one.
var manualEntries = []manifestEntry{
	// Pure manual clients.
	{
		DistributionName:  "cloud.google.com/go/bigquery",
		Description:       "BigQuery",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/bigquery",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/bigtable",
		Description:       "Cloud BigTable",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/bigtable",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/datastore",
		Description:       "Cloud Datastore",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/datastore",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/iam",
		Description:       "Cloud IAM",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/iam",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/storage",
		Description:       "Cloud Storage (GCS)",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/storage",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/rpcreplay",
		Description:       "RPC Replay",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/rpcreplay",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/profiler",
		Description:       "Cloud Profiler",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/profiler",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/compute/metadata",
		Description:       "Service Metadata API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/compute/metadata",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/functions/metadata",
		Description:       "Cloud Functions",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/functions/metadata",
		ReleaseLevel:      "alpha",
	},
	// Manuals with a GAPIC.
	{
		DistributionName:  "cloud.google.com/go/errorreporting",
		Description:       "Cloud Error Reporting API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/errorreporting",
		ReleaseLevel:      "beta",
	},
	{
		DistributionName:  "cloud.google.com/go/firestore",
		Description:       "Cloud Firestore API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/firestore",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/logging",
		Description:       "Cloud Logging API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/logging",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/pubsub",
		Description:       "Cloud PubSub",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/pubsub",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/spanner",
		Description:       "Cloud Spanner",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/spanner",
		ReleaseLevel:      "ga",
	},
}

// manifest writes a manifest file with info about all of the confs.
func (g *GapicGenerator) manifest(confs []*microgenConfig) error {
	log.Println("updating gapic manifest")
	entries := map[string]manifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(g.googleCloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	for _, manual := range manualEntries {
		entries[manual.DistributionName] = manual
	}
	for _, conf := range confs {
		yamlPath := filepath.Join(g.googleapisDir, conf.apiServiceConfigPath)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return err
		}
		yamlConfig := struct {
			Title string `yaml:"title"` // We only need the title field.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return fmt.Errorf("Decode: %v", err)
		}
		entry := manifestEntry{
			DistributionName:  conf.importPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           "https://pkg.go.dev/" + conf.importPath,
			ReleaseLevel:      conf.releaseLevel,
		}
		entries[conf.importPath] = entry
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// copyMicrogenFiles takes microgen files from gocloudDir/cloud.google.com/go
// and places them in gocloudDir.
func (g *GapicGenerator) copyMicrogenFiles() error {
	// The period at the end is analagous to * (copy everything in this dir).
	c := execv.Command("cp", "-R", g.googleCloudDir+"/cloud.google.com/go/.", ".")
	c.Dir = g.googleCloudDir
	if err := c.Run(); err != nil {
		return err
	}

	c = execv.Command("rm", "-rf", "cloud.google.com")
	c.Dir = g.googleCloudDir
	return c.Run()
}

func (g *GapicGenerator) parseAPIShortnames(confs []*microgenConfig, manualEntries []manifestEntry) (map[string]string, error) {
	shortnames := map[string]string{}
	for _, conf := range confs {
		yamlPath := filepath.Join(g.googleapisDir, conf.apiServiceConfigPath)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		config := struct {
			Name string `yaml:"name"`
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&config); err != nil {
			return nil, fmt.Errorf("Decode: %v", err)
		}
		shortname := strings.TrimSuffix(config.Name, ".googleapis.com")
		shortnames[conf.importPath] = shortname
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

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
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/internal/gapicgen/execv"
	"cloud.google.com/go/internal/gapicgen/execv/gocmd"
	"cloud.google.com/go/internal/gensnippets"
	"gopkg.in/yaml.v2"
)

var (
	//go:embed _README.md.txt
	readmeTmpl string
	//go:embed _version.go.txt
	versionTmpl string
	//go:embed _internal_version.go.txt
	internalVersionTmpl string
)

// GapicGenerator is used to regenerate gapic libraries.
type GapicGenerator struct {
	googleapisDir     string
	protoDir          string
	googleCloudDir    string
	genprotoDir       string
	gapicToGenerate   string
	regenOnly         bool
	onlyGenerateGapic bool
	genModule         bool
	modifiedPkgs      []string
	forceAll          bool
	genAlias          bool
}

// NewGapicGenerator creates a GapicGenerator.
func NewGapicGenerator(c *Config, modifiedPkgs []string) *GapicGenerator {
	return &GapicGenerator{
		googleapisDir:     c.GoogleapisDir,
		protoDir:          c.ProtoDir,
		googleCloudDir:    c.GapicDir,
		genprotoDir:       c.GenprotoDir,
		gapicToGenerate:   c.GapicToGenerate,
		regenOnly:         c.RegenOnly,
		onlyGenerateGapic: c.OnlyGenerateGapic,
		genModule:         c.GenModule,
		modifiedPkgs:      modifiedPkgs,
		forceAll:          c.ForceAll,
		genAlias:          c.GenAlias,
	}
}

type modInfo struct {
	path              string
	importPath        string
	serviceImportPath string
}

// Regen generates gapics.
func (g *GapicGenerator) Regen(ctx context.Context) error {
	log.Println("regenerating gapics")
	var newMods []modInfo
	for _, c := range MicrogenGapicConfigs {
		if !g.shouldGenerateConfig(c) {
			continue
		}

		modImportPath := filepath.Join("cloud.google.com/go", strings.Split(strings.TrimPrefix(c.ImportPath, "cloud.google.com/go/"), "/")[0])
		modPath := filepath.Join(g.googleCloudDir, modImportPath)
		if g.genModule {
			if err := generateModule(modPath, modImportPath); err != nil {
				return err
			}
			newMods = append(newMods, modInfo{
				path:              filepath.Join(g.googleCloudDir, strings.TrimPrefix(modImportPath, "cloud.google.com/go")),
				importPath:        modImportPath,
				serviceImportPath: c.ImportPath,
			})
		}
		if err := g.microgen(c); err != nil {
			return err
		}
		if err := g.genVersionFile(c); err != nil {
			return err
		}
		if g.genAlias {
			if err := g.genAliasShim(modPath); err != nil {
				return err
			}
		}
		if g.genModule {
			if err := gocmd.ModTidy(modPath); err != nil {
				return nil
			}
		}
	}

	if err := g.copyMicrogenFiles(); err != nil {
		return err
	}

	// Get rid of diffs related to bad formatting.
	if err := gocmd.Vet(g.googleCloudDir); err != nil {
		return err
	}

	if g.regenOnly {
		return nil
	}

	manifest, err := g.manifest(MicrogenGapicConfigs)
	if err != nil {
		return err
	}

	if g.genModule {
		for _, modInfo := range newMods {
			generateReadmeAndChanges(modInfo.path, modInfo.importPath, manifest[modInfo.serviceImportPath].Description)
		}
	}

	if !g.onlyGenerateGapic {
		if err := g.regenSnippets(ctx); err != nil {
			return err
		}
	}

	if !g.onlyGenerateGapic {
		if err := execv.ForEachMod(g.googleCloudDir, g.addModReplaceGenproto); err != nil {
			return err
		}
	}

	if err := gocmd.Vet(g.googleCloudDir); err != nil {
		return err
	}

	if err := gocmd.Build(g.googleCloudDir); err != nil {
		return err
	}

	if !g.onlyGenerateGapic {
		if err := execv.ForEachMod(g.googleCloudDir, g.dropModReplaceGenproto); err != nil {
			return err
		}
	}

	return nil
}

func (g *GapicGenerator) shouldGenerateConfig(c *MicrogenConfig) bool {
	if g.forceAll && !c.StopGeneration {
		return true
	}

	// Skip generation if generating all of the gapics and the associated
	// config has a block on it. Or if generating a single gapic and it does
	// not match the specified import path.
	if (c.StopGeneration && g.gapicToGenerate == "") ||
		(g.gapicToGenerate != "" && !strings.Contains(g.gapicToGenerate, c.ImportPath)) ||
		(g.forceAll && !c.StopGeneration) {
		return false
	}
	return true
}

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (g *GapicGenerator) regenSnippets(ctx context.Context) error {
	log.Println("regenerating snippets")

	snippetDir := filepath.Join(g.googleCloudDir, "internal", "generated", "snippets")
	apiShortnames, err := ParseAPIShortnames(g.googleapisDir, MicrogenGapicConfigs, ManualEntries)
	if err != nil {
		return err
	}
	if err := gensnippets.Generate(g.googleCloudDir, snippetDir, apiShortnames); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := replaceAllForSnippets(g.googleCloudDir, snippetDir); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}
	return nil
}

func replaceAllForSnippets(googleCloudDir, snippetDir string) error {
	return execv.ForEachMod(googleCloudDir, func(dir string) error {
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

// addModReplaceGenproto adds a genproto replace statement that points genproto
// to the local copy. This is necessary since the remote genproto may not have
// changes that are necessary for the in-flight regen.
func (g *GapicGenerator) addModReplaceGenproto(dir string) error {
	log.Printf("[%s] adding temporary genproto replace statement", dir)
	c := execv.Command("bash", "-c", `
set -ex

go mod edit -replace "google.golang.org/genproto=$GENPROTO_DIR"
go mod tidy
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

git restore go.mod
git restore go.sum 
`)
	c.Dir = dir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// microgen runs the microgenerator on a single microgen config.
func (g *GapicGenerator) microgen(conf *MicrogenConfig) error {
	log.Println("microgen generating", conf.Pkg)

	var protoFiles []string
	inputDir := filepath.Join(g.googleapisDir, conf.InputDirectoryPath)
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// Ignore compute_small.proto which is just for testing and would cause a collision if used in generation.
		//
		// TODO(noahdietz): Remove this when it is no longer needed.
		if strings.Contains(entry.Name(), ".proto") && !strings.Contains(entry.Name(), "compute_small.proto") {
			protoFiles = append(protoFiles, filepath.Join(inputDir, entry.Name()))
		}
	}

	args := []string{"-I", g.googleapisDir,
		"--experimental_allow_proto3_optional",
		"-I", g.protoDir,
		"--go_gapic_out", g.googleCloudDir,
		"--go_gapic_opt", fmt.Sprintf("go-gapic-package=%s;%s", conf.ImportPath, conf.Pkg),
		"--go_gapic_opt", fmt.Sprintf("api-service-config=%s", filepath.Join(conf.InputDirectoryPath, conf.ApiServiceConfigPath))}

	if conf.ReleaseLevel != "" {
		args = append(args, "--go_gapic_opt", fmt.Sprintf("release-level=%s", conf.ReleaseLevel))
	}
	if conf.GRPCServiceConfigPath != "" {
		args = append(args, "--go_gapic_opt", fmt.Sprintf("grpc-service-config=%s", filepath.Join(conf.InputDirectoryPath, conf.GRPCServiceConfigPath)))
	}
	if !conf.DisableMetadata {
		args = append(args, "--go_gapic_opt", "metadata")
	}
	if len(conf.Transports) > 0 {
		args = append(args, "--go_gapic_opt", fmt.Sprintf("transport=%s", strings.Join(conf.Transports, "+")))
	}
	if conf.NumericEnumsEnabled {
		args = append(args, "--go_gapic_opt", "rest-numeric-enums")
	}
	// This is a bummer way of toggling diregapic generation, but it compute is the only one for the near term.
	if conf.Pkg == "compute" {
		args = append(args, "--go_gapic_opt", "diregapic")
	}
	if stubsDir := conf.getStubsDir(); stubsDir != "" {
		// Enable protobuf/gRPC generation in the google-cloud-go directory.
		args = append(args, "--go_out=plugins=grpc:"+g.googleCloudDir)

		// For each file to be generated i.e. each file in the proto package,
		// override the go_package option. Applied to both the protobuf/gRPC
		// generated code, and to notify the GAPIC generator of the new
		// import path used to reference those stubs.
		stubPkgPath := filepath.Join(conf.ImportPath, stubsDir)
		for _, f := range protoFiles {
			f = strings.TrimPrefix(f, g.googleapisDir+"/")
			// Storage is a special case because it is generating a hidden beta
			// proto surface.
			if conf.ImportPath == "cloud.google.com/go/storage/internal/apiv2" {
				rerouteGoPkg := fmt.Sprintf("M%s=%s;%s", f, stubPkgPath, conf.Pkg)
				args = append(args,
					"--go_opt="+rerouteGoPkg,
					"--go_gapic_opt="+rerouteGoPkg,
				)
			} else {
				var stubPkg string
				// grafeas is a special case since protos are not at the root of
				// client definition
				if conf.InputDirectoryPath == "google/devtools/containeranalysis/v1beta1/grafeas" {
					stubPkgPath = "cloud.google.com/go/containeranalysis/apiv1beta1/grafeas/grafeaspb"
					stubPkg = "grafeaspb"
				} else {
					stubPkg = conf.Pkg + "pb"
				}
				rerouteGoPkg := fmt.Sprintf("M%s=%s;%s", f, stubPkgPath, stubPkg)
				args = append(args, "--go_opt="+rerouteGoPkg)
				if conf.isMigrated() {
					args = append(args, "--go_gapic_opt="+rerouteGoPkg)
				}
			}
		}
	}

	args = append(args, protoFiles...)
	c := execv.Command("protoc", args...)
	c.Dir = g.googleapisDir
	return c.Run()
}

func (g *GapicGenerator) genVersionFile(conf *MicrogenConfig) error {
	// These directories are not modules on purpose, don't generate a version
	// file for them.
	if conf.ImportPath == "cloud.google.com/go/longrunning/autogen" ||
		conf.ImportPath == "cloud.google.com/go/debugger/apiv2" {
		return nil
	}
	relDir := strings.TrimPrefix(conf.ImportPath, "cloud.google.com/go/")
	rootPackage := strings.Split(relDir, "/")[0]
	rootModInternal := fmt.Sprintf("cloud.google.com/go/%s/internal", rootPackage)

	f, err := os.Create(filepath.Join(g.googleCloudDir, conf.ImportPath, "version.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	t := template.Must(template.New("version").Parse(versionTmpl))
	versionData := struct {
		Year               int
		Package            string
		ModuleRootInternal string
	}{
		Year:               time.Now().Year(),
		Package:            conf.Pkg,
		ModuleRootInternal: rootModInternal,
	}
	if err := t.Execute(f, versionData); err != nil {
		return err
	}

	if g.genModule {
		os.MkdirAll(filepath.Join(g.googleCloudDir, rootModInternal), os.ModePerm)

		f2, err := os.Create(filepath.Join(g.googleCloudDir, rootModInternal, "version.go"))
		if err != nil {
			return err
		}
		defer f2.Close()

		t2 := template.Must(template.New("internal_version").Parse(internalVersionTmpl))
		internalVersionData := struct {
			Year int
		}{
			Year: time.Now().Year(),
		}
		if err := t2.Execute(f2, internalVersionData); err != nil {
			return err
		}
	}
	return nil
}

// ManifestEntry is used for JSON marshaling in manifest.
type ManifestEntry struct {
	DistributionName  string      `json:"distribution_name"`
	Description       string      `json:"description"`
	Language          string      `json:"language"`
	ClientLibraryType string      `json:"client_library_type"`
	DocsURL           string      `json:"docs_url"`
	ReleaseLevel      string      `json:"release_level"`
	LibraryType       LibraryType `json:"library_type"`
}

type LibraryType string

const (
	GapicAutoLibraryType   LibraryType = "GAPIC_AUTO"
	GapicManualLibraryType LibraryType = "GAPIC_MANUAL"
	CoreLibraryType        LibraryType = "CORE"
	AgentLibraryType       LibraryType = "AGENT"
	OtherLibraryType       LibraryType = "OTHER"
)

// TODO: consider getting Description from the gapic, if there is one.
var ManualEntries = []ManifestEntry{
	// Pure manual clients.
	{
		DistributionName:  "cloud.google.com/go/bigquery",
		Description:       "BigQuery",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/bigquery/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/bigtable",
		Description:       "Cloud BigTable",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/bigtable/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/datastore",
		Description:       "Cloud Datastore",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/datastore/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/iam",
		Description:       "Cloud IAM",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/iam/latest",
		ReleaseLevel:      "ga",
		LibraryType:       CoreLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/storage",
		Description:       "Cloud Storage (GCS)",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/storage/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/rpcreplay",
		Description:       "RPC Replay",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/latest/rpcreplay",
		ReleaseLevel:      "ga",
		LibraryType:       OtherLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/profiler",
		Description:       "Cloud Profiler",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/profiler/latest",
		ReleaseLevel:      "ga",
		LibraryType:       AgentLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/compute/metadata",
		Description:       "Service Metadata API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/compute/latest/metadata",
		ReleaseLevel:      "ga",
		LibraryType:       CoreLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/functions/metadata",
		Description:       "Cloud Functions",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/functions/latest/metadata",
		ReleaseLevel:      "alpha",
		LibraryType:       CoreLibraryType,
	},
	// Manuals with a GAPIC.
	{
		DistributionName:  "cloud.google.com/go/errorreporting",
		Description:       "Cloud Error Reporting API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/errorreporting/latest",
		ReleaseLevel:      "beta",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/firestore",
		Description:       "Cloud Firestore API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/firestore/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/logging",
		Description:       "Cloud Logging API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/logging/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/pubsub",
		Description:       "Cloud PubSub",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/pubsub/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/spanner",
		Description:       "Cloud Spanner",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/spanner/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/pubsublite",
		Description:       "Cloud PubSub Lite",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/pubsublite/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
}

// manifest writes a manifest file with info about all of the confs.
func (g *GapicGenerator) manifest(confs []*MicrogenConfig) (map[string]ManifestEntry, error) {
	log.Println("updating gapic manifest")
	entries := map[string]ManifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(g.googleCloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for _, manual := range ManualEntries {
		entries[manual.DistributionName] = manual
	}
	for _, conf := range confs {
		yamlPath := filepath.Join(g.googleapisDir, conf.InputDirectoryPath, conf.ApiServiceConfigPath)
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
		docURL, err := docURL(g.googleCloudDir, conf.ImportPath)
		if err != nil {
			return nil, fmt.Errorf("unable to build docs URL: %v", err)
		}
		entry := ManifestEntry{
			DistributionName:  conf.ImportPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           docURL,
			ReleaseLevel:      conf.ReleaseLevel,
			LibraryType:       GapicAutoLibraryType,
		}
		entries[conf.ImportPath] = entry
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return entries, enc.Encode(entries)
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

func (g *GapicGenerator) genAliasShim(modPath string) error {
	aliasshimBody := `// Copyright 2022 Google LLC
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

// Code generated by gapicgen. DO NOT EDIT.

//go:build aliasshim
// +build aliasshim

// Package aliasshim is used to keep the dependency on go-genproto during our
// go-genproto to google-cloud-go stubs migration window.
package aliasshim

import _ "google.golang.org/genproto/protobuf/api"
`
	os.MkdirAll(filepath.Join(modPath, "aliasshim"), os.ModePerm)
	f, err := os.Create(filepath.Join(modPath, "aliasshim", "aliasshim.go"))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprint(f, aliasshimBody)
	return err
}

func ParseAPIShortnames(googleapisDir string, confs []*MicrogenConfig, manualEntries []ManifestEntry) (map[string]string, error) {
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

func docURL(cloudDir, importPath string) (string, error) {
	suffix := strings.TrimPrefix(importPath, "cloud.google.com/go/")
	mod, err := gocmd.CurrentMod(filepath.Join(cloudDir, suffix))
	if err != nil {
		return "", err
	}
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
	return "https://cloud.google.com/go/docs/reference/" + mod + "/latest/" + pkgPath, nil
}

func generateModule(path, importPath string) error {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil
	}
	log.Printf("Creating %s/go.mod", path)
	return gocmd.ModInit(path, importPath)
}

func generateReadmeAndChanges(path, importPath, apiName string) error {
	readmePath := filepath.Join(path, "README.md")
	log.Printf("Creating %q", readmePath)
	readmeFile, err := os.Create(readmePath)
	if err != nil {
		return err
	}
	defer readmeFile.Close()
	t := template.Must(template.New("readme").Parse(readmeTmpl))
	readmeData := struct {
		Name       string
		ImportPath string
	}{
		Name:       apiName,
		ImportPath: importPath,
	}
	if err := t.Execute(readmeFile, readmeData); err != nil {
		return err
	}

	changesPath := filepath.Join(path, "CHANGES.md")
	log.Printf("Creating %q", changesPath)
	changesFile, err := os.Create(changesPath)
	if err != nil {
		return err
	}
	defer changesFile.Close()
	_, err = changesFile.WriteString("# Changes\n")
	return err
}

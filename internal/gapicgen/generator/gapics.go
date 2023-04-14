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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/gapicgen/execv"
	"cloud.google.com/go/internal/gapicgen/execv/gocmd"
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
	for _, c := range MicrogenGapicConfigs {
		if !g.shouldGenerateConfig(c) {
			continue
		}
		if err := g.microgen(c); err != nil {
			return err
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
	if g.forceAll && !c.StopGeneration() {
		return true
	}

	// Skip generation if generating all of the gapics and the associated
	// config has a block on it. Or if generating a single gapic and it does
	// not match the specified import path.
	if (c.StopGeneration() && g.gapicToGenerate == "") ||
		(g.gapicToGenerate != "" && !strings.Contains(g.gapicToGenerate, c.ImportPath)) ||
		(g.forceAll && !c.StopGeneration()) {
		return false
	}
	return true
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
		// TODO(chrisdsmith): Enable snippets by deleting the next line after removing internal/gapicgen/gensnippets.
		"--go_gapic_opt", "omit-snippets",
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
	if len(conf.Transports) == 0 {
		conf.Transports = []string{"grpc", "rest"}
	}
	if len(conf.Transports) > 0 {
		args = append(args, "--go_gapic_opt", fmt.Sprintf("transport=%s", strings.Join(conf.Transports, "+")))
	}
	if !conf.NumericEnumsDisabled {
		args = append(args, "--go_gapic_opt", "rest-numeric-enums")
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
				if conf.InputDirectoryPath == "google/devtools/containeranalysis/v1beta1/grafeas" {
					// grafeas is a special case since protos are not at the root of
					// client definition
					stubPkgPath = "cloud.google.com/go/containeranalysis/apiv1beta1/grafeas/grafeaspb"
					stubPkg = "grafeaspb"
				} else if conf.InputDirectoryPath == "google/firestore/admin/v1" {
					// firestore/admin is a special case since the gapic is generated
					// at a non-standard spot
					stubPkgPath = "cloud.google.com/go/firestore/apiv1/admin/adminpb"
					stubPkg = "adminpb"
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

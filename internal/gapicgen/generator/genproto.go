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
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/internal/gapicgen/execv"
	"cloud.google.com/go/internal/gapicgen/execv/gocmd"
	"cloud.google.com/go/internal/gapicgen/git"
	"golang.org/x/sync/errgroup"
)

var goPkgOptRe = regexp.MustCompile(`(?m)^option go_package = (.*);`)

// denylist is a set of clients to NOT generate.
var denylist = map[string]bool{
	// TODO(codyoss): re-enable after issue is resolve -- https://github.com/googleapis/go-genproto/issues/357
	"google.golang.org/genproto/googleapis/cloud/recommendationengine/v1beta1": true,

	// These two container APIs are currently frozen. They should not be updated
	// due to manual layer built on top of them.
	"google.golang.org/genproto/googleapis/grafeas/v1":                    true,
	"google.golang.org/genproto/googleapis/devtools/containeranalysis/v1": true,

	// Temporarily stop generation of removed protos. Will be manually cleaned
	// up with: https://github.com/googleapis/google-cloud-go/issues/4098
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1alpha2": true,

	// Not properly configured:
	"google.golang.org/genproto/googleapis/cloud/ondemandscanning/v1beta1": true,
	"google.golang.org/genproto/googleapis/cloud/ondemandscanning/v1":      true,
}

// GenprotoGenerator is used to generate code for googleapis/go-genproto.
type GenprotoGenerator struct {
	genprotoDir        string
	googleapisDir      string
	googleapisDiscoDir string
	protoSrcDir        string
	forceAll           bool
}

// NewGenprotoGenerator creates a new GenprotoGenerator.
func NewGenprotoGenerator(c *Config) *GenprotoGenerator {
	return &GenprotoGenerator{
		genprotoDir:        c.GenprotoDir,
		googleapisDir:      c.GoogleapisDir,
		googleapisDiscoDir: c.GoogleapisDiscoDir,
		protoSrcDir:        filepath.Join(c.ProtoDir, "/src"),
		forceAll:           c.ForceAll,
	}
}

var skipPrefixes = []string{
	"google.golang.org/genproto/googleapis/ads",
}

func hasPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// Regen regenerates the genproto repository.
// regenGenproto regenerates the genproto repository.
//
// regenGenproto recursively walks through each directory named by given
// arguments, looking for all .proto files. (Symlinks are not followed.) Any
// proto file without `go_package` option or whose option does not begin with
// the genproto prefix is ignored.
//
// If multiple roots contain files with the same name, eg "root1/path/to/file"
// and "root2/path/to/file", only the first file is processed; the rest are
// ignored.
//
// Protoc is executed on remaining files, one invocation per set of files
// declaring the same Go package.
func (g *GenprotoGenerator) Regen(ctx context.Context) error {
	log.Println("regenerating genproto")

	// Create space to put generated .pb.go's.
	c := execv.Command("mkdir", "-p", "generated")
	c.Dir = g.genprotoDir
	if err := c.Run(); err != nil {
		return err
	}

	// Get the last processed googleapis hash.
	lastHash, err := ioutil.ReadFile(filepath.Join(g.genprotoDir, "regen.txt"))
	if err != nil {
		return err
	}

	// TODO(noahdietz): In local mode, since it clones a shallow copy with 1 commit,
	// if the last regenerated hash is earlier than the top commit, the git diff-tree
	// command fails. This is is a bit of a rough edge. Using my local clone of
	// googleapis rectified the issue.
	pkgFiles, err := g.getUpdatedPackages(string(lastHash))
	if err != nil {
		return err
	}
	if len(pkgFiles) == 0 {
		return errors.New("couldn't find any pkgfiles")
	}

	log.Println("generating from protos")
	grp, _ := errgroup.WithContext(ctx)
	for pkg, fileNames := range pkgFiles {
		if !strings.HasPrefix(pkg, "google.golang.org/genproto") || denylist[pkg] || hasPrefix(pkg, skipPrefixes) {
			continue
		}
		pk := pkg
		fn := fileNames
		grp.Go(func() error {
			log.Println("running protoc on", pk)
			return g.protoc(fn, true /* grpc */)
		})
	}
	// TODO(noahdietz): This needs to be generalized to support any proto in googleapis-discovery.
	// It's hard because the regen.txt contains the committish from googleapis last used to regen.
	grp.Go(func() error {
		log.Println("running protoc on compute")
		return g.protoc([]string{"google/cloud/compute/v1/compute.proto"}, false /* grpc */)
	})
	if err := grp.Wait(); err != nil {
		return err
	}

	if err := g.moveAndCleanupGeneratedSrc(); err != nil {
		return err
	}

	if err := gocmd.Vet(g.genprotoDir); err != nil {
		return err
	}

	if err := gocmd.Build(g.genprotoDir); err != nil {
		return err
	}

	return nil
}

// goPkg reports the import path declared in the given file's `go_package`
// option. If the option is missing, goPkg returns empty string.
func goPkg(fileName string) (string, error) {
	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		return "", err
	}

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

// protoc executes the "protoc" command on files named in fileNames, and outputs
// to "<genprotoDir>/generated".
func (g *GenprotoGenerator) protoc(fileNames []string, grpc bool) error {
	stubs := fmt.Sprintf("--go_out=%s/generated", g.genprotoDir)
	if grpc {
		stubs = fmt.Sprintf("--go_out=plugins=grpc:%s/generated", g.genprotoDir)
	}
	args := []string{"--experimental_allow_proto3_optional", stubs, "-I", g.googleapisDiscoDir, "-I", g.googleapisDir, "-I", g.protoSrcDir}
	args = append(args, fileNames...)
	c := execv.Command("protoc", args...)
	c.Dir = g.genprotoDir
	return c.Run()
}

// getUpdatedPackages parses all of the new commits to find what packages need
// to be regenerated.
func (g *GenprotoGenerator) getUpdatedPackages(googleapisHash string) (map[string][]string, error) {
	if g.forceAll {
		return g.getAllPackages()
	}
	files, err := git.UpdateFilesSinceHash(g.googleapisDir, googleapisHash)
	if err != nil {
		return nil, err
	}
	pkgFiles := make(map[string][]string)
	for _, v := range files {
		if !strings.HasSuffix(v, ".proto") {
			continue
		}
		path := filepath.Join(g.googleapisDir, v)
		pkg, err := goPkg(path)
		if err != nil {
			return nil, err
		}
		pkgFiles[pkg] = append(pkgFiles[pkg], path)
	}
	return pkgFiles, nil
}

func (g *GenprotoGenerator) getAllPackages() (map[string][]string, error) {
	seenFiles := make(map[string]bool)
	pkgFiles := make(map[string][]string)
	for _, root := range []string{g.googleapisDir} {
		walkFn := func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || !strings.HasSuffix(path, ".proto") {
				return nil
			}

			switch rel, err := filepath.Rel(root, path); {
			case err != nil:
				return err
			case seenFiles[rel]:
				return nil
			default:
				seenFiles[rel] = true
			}

			pkg, err := goPkg(path)
			if err != nil {
				return err
			}
			pkgFiles[pkg] = append(pkgFiles[pkg], path)
			return nil
		}
		if err := filepath.Walk(root, walkFn); err != nil {
			return nil, err
		}
	}
	return pkgFiles, nil
}

// moveAndCleanupGeneratedSrc moves all generated src to their correct locations
// in the repository, because protoc puts it in a folder called `generated/``.
func (g *GenprotoGenerator) moveAndCleanupGeneratedSrc() error {
	log.Println("moving generated code")
	// The period at the end is analogous to * (copy everything in this dir).
	c := execv.Command("cp", "-R", filepath.Join(g.genprotoDir, "generated", "google.golang.org", "genproto", "googleapis"), g.genprotoDir)
	if err := c.Run(); err != nil {
		return err
	}

	c = execv.Command("rm", "-rf", "generated")
	c.Dir = g.genprotoDir
	if err := c.Run(); err != nil {
		return err
	}

	return nil
}

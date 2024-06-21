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
var ErrNoProcessing = errors.New("there are not files to regenerate")

// GenprotoGenerator is used to generate code for googleapis/go-genproto.
type GenprotoGenerator struct {
	genprotoDir   string
	googleapisDir string
	protoSrcDir   string
	forceAll      bool
}

// NewGenprotoGenerator creates a new GenprotoGenerator.
func NewGenprotoGenerator(c *Config) *GenprotoGenerator {
	return &GenprotoGenerator{
		genprotoDir:   c.GenprotoDir,
		googleapisDir: c.GoogleapisDir,
		protoSrcDir:   filepath.Join(c.ProtoDir, "/src"),
		forceAll:      c.ForceAll,
	}
}

// Only the packages listed here are generated into go-genproto.
var generateList = []string{
	"google.golang.org/genproto/googleapis/type/expr",
	"google.golang.org/genproto/googleapis/rpc/http",
	"google.golang.org/genproto/googleapis/type/latlng",
	"google.golang.org/genproto/googleapis/genomics/v1alpha2",
	"google.golang.org/genproto/googleapis/type/date",
	"google.golang.org/genproto/googleapis/type/date_time_range",
	"google.golang.org/genproto/googleapis/api/metric",
	"google.golang.org/genproto/googleapis/api/distribution",
	"google.golang.org/genproto/googleapis/chromeos/moblab/v1beta1",
	"google.golang.org/genproto/googleapis/apps/script/type/slides",
	"google.golang.org/genproto/googleapis/api/expr/v1beta1",
	"google.golang.org/genproto/googleapis/apps/script/type/gmail",
	"google.golang.org/genproto/googleapis/type/month",
	"google.golang.org/genproto/googleapis/actions/sdk/v2/interactionmodel/type",
	"google.golang.org/genproto/googleapis/apps/alertcenter/v1beta1",
	"google.golang.org/genproto/googleapis/api/error_reason",
	"google.golang.org/genproto/googleapis/assistant/embedded/v1alpha1",
	"google.golang.org/genproto/googleapis/type/localized_text",
	"google.golang.org/genproto/googleapis/type/interval",
	"google.golang.org/genproto/googleapis/watcher/v1",
	"google.golang.org/genproto/googleapis/apps/script/type/docs",
	"google.golang.org/genproto/googleapis/api/monitoredres",
	"google.golang.org/genproto/googleapis/actions/sdk/v2/interactionmodel",
	"google.golang.org/genproto/googleapis/type/dayofweek",
	"google.golang.org/genproto/googleapis/gapic/metadata",
	"google.golang.org/genproto/googleapis/chat/logging/v1",
	"google.golang.org/genproto/googleapis/api/expr/v1alpha1",
	"google.golang.org/genproto/googleapis/grafeas/v1",
	"google.golang.org/genproto/googleapis/type/quaternion",
	"google.golang.org/genproto/googleapis/type/calendarperiod",
	"google.golang.org/genproto/googleapis/type/date_range",
	"google.golang.org/genproto/googleapis/rpc/status",
	"google.golang.org/genproto/googleapis/rpc/context",
	"google.golang.org/genproto/googleapis/rpc/code",
	"google.golang.org/genproto/googleapis/api/visibility",
	"google.golang.org/genproto/googleapis/streetview/publish/v1",
	"google.golang.org/genproto/googleapis/type/money",
	"google.golang.org/genproto/googleapis/type/decimal",
	"google.golang.org/genproto/googleapis/type/color",
	"google.golang.org/genproto/googleapis/apps/drive/activity/v2",
	"google.golang.org/genproto/googleapis/apps/script/type/sheets",
	"google.golang.org/genproto/googleapis/type/timeofday",
	"google.golang.org/genproto/googleapis/home/graph/v1",
	"google.golang.org/genproto/googleapis/container/v1alpha1",
	"google.golang.org/genproto/googleapis/rpc/errdetails",
	"google.golang.org/genproto/googleapis/actions/sdk/v2",
	"google.golang.org/genproto/googleapis/networking/trafficdirector/type",
	"google.golang.org/genproto/googleapis/actions/sdk/v2/conversation",
	"google.golang.org/genproto/googleapis/home/enterprise/sdm/v1",
	"google.golang.org/genproto/googleapis/bytestream",
	"google.golang.org/genproto/googleapis/api",
	"google.golang.org/genproto/googleapis/apps/script/type",
	"google.golang.org/genproto/googleapis/api/configchange",
	"google.golang.org/genproto/googleapis/ccc/hosted/marketplace/v2",
	"google.golang.org/genproto/googleapis/chromeos/uidetection/v1",
	"google.golang.org/genproto/googleapis/type/datetime",
	"google.golang.org/genproto/googleapis/geo/type/viewport",
	"google.golang.org/genproto/googleapis/type/phone_number",
	"google.golang.org/genproto/googleapis/type/fraction",
	"google.golang.org/genproto/googleapis/apps/drive/labels/v2",
	"google.golang.org/genproto/googleapis/example/library/v1",
	"google.golang.org/genproto/googleapis/api/label",
	"google.golang.org/genproto/googleapis/bigtable/admin/v2",
	"google.golang.org/genproto/googleapis/api/httpbody",
	"google.golang.org/genproto/googleapis/partner/aistreams/v1alpha1",
	"google.golang.org/genproto/googleapis/apps/script/type/drive",
	"google.golang.org/genproto/googleapis/bigtable/v2",
	"google.golang.org/genproto/googleapis/search/partnerdataingestion/logging/v1",
	"google.golang.org/genproto/googleapis/apps/script/type/calendar",
	"google.golang.org/genproto/googleapis/rpc/context/attribute_context",
	"google.golang.org/genproto/googleapis/api/expr/conformance/v1alpha1",
	"google.golang.org/genproto/googleapis/actions/sdk/v2/interactionmodel/prompt",
	"google.golang.org/genproto/googleapis/api/serviceconfig",
	"google.golang.org/genproto/googleapis/apps/drive/labels/v2beta",
	"google.golang.org/genproto/googleapis/genomics/v1",
	"google.golang.org/genproto/googleapis/api/annotations",
	"google.golang.org/genproto/googleapis/type/postaladdress",
	"google.golang.org/genproto/googleapis/firebase/fcm/connection/v1alpha1",
	"google.golang.org/genproto/googleapis/assistant/embedded/v1alpha2",
	"google.golang.org/genproto/googleapis/datastore/v1",
	"google.golang.org/genproto/googleapis/datastore/admin/v1",
	"google.golang.org/genproto/googleapis/datastore/admin/v1beta1",
	"google.golang.org/genproto/googleapis/apps/card/v1",
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
	lastHash, err := os.ReadFile(filepath.Join(g.genprotoDir, "regen.txt"))
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
	pkgFiles, err = filterPackages(pkgFiles)
	if err != nil {
		return err
	}

	log.Println("generating from protos")
	grp, _ := errgroup.WithContext(ctx)
	for pkg, fileNames := range pkgFiles {
		pk := pkg
		fn := fileNames

		grp.Go(func() error {
			log.Println("running protoc on", pk)
			return g.protoc(fn)
		})
	}
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

func filterPackages(in map[string][]string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, allowed := range generateList {
		if files, present := in[allowed]; present {
			out[allowed] = files
		}
	}
	if len(out) == 0 {
		return nil, ErrNoProcessing
	}
	return out, nil
}

// goPkg reports the import path declared in the given file's `go_package`
// option. If the option is missing, goPkg returns empty string.
func goPkg(fileName string) (string, error) {
	content, err := os.ReadFile(fileName)
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
func (g *GenprotoGenerator) protoc(fileNames []string) error {
	args := []string{
		"--experimental_allow_proto3_optional",
		fmt.Sprintf("--go_out=plugins=grpc:%s/generated", g.genprotoDir),
		"-I", g.googleapisDir,
		"-I", g.protoSrcDir,
	}
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
		if strings.HasSuffix(v, "compute_small.proto") {
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
// in the repository, because protoc puts it in a folder called `generated/“.
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

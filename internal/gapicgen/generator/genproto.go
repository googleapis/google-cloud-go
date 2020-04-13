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

	"golang.org/x/sync/errgroup"
)

var goPkgOptRe = regexp.MustCompile(`(?m)^option go_package = (.*);`)

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
func regenGenproto(ctx context.Context, genprotoDir, googleapisDir, protoDir string) error {
	log.Println("regenerating genproto")

	// The protoc include directory is actually the "src" directory of the repo.
	protoDir += "/src"

	// Create space to put generated .pb.go's.
	c := command("mkdir", "generated")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return err
	}

	// Record and map all .proto files to their Go packages.
	seenFiles := make(map[string]bool)
	pkgFiles := make(map[string][]string)
	for _, root := range []string{googleapisDir, protoDir} {
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
			return err
		}
	}

	if len(pkgFiles) == 0 {
		return errors.New("couldn't find any pkgfiles")
	}

	// Run protoc on all protos of all packages.
	grp, _ := errgroup.WithContext(ctx)
	for pkg, fnames := range pkgFiles {
		if !strings.HasPrefix(pkg, "google.golang.org/genproto") {
			continue
		}
		pk := pkg
		fn := fnames
		grp.Go(func() error {
			log.Println("running protoc on", pk)
			return protoc(genprotoDir, googleapisDir, protoDir, fn)
		})
	}
	if err := grp.Wait(); err != nil {
		return err
	}

	// Move all generated content to their correct locations in the repository,
	// because protoc puts it in a folder called generated/.

	// The period at the end is analagous to * (copy everything in this dir).
	c = command("cp", "-R", "generated/google.golang.org/genproto/.", ".")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return err
	}

	c = command("rm", "-rf", "generated")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return err
	}

	// Throw away changes to some special libs.
	for _, lib := range []string{"googleapis/grafeas/v1", "googleapis/devtools/containeranalysis/v1"} {
		c = command("git", "checkout", lib)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Dir = genprotoDir
		if err := c.Run(); err != nil {
			return err
		}

		c = command("git", "clean", "-df", lib)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Dir = genprotoDir
		if err := c.Run(); err != nil {
			return err
		}
	}

	// Clean up and check it all compiles.
	if err := vet(genprotoDir); err != nil {
		return err
	}

	if err := build(genprotoDir); err != nil {
		return err
	}

	return nil
}

// goPkg reports the import path declared in the given file's `go_package`
// option. If the option is missing, goPkg returns empty string.
func goPkg(fname string) (string, error) {
	content, err := ioutil.ReadFile(fname)
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

// protoc executes the "protoc" command on files named in fnames, and outputs
// to "<genprotoDir>/generated".
func protoc(genprotoDir, googleapisDir, protoDir string, fnames []string) error {
	args := []string{fmt.Sprintf("--go_out=plugins=grpc:%s/generated", genprotoDir), "-I", googleapisDir, "-I", protoDir}
	args = append(args, fnames...)
	c := command("protoc", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = genprotoDir
	return c.Run()
}

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

// genlocal is a binary for generating gapics locally. It may be used to test out
// new changes, test the generation of a new library, test new generator tweaks,
// run generators against googleapis-private, and various other local tasks.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen"
	"cloud.google.com/go/internal/gapicgen/generator"
	"golang.org/x/sync/errgroup"
	"gopkg.in/src-d/go-git.v4"
)

var (
	toolsNeeded = []string{"go", "protoc"}

	usage = func() {
		fmt.Fprintln(os.Stderr, "genlocal")
		os.Exit(2)
	}
)

func main() {
	log.SetFlags(0)

	flag.Usage = usage
	flag.Parse()

	ctx := context.Background()

	if err := gapicgen.VerifyAllToolsExist(toolsNeeded); err != nil {
		log.Fatal(err)
	}

	// Create temp dirs.

	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("working out %s\n", tmpDir)

	googleapisDir := filepath.Join(tmpDir, "googleapis")
	gocloudDir := filepath.Join(tmpDir, "gocloud")
	genprotoDir := filepath.Join(tmpDir, "genproto")
	protoDir := filepath.Join(tmpDir, "proto")

	// Clone repos.

	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/googleapis.git", googleapisDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/go-genproto", genprotoDir)
	})
	grp.Go(func() error {
		return gitClone("https://code.googlesource.com/gocloud", gocloudDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/google/protobuf", protoDir)
	})
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.

	if err := generator.Generate(ctx, googleapisDir, genprotoDir, gocloudDir, protoDir); err != nil {
		log.Printf("Generator ran (and failed) in %s\n", tmpDir)
		log.Fatal(err)
	}

	// Log results.

	log.Println(genprotoDir)
	log.Println(gocloudDir)
}

// gitClone clones a repository in the given directory.
func gitClone(repo, dir string) error {
	log.Printf("cloning %s\n", repo)

	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:      repo,
		Progress: os.Stdout,
	})
	return err
}

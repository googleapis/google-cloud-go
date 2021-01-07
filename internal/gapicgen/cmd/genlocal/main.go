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

// +build !windows

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
	"strings"

	"cloud.google.com/go/internal/gapicgen"
	"cloud.google.com/go/internal/gapicgen/generator"
	"golang.org/x/sync/errgroup"
	"gopkg.in/src-d/go-git.v4"
)

var (
	toolsNeeded = []string{"go", "protoc"}
)

func main() {
	log.SetFlags(0)
	if err := gapicgen.VerifyAllToolsExist(toolsNeeded); err != nil {
		log.Fatal(err)
	}

	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("temp dir created at %s\n", tmpDir)

	googleapisDir := flag.String("googleapis-dir", filepath.Join(tmpDir, "googleapis"), "Directory where sources of googleapis/googleapis resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	gocloudDir := flag.String("gocloud-dir", filepath.Join(tmpDir, "gocloud"), "Directory where sources of googleapis/google-cloud-go resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	genprotoDir := flag.String("genproto-dir", filepath.Join(tmpDir, "genproto"), "Directory where sources of googleapis/go-genproto resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	protoDir := flag.String("proto-dir", filepath.Join(tmpDir, "proto"), "Directory where sources of google/protobuf resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	gapicToGenerate := flag.String("gapic", "", `Specifies which gapic to generate. The value should be in the form of an import path (Ex: cloud.google.com/go/pubsub/apiv1). The default "" generates all gapics.`)
	onlyGapics := flag.Bool("only-gapics", false, "Enabling stops regenerating genproto.")
	verbose := flag.Bool("verbose", false, "Enables verbose logging.")
	flag.Parse()

	ctx := context.Background()

	// Clone repositories if needed.
	grp, _ := errgroup.WithContext(ctx)
	gitClone(grp, "https://github.com/googleapis/googleapis.git", *googleapisDir, tmpDir)
	gitClone(grp, "https://github.com/googleapis/go-genproto", *genprotoDir, tmpDir)
	gitClone(grp, "https://github.com/googleapis/google-cloud-go", *gocloudDir, tmpDir)
	gitClone(grp, "https://github.com/protocolbuffers/protobuf", *protoDir, tmpDir)
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.
	conf := &generator.Config{
		GoogleapisDir:     *googleapisDir,
		GenprotoDir:       *genprotoDir,
		GapicDir:          *gocloudDir,
		ProtoDir:          *protoDir,
		GapicToGenerate:   *gapicToGenerate,
		OnlyGenerateGapic: *onlyGapics,
	}
	changes, err := generator.Generate(ctx, conf)
	if err != nil {
		log.Printf("Generator ran (and failed) in %s\n", tmpDir)
		log.Fatal(err)
	}

	// Log results.
	log.Println(genprotoDir)
	log.Println(gocloudDir)

	if *verbose {
		log.Println("Changes:")
		fmt.Println()
		for _, v := range changes {
			fmt.Println("********************************************")
			fmt.Println(v.Body)
		}
	}
}

// gitClone clones a repository in the given directory if dir is not in tmpDir.
func gitClone(eg *errgroup.Group, repo, dir, tmpDir string) {
	if !strings.HasPrefix(dir, tmpDir) {
		return
	}
	eg.Go(func() error {
		log.Printf("cloning %s\n", repo)

		_, err := git.PlainClone(dir, false, &git.CloneOptions{
			URL:      repo,
			Progress: os.Stdout,
		})
		return err
	})
}

// Copyright 2021 Google LLC
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

//go:build !windows
// +build !windows

package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen/generator"
	"golang.org/x/sync/errgroup"
	"gopkg.in/src-d/go-git.v4"
)

type localConfig struct {
	googleapisDir   string
	gocloudDir      string
	genprotoDir     string
	protoDir        string
	gapicToGenerate string
	onlyGapics      bool
	regenOnly       bool
	forceAll        bool
	genModule       bool
	genAlias        bool
}

func genLocal(ctx context.Context, c localConfig) error {
	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("temp dir created at %s\n", tmpDir)
	tmpGoogleapisDir := filepath.Join(tmpDir, "googleapis")
	tmpGenprotoDir := filepath.Join(tmpDir, "genproto")
	tmpGocloudDir := filepath.Join(tmpDir, "gocloud")
	tmpProtoDir := filepath.Join(tmpDir, "proto")

	// Clone repositories if needed.
	grp, _ := errgroup.WithContext(ctx)
	gitShallowClone(grp, "https://github.com/googleapis/googleapis.git", c.googleapisDir, tmpGoogleapisDir)
	gitShallowClone(grp, "https://github.com/googleapis/go-genproto", c.genprotoDir, tmpGenprotoDir)
	gitShallowClone(grp, "https://github.com/googleapis/google-cloud-go", c.gocloudDir, tmpGocloudDir)
	gitShallowClone(grp, "https://github.com/protocolbuffers/protobuf", c.protoDir, tmpProtoDir)
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.
	conf := &generator.Config{
		GoogleapisDir:     deafultDir(tmpGoogleapisDir, c.googleapisDir),
		GenprotoDir:       deafultDir(tmpGenprotoDir, c.genprotoDir),
		GapicDir:          deafultDir(tmpGocloudDir, c.gocloudDir),
		ProtoDir:          deafultDir(tmpProtoDir, c.protoDir),
		GapicToGenerate:   c.gapicToGenerate,
		OnlyGenerateGapic: c.onlyGapics,
		LocalMode:         true,
		RegenOnly:         c.regenOnly,
		ForceAll:          c.forceAll,
		GenModule:         c.genModule,
		GenAlias:          c.genAlias,
	}
	if _, err := generator.Generate(ctx, conf); err != nil {
		log.Printf("Generator ran (and failed) in %s\n", tmpDir)
		log.Fatal(err)
	}
	return nil
}

// gitShallowClone clones a repository into the provided tmpDir if a dir has not
// been specified.
func gitShallowClone(eg *errgroup.Group, repo, dir, tmpDir string) {
	if dir != "" {
		return
	}
	eg.Go(func() error {
		log.Printf("cloning %s\n", repo)

		_, err := git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL:      repo,
			Progress: os.Stdout,
			Depth:    1,
			Tags:     git.NoTags,
		})
		return err
	})
}

// deafultDir returns the default option if dir is not set.
func deafultDir(def, dir string) string {
	if dir == "" {
		return def
	}
	return dir
}

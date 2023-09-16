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
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen/generator"
	"github.com/go-git/go-git/v5"
	"golang.org/x/sync/errgroup"
)

type localConfig struct {
	googleapisDir string
	genprotoDir   string
	protoDir      string
	regenOnly     bool
	forceAll      bool
	genAlias      bool
}

func genLocal(ctx context.Context, c localConfig) error {
	log.Println("creating temp dir")
	tmpDir, err := os.MkdirTemp("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("temp dir created at %s\n", tmpDir)
	tmpGoogleapisDir := filepath.Join(tmpDir, "googleapis")
	tmpGenprotoDir := filepath.Join(tmpDir, "genproto")
	tmpProtoDir := filepath.Join(tmpDir, "proto")

	// Clone repositories if needed.
	grp, _ := errgroup.WithContext(ctx)
	gitShallowClone(grp, "https://github.com/googleapis/googleapis.git", c.googleapisDir, tmpGoogleapisDir)
	gitShallowClone(grp, "https://github.com/googleapis/go-genproto", c.genprotoDir, tmpGenprotoDir)
	gitShallowClone(grp, "https://github.com/protocolbuffers/protobuf", c.protoDir, tmpProtoDir)
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.
	conf := &generator.Config{
		GoogleapisDir: defaultDir(tmpGoogleapisDir, c.googleapisDir),
		GenprotoDir:   defaultDir(tmpGenprotoDir, c.genprotoDir),
		ProtoDir:      defaultDir(tmpProtoDir, c.protoDir),
		LocalMode:     true,
		RegenOnly:     c.regenOnly,
		ForceAll:      c.forceAll,
		GenAlias:      c.genAlias,
	}
	if _, err := generator.Generate(ctx, conf); err != nil {
		return err
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

// defaultDir returns the default option if dir is not set.
func defaultDir(def, dir string) string {
	if dir == "" {
		return def
	}
	return dir
}

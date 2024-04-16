// Copyright 2020 Google LLC
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
	"fmt"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen/generator"
	"cloud.google.com/go/internal/gapicgen/git"
	"golang.org/x/sync/errgroup"
)

// generate downloads sources and generates pull requests for go-genproto and
// google-cloud-go if needed.
func generate(ctx context.Context, githubClient *git.GithubClient, forceAll bool) error {
	log.Println("creating temp dir")
	tmpDir, err := os.MkdirTemp("", "update-genproto")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("working out %s\n", tmpDir)

	googleapisDir := filepath.Join(tmpDir, "googleapis")
	genprotoDir := filepath.Join(tmpDir, "genproto")
	protoDir := filepath.Join(tmpDir, "proto")

	// Clone repositories.
	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir)
	})
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/go-genproto", genprotoDir)
	})

	grp.Go(func() error {
		return git.DeepClone("https://github.com/protocolbuffers/protobuf", protoDir)
	})
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.
	conf := &generator.Config{
		GoogleapisDir: googleapisDir,
		GenprotoDir:   genprotoDir,
		ProtoDir:      protoDir,
		ForceAll:      forceAll,
	}
	changes, err := generator.Generate(ctx, conf)
	if err != nil {
		return err
	}

	// Create PR
	genprotoHasChanges, err := git.HasChanges(genprotoDir)
	if err != nil {
		return err
	}

	if !genprotoHasChanges {
		log.Println("no changes detected")
		return nil
	}
	genprotoPRNum, err := githubClient.CreateGenprotoPR(ctx, genprotoDir, false, changes)
	if err != nil {
		return fmt.Errorf("error creating PR for genproto (may need to check logs for more errors): %w", err)
	}
	log.Printf("https://github.com/googleapis/go-genproto/pull/%d", genprotoPRNum)
	return nil
}

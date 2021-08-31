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
	"io/ioutil"
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
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("working out %s\n", tmpDir)

	googleapisDir := filepath.Join(tmpDir, "googleapis")
	googleapisDiscoDir := filepath.Join(tmpDir, "googleapis-discovery")
	gocloudDir := filepath.Join(tmpDir, "gocloud")
	genprotoDir := filepath.Join(tmpDir, "genproto")
	protoDir := filepath.Join(tmpDir, "proto")

	// Clone repositories.

	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir)
	})
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/googleapis-discovery", googleapisDiscoDir)
	})
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/go-genproto", genprotoDir)
	})
	grp.Go(func() error {
		return git.DeepClone("https://github.com/googleapis/google-cloud-go", gocloudDir)
	})
	grp.Go(func() error {
		return git.DeepClone("https://github.com/protocolbuffers/protobuf", protoDir)
	})
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.
	conf := &generator.Config{
		GoogleapisDir:      googleapisDir,
		GoogleapisDiscoDir: googleapisDiscoDir,
		GenprotoDir:        genprotoDir,
		GapicDir:           gocloudDir,
		ProtoDir:           protoDir,
		ForceAll:           forceAll,
	}
	changes, err := generator.Generate(ctx, conf)
	if err != nil {
		return err
	}

	// Create PRs.
	genprotoHasChanges, err := git.HasChanges(genprotoDir)
	if err != nil {
		return err
	}

	gocloudHasChanges, err := git.HasChanges(gocloudDir)
	if err != nil {
		return err
	}

	switch {
	case genprotoHasChanges && gocloudHasChanges:
		// Both have changes.
		genprotoPRNum, err := githubClient.CreateGenprotoPR(ctx, genprotoDir, true, changes)
		if err != nil {
			return fmt.Errorf("error creating PR for genproto (may need to check logs for more errors): %v", err)
		}

		gocloudPRNum, err := githubClient.CreateGocloudPR(ctx, gocloudDir, genprotoPRNum, changes)
		if err != nil {
			return fmt.Errorf("error creating CL for veneers (may need to check logs for more errors): %v", err)
		}

		if err := githubClient.AmendGenprotoPR(ctx, genprotoPRNum, genprotoDir, gocloudPRNum, changes); err != nil {
			return fmt.Errorf("error amending genproto PR: %v", err)
		}

		genprotoPRURL := fmt.Sprintf("https://github.com/googleapis/go-genproto/pull/%d", genprotoPRNum)
		gocloudPRURL := fmt.Sprintf("https://github.com/googleapis/google-cloud-go/pull/%d", genprotoPRNum)
		log.Println(genprotoPRURL)
		log.Println(gocloudPRURL)
	case genprotoHasChanges:
		// Only genproto has changes.
		genprotoPRNum, err := githubClient.CreateGenprotoPR(ctx, genprotoDir, false, changes)
		if err != nil {
			return fmt.Errorf("error creating PR for genproto (may need to check logs for more errors): %v", err)
		}

		genprotoPRURL := fmt.Sprintf("https://github.com/googleapis/go-genproto/pull/%d", genprotoPRNum)
		log.Println(genprotoPRURL)
		log.Println("gocloud had no changes")
	case gocloudHasChanges:
		// Only gocloud has changes.
		gocloudPRNum, err := githubClient.CreateGocloudPR(ctx, gocloudDir, -1, changes)
		if err != nil {
			return fmt.Errorf("error creating CL for veneers (may need to check logs for more errors): %v", err)
		}

		gocloudPRURL := fmt.Sprintf("https://github.com/googleapis/google-cloud-go/pull/%d", gocloudPRNum)
		log.Println("genproto had no changes")
		log.Println(gocloudPRURL)
	default:
		// Neither have changes.
		log.Println("Neither genproto nor gocloud had changes")
	}
	return nil
}

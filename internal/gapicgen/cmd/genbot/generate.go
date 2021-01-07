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

// +build !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen/generator"
	"golang.org/x/sync/errgroup"
	"gopkg.in/src-d/go-git.v4"
)

// generate downloads sources and generates pull requests for go-genproto and
// google-cloud-go if needed.
func generate(ctx context.Context, githubClient *GithubClient) error {
	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("working out %s\n", tmpDir)

	googleapisDir := filepath.Join(tmpDir, "googleapis")
	gocloudDir := filepath.Join(tmpDir, "gocloud")
	genprotoDir := filepath.Join(tmpDir, "genproto")
	protoDir := filepath.Join(tmpDir, "proto")

	// Clone repositories.

	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/googleapis", googleapisDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/go-genproto", genprotoDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/google-cloud-go", gocloudDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/protocolbuffers/protobuf", protoDir)
	})
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.
	conf := &generator.Config{
		GoogleapisDir: googleapisDir,
		GenprotoDir:   genprotoDir,
		GapicDir:      gocloudDir,
		ProtoDir:      protoDir,
	}
	changes, err := generator.Generate(ctx, conf)
	if err != nil {
		return err
	}

	// Create PRs.
	genprotoHasChanges, err := hasChanges(genprotoDir)
	if err != nil {
		return err
	}

	gocloudHasChanges, err := hasChanges(gocloudDir)
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

// gitClone clones a repository in the given directory.
func gitClone(repo, dir string) error {
	log.Printf("cloning %s\n", repo)

	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:      repo,
		Progress: os.Stdout,
	})
	return err
}

// hasChanges reports whether the given directory has uncommitted git changes.
func hasChanges(dir string) (bool, error) {
	// Write command output to both os.Stderr and local, so that we can check
	// whether there are modified files.
	inmem := &bytes.Buffer{}
	w := io.MultiWriter(os.Stderr, inmem)

	c := exec.Command("bash", "-c", "git status --short")
	c.Dir = dir
	c.Stdout = w
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	err := c.Run()

	return inmem.Len() > 0, err
}

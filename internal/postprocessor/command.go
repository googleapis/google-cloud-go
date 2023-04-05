// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"log"
	"os"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv"
	"github.com/go-git/go-git/v5"
)

// filesChanged returns a list of files changed in a commit for the provdied
// hash in the given gitDir. Copied fromm google-cloud-go/gapicgen/git/git.go
func filesChanged(dir, hash string) ([]string, error) {
	out := execv.Command("git", "show", "--pretty=format:", "--name-only", hash)
	out.Dir = dir
	b, err := out.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

// runAll uses git to tell if the PR being updated should run all post
// processing logic.
func runAll(dir, branchOverride string) (bool, error) {
	if branchOverride != "" {
		// This means we are running the post processor locally and want it to
		// fully function -- so we lie.
		return true, nil
	}
	c := execv.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return false, err
	}
	branchName := strings.TrimSpace(string(b))
	return strings.HasPrefix(branchName, owlBotBranchPrefix), nil
}

// DeepClone clones a repository in the given directory.
func DeepClone(repo, dir string) error {
	log.Printf("cloning %s\n", repo)

	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:      repo,
		Progress: os.Stdout,
	})
	return err
}

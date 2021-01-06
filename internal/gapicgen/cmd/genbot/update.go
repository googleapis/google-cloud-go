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
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
)

func updateGocloudPR(ctx context.Context, githubClient *GithubClient, pr *PullRequest) error {
	if pr.Author != githubClient.Username {
		return fmt.Errorf("Pull request author %q does not match authenticated user %q", pr.Author, githubClient.Username)
	}

	// Checkout PR and update go.mod
	if err := updateGocloudGoMod(pr); err != nil {
		return err
	}

	if pr.IsDraft {
		if err := githubClient.MarkPRReadyForReview(ctx, pr.Repo, pr.NodeID); err != nil {
			return fmt.Errorf("unable to mark PR %v ready for review: %v", pr.Number, err)
		}
	}

	// Done!
	log.Printf("done updating google-cloud-go PR: %s\n", pr.URL)
	return nil
}

// updateGocloudGoMod updates the go.mod to include latest version of genproto
// for the given gocloud ref.
func updateGocloudGoMod(pr *PullRequest) error {
	tmpDir, err := ioutil.TempDir("", "finalize-gerrit-cl")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	c := exec.Command("/bin/bash", "-c", `
set -ex

git init
git remote add origin https://github.com/googleapis/google-cloud-go
git fetch --all
git checkout $BRANCH_NAME

# tidyall
go mod tidy
for i in $(find . -name go.mod); do
	pushd $(dirname $i);
		# Update genproto and api to latest for every module (latest version is
		# always correct version). tidy will remove the dependencies if they're not
		# actually used by the client.
		go get -u google.golang.org/api | true # We don't care that there's no files at root.
		go get -u google.golang.org/genproto | true # We don't care that there's no files at root.
		go mod tidy;
	popd;
done

git add -A
filesUpdated=$( git status --short | wc -l )
if [ $filesUpdated -gt 0 ];
then
    git config credential.helper store # cache creds from ~/.git-credentials
   	git commit --amend --no-edit
	git push -f origin $BRANCH_NAME
fi
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("BRANCH_NAME=%s", gocloudBranchName),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	c.Dir = tmpDir
	return c.Run()
}

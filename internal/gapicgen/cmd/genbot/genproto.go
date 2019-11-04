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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/google/go-github/github"
)

const (
	genprotoBranchName  = "regen_genproto"
	genprotoCommitTitle = "auto-regenerate .pb.go files"
	genprotoCommitBody  = `
This is an auto-generated regeneration of the .pb.go files by autogogen. Once
this PR is submitted, autotogen will update the corresponding CL at gocloud
to depend on the newer version of go-genproto, and assign reviewers. Whilst this
or any regen PR is open in go-genproto, autogogen will not create any more
regeneration PRs or CLs. If all regen PRs are closed, autogogen will create a
new set of regeneration PRs and CLs once per night.

If you have been assigned to review this CL, please:

- Ensure that CI is passin If it's failing, it requires your manual attention.
- Approve and submit this PR if you believe it's ready to ship. That will prompt
  autogogen to assign reviewers to the gocloud CL.
`
)

// genprotoReviewers is the list of github usernames that will be assigned to
// review the genproto PR.
//
// NOTE: Googler emails will not work - this list must only contain the github
// username of the reviewer.
//
// TODO(ndietz): Can we use github teams?
var genprotoReviewers = []string{"jadekler", "hongalex", "broady", "noahdietz", "tritone", "codyoss", "tbpg"}

// prGenproto creates a PR for a given genproto change.
func prGenproto(ctx context.Context, githubClient *github.Client, genprotoDir string) (prNumber int, _ error) {
	log.Println("creating genproto PR")

	c := exec.Command("/bin/bash", "-c", `
set -ex

git remote -v

eval $(ssh-agent)
ssh-add $GITHUB_KEY

git remote add regen_remote $FORK_URL
git checkout master

git branch -D $BRANCH_NAME || true
git push -d regen_remote $BRANCH_NAME || true

git add -A
git checkout -b $BRANCH_NAME
git commit -m "$COMMIT_TITLE" -m "$COMMIT_BODY"
git push regen_remote $BRANCH_NAME
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("FORK_URL=%s", fmt.Sprintf("git@github.com:%s/go-genproto.git", *githubUsername)),
		fmt.Sprintf("COMMIT_TITLE=%s", genprotoCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", genprotoCommitBody),
		fmt.Sprintf("BRANCH_NAME=%s", genprotoBranchName),
		fmt.Sprintf("GITHUB_KEY=%s", *githubSSHKeyPath),
	}
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return 0, err
	}

	head := fmt.Sprintf("%s:%s", *githubUsername, genprotoBranchName)
	base := "master"
	t := genprotoCommitTitle // Because we have to take the address.
	b := genprotoCommitBody  // Because we have to take the address.
	pr, _, err := githubClient.PullRequests.Create(ctx, "googleapis", "go-genproto", &github.NewPullRequest{
		Title: &t,
		Body:  &b,
		Head:  &head,
		Base:  &base,
	})
	if err != nil {
		return 0, err
	}

	// Can't assign the submitter of the PR as a reviewer.
	var reviewers []string
	for _, r := range genprotoReviewers {
		if r != *githubUsername {
			reviewers = append(reviewers, r)
		}
	}

	if _, _, err := githubClient.PullRequests.RequestReviewers(ctx, "googleapis", "go-genproto", pr.GetNumber(), github.ReviewersRequest{
		Reviewers: reviewers,
	}); err != nil {
		return 0, err
	}

	log.Printf("creating genproto PR... done %s\n", pr.GetHTMLURL())

	return pr.GetNumber(), nil
}

// amendPRWithCLURL amends the given genproto PR with a link to the given
// gocloud CL.
func amendPRWithCLURL(ctx context.Context, githubClient *github.Client, genprotoPRNum int, genprotoDir, gocloudCL string) error {
	newBody := fmt.Sprintf(`%s

Corresponding gocloud CL: %s
`, genprotoCommitBody, gocloudCL)

	c := exec.Command("/bin/bash", "-c", `
set -ex

eval $(ssh-agent)
ssh-add $GITHUB_KEY

git remote add amend_remote $FORK_URL
git checkout $BRANCH_NAME
git commit --amend -m "$COMMIT_TITLE" -m "$COMMIT_BODY"
git push -f amend_remote $BRANCH_NAME
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("FORK_URL=%s", fmt.Sprintf("git@github.com:%s/go-genproto.git", *githubUsername)),
		fmt.Sprintf("COMMIT_TITLE=%s", genprotoCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", newBody),
		fmt.Sprintf("BRANCH_NAME=%s", genprotoBranchName),
		fmt.Sprintf("GITHUB_KEY=%s", *githubSSHKeyPath),
	}
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return err
	}

	_, _, err := githubClient.PullRequests.Edit(ctx, "googleapis", "go-genproto", genprotoPRNum, &github.PullRequest{
		Body: &newBody,
	})
	return err
}

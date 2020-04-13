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
This is an auto-generated regeneration of the .pb.go files by
cloud.google.com/go/internal/gapicgen. Once this PR is submitted, genmgr will
update the corresponding CL at gocloud to depend on the newer version of
go-genproto, and assign reviewers. Whilst this or any regen PR is open in
go-genproto, gapicgen will not create any more regeneration PRs or CLs. If all
regen PRs are closed, gapicgen will create a new set of regeneration PRs and
CLs once per night.

If you have been assigned to review this CL, please:

- Ensure that CI is passing. If it's failing, it requires your manual attention.
- Approve and submit this PR if you believe it's ready to ship. That will prompt
  genmgr to assign reviewers to the gocloud CL.
`
)

// genprotoReviewers is the list of github usernames that will be assigned to
// review the genproto PR.
//
// NOTE: Googler emails will not work - this list must only contain the github
// username of the reviewer.
//
// TODO(ndietz): Can we use github teams?
var genprotoReviewers = []string{"hongalex", "broady", "noahdietz", "tritone", "codyoss", "tbpg"}

// prGenproto creates a PR for a given genproto change.
//
// hasCorrespondingCL indicates that there is a corresponding gocloud CL.
func prGenproto(ctx context.Context, githubClient *github.Client, genprotoDir string, hasCorrespondingCL bool) (prNumber int, _ error) {
	log.Println("creating genproto PR")

	body := genprotoCommitBody
	if !hasCorrespondingCL {
		body += "\n\nThere is no corresponding gocloud CL.\n"
	}

	c := exec.Command("/bin/bash", "-c", `
set -ex

git config credential.helper store # cache creds from ~/.git-credentials

git branch -D $BRANCH_NAME || true
git push -d origin $BRANCH_NAME || true

git add -A
git checkout -b $BRANCH_NAME
git commit -m "$COMMIT_TITLE" -m "$COMMIT_BODY"
git push origin $BRANCH_NAME
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("COMMIT_TITLE=%s", genprotoCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", body),
		fmt.Sprintf("BRANCH_NAME=%s", genprotoBranchName),
	}
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return 0, err
	}

	head := fmt.Sprintf("googleapis:" + genprotoBranchName)
	base := "master"
	t := genprotoCommitTitle // Because we have to take the address.
	pr, _, err := githubClient.PullRequests.Create(ctx, "googleapis", "go-genproto", &github.NewPullRequest{
		Title: &t,
		Body:  &body,
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
	newBody := genprotoCommitBody + fmt.Sprintf("\n\nCorresponding gocloud CL: %s\n", gocloudCL)

	c := exec.Command("/bin/bash", "-c", `
set -ex

git config credential.helper store # cache creds from ~/.git-credentials

git checkout $BRANCH_NAME
git commit --amend -m "$COMMIT_TITLE" -m "$COMMIT_BODY"
git push -f origin $BRANCH_NAME
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("COMMIT_TITLE=%s", genprotoCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", newBody),
		fmt.Sprintf("BRANCH_NAME=%s", genprotoBranchName),
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

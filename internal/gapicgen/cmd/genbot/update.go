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

	// If the PR has no reviewers, add them.
	hasReviewers, err := githubClient.HasReviewers(ctx, pr.Repo, pr.Number)
	if err != nil {
		return err
	}

	if !hasReviewers {
		if err := githubClient.AddReviewers(ctx, pr.Repo, pr.Number); err != nil {
			return err
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

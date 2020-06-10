package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const (
	gocloudBranchName  = "regen_gocloud"
	gocloudCommitTitle = "all: auto-regenerate gapics"
	gocloudCommitBody  = `
This is an auto-generated regeneration of the gapic clients by
cloud.google.com/go/internal/gapicgen. Once the corresponding gocloud PR is
submitted, genmgr will update this CL with a newer dependency to the newer
version of gocloud and assign reviewers to this CL.

If you have been assigned to review this CL, please:

- Ensure that the version of gocloud in go.mod has been updated.
- Ensure that CI is passing. If it's failing, it requires your manual attention.
- +2 and submit this CL if you believe it's ready to ship.
`

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

// githubReviewers is the list of github usernames that will be assigned to
// review the PRs.
//
// TODO(ndietz): Can we use github teams?
var githubReviewers = []string{"hongalex", "broady", "noahdietz", "tritone", "codyoss", "tbpg"}

type PullRequest struct {
	Author  string
	Title   string
	URL     string
	Created time.Time
	Number  int
	Repo    string
}

// GithubClient is a convenience wrapper around a github.Client.
type GithubClient struct {
	c *github.Client
	// Username is the GitHub username. Read-only.
	Username string
}

// NewGithubClient creates a new GithubClient.
func NewGithubClient(ctx context.Context, username, name, email, accessToken string) (*GithubClient, error) {
	if err := setGitCreds(name, email); err != nil {
		return nil, err
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return &GithubClient{c: github.NewClient(tc), Username: username}, nil
}

// SetGitCreds sets credentials for gerrit.
func setGitCreds(githubName, githubEmail string) error {
	c := exec.Command("git", "config", "--global", "user.name", githubName)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	if err := c.Run(); err != nil {
		return err
	}

	c = exec.Command("git", "config", "--global", "user.email", githubEmail)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// GetRegenPR finds the first open regen pull request for the provided repo.
func (gc *GithubClient) GetRegenPR(ctx context.Context, repo string) (*PullRequest, error) {
	log.Printf("getting %v changes", repo)

	// We don't bother paginating, because it hurts our requests quota and makes
	// the page slower without a lot of value.
	opt := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 50},
		State:       "open",
	}
	prs, _, err := gc.c.PullRequests.List(ctx, "googleapis", repo, opt)
	if err != nil {
		return nil, err
	}
	for _, pr := range prs {
		if !strings.Contains(pr.GetTitle(), "regen") {
			continue
		}
		return &PullRequest{
			Author:  pr.GetUser().GetLogin(),
			Title:   pr.GetTitle(),
			URL:     pr.GetHTMLURL(),
			Created: pr.GetCreatedAt(),
			Number:  pr.GetNumber(),
			Repo:    repo,
		}, nil
	}
	return nil, nil
}

// CreateGenprotoPR creates a PR for a given genproto change.
//
// hasCorrespondingPR indicates that there is a corresponding google-cloud-go PR.
func (gc *GithubClient) CreateGenprotoPR(ctx context.Context, genprotoDir string, hasCorrespondingPR bool) (prNumber int, _ error) {
	log.Println("creating genproto PR")

	body := genprotoCommitBody
	if !hasCorrespondingPR {
		body += "\n\nThere is no corresponding google-cloud-go PR.\n"
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
	pr, _, err := gc.c.PullRequests.Create(ctx, "googleapis", "go-genproto", &github.NewPullRequest{
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
	for _, r := range githubReviewers {
		if r != *githubUsername {
			reviewers = append(reviewers, r)
		}
	}

	if _, _, err := gc.c.PullRequests.RequestReviewers(ctx, "googleapis", "go-genproto", pr.GetNumber(), github.ReviewersRequest{
		Reviewers: reviewers,
	}); err != nil {
		return 0, err
	}

	log.Printf("creating genproto PR... done %s\n", pr.GetHTMLURL())

	return pr.GetNumber(), nil
}

// CreateGocloudPR creats a PR for a given google-cloud-go change.
func (gc *GithubClient) CreateGocloudPR(ctx context.Context, gocloudDir string, genprotoPRNum int) (prNumber int, _ error) {
	log.Println("creating google-cloud-go PR")

	var body string
	if genprotoPRNum > 0 {
		body = gocloudCommitBody + fmt.Sprintf("\n\nCorresponding genproto PR: https://github.com/googleapis/go-genproto/pull/%d\n", genprotoPRNum)
	} else {
		body = gocloudCommitBody + "\n\nThere is no corresponding genproto PR.\n"
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
		fmt.Sprintf("COMMIT_TITLE=%s", gocloudCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", body),
		fmt.Sprintf("BRANCH_NAME=%s", gocloudBranchName),
	}
	c.Dir = gocloudDir
	if err := c.Run(); err != nil {
		return 0, err
	}

	head := fmt.Sprintf("googleapis:" + gocloudBranchName)
	base := "master"
	t := gocloudCommitTitle // Because we have to take the address.
	pr, _, err := gc.c.PullRequests.Create(ctx, "googleapis", "google-cloud-go", &github.NewPullRequest{
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
	for _, r := range githubReviewers {
		if r != *githubUsername {
			reviewers = append(reviewers, r)
		}
	}

	if _, _, err := gc.c.PullRequests.RequestReviewers(ctx, "googleapis", "google-cloud-go", pr.GetNumber(), github.ReviewersRequest{
		Reviewers: reviewers,
	}); err != nil {
		return 0, err
	}

	log.Printf("creating google-cloud-go PR... done %s\n", pr.GetHTMLURL())

	return pr.GetNumber(), nil
}

// HasReviewers checks if a given pR has reviewers.
func (gc *GithubClient) HasReviewers(ctx context.Context, repo string, number int) (bool, error) {
	reviewers, _, err := gc.c.PullRequests.ListReviewers(ctx, "googleapis", repo, number, nil)
	if err != nil {
		return false, err
	}
	return len(reviewers.Users)+len(reviewers.Teams) > 0, nil
}

// AddReviewers adds reviewers to the pull request.
func (gc *GithubClient) AddReviewers(ctx context.Context, repo string, number int) error {
	_, _, err := gc.c.PullRequests.RequestReviewers(ctx, "googleapis", repo, number, github.ReviewersRequest{
		Reviewers: githubReviewers,
	})
	return err
}

// AmendWithPRURL amends the given genproto PR with a link to the given
// google-cloud-go PR.
func (gc *GithubClient) AmendWithPRURL(ctx context.Context, genprotoPRNum int, genprotoDir string, gocloudPRNum int) error {
	newBody := genprotoCommitBody + fmt.Sprintf("\n\nCorresponding google-cloud-go PR: googleapis/google-cloud-go#%d\n", gocloudPRNum)

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
	_, _, err := gc.c.PullRequests.Edit(ctx, "googleapis", "go-genproto", genprotoPRNum, &github.PullRequest{
		Body: &newBody,
	})
	return err
}

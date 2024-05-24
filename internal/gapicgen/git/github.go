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

package git

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/internal/gapicgen/execv"
	"github.com/google/go-github/v59/github"
	"golang.org/x/oauth2"
)

const (
	genprotoBranchName  = "regen_genproto"
	genprotoCommitTitle = "chore(all): auto-regenerate .pb.go files"
	genprotoCommitBody  = `
This is an auto-generated regeneration of the .pb.go files by
cloud.google.com/go/internal/gapicgen. Once this PR is submitted, genbot will
update the corresponding PR to depend on the newer version of go-genproto, and
assign reviewers. Whilst this or any regen PR is open in go-genproto, genbot
will not create any more regeneration PRs. If all regen PRs are closed,
gapicgen will create a new set of regeneration PRs once per night.

If you have been assigned to review this PR, please:

- Ensure that CI is passing. If it's failing, it requires your manual attention.
- Approve and submit this PR if you believe it's ready to ship. That will prompt
genbot to assign reviewers to the google-cloud-go PR.
`
	noPRCommitBody   = "\n\nThere is no corresponding google-cloud-go PR.\n"
	maxCommitBodyLen = 65536
)

var (
	conventionalCommitScopeRe = regexp.MustCompile(`.*\((.*)\): .*`)
	maxChangesLen             = maxCommitBodyLen - len(genprotoCommitBody) - len(noPRCommitBody)
)

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Author  string
	Title   string
	URL     string
	Created time.Time
	IsOpen  bool
	Number  int
	Repo    string
	IsDraft bool
	NodeID  string
}

// GithubClient is a convenience wrapper around Github clients.
type GithubClient struct {
	cV3 *github.Client
	// Username is the GitHub username. Read-only.
	Username string
}

// NewGithubClient creates a new GithubClient.
func NewGithubClient(ctx context.Context, username, name, email, accessToken string) (*GithubClient, error) {
	if err := setGitCreds(name, email, username, accessToken); err != nil {
		return nil, err
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return &GithubClient{cV3: github.NewClient(tc), Username: username}, nil
}

// setGitCreds configures credentials for GitHub.
func setGitCreds(githubName, githubEmail, githubUsername, accessToken string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	gitCredentials := []byte(fmt.Sprintf("https://%s:%s@github.com", githubUsername, accessToken))
	if err := os.WriteFile(path.Join(u.HomeDir, ".git-credentials"), gitCredentials, 0644); err != nil {
		return err
	}
	c := execv.Command("git", "config", "--global", "user.name", githubName)
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	if err := c.Run(); err != nil {
		return err
	}

	c = execv.Command("git", "config", "--global", "user.email", githubEmail)
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// GetRegenPR finds the first regen pull request with the given status. Accepted
// statues are: open, closed, or all.
func (gc *GithubClient) GetRegenPR(ctx context.Context, repo, status string) (*PullRequest, error) {
	return gc.GetPRWithTitle(ctx, repo, status, "auto-regenerate")
}

// GetPRWithTitle finds the first pull request with the given status and title.
// Accepted statues are: open, closed, or all.
func (gc *GithubClient) GetPRWithTitle(ctx context.Context, repo, status, title string) (*PullRequest, error) {
	log.Printf("getting %v pull requests with status %q", repo, status)

	// We don't bother paginating, because it hurts our requests quota and makes
	// the page slower without a lot of value.
	opt := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 50},
		State:       status,
	}
	prs, _, err := gc.cV3.PullRequests.List(ctx, "googleapis", repo, opt)
	if err != nil {
		return nil, err
	}
	for _, pr := range prs {
		if !strings.Contains(pr.GetTitle(), title) {
			continue
		}
		if pr.GetUser().GetLogin() != gc.Username {
			continue
		}
		return &PullRequest{
			Author:  pr.GetUser().GetLogin(),
			Title:   pr.GetTitle(),
			URL:     pr.GetHTMLURL(),
			Created: pr.GetCreatedAt().Time,
			IsOpen:  pr.GetState() == "open",
			Number:  pr.GetNumber(),
			Repo:    repo,
			IsDraft: pr.GetDraft(),
			NodeID:  pr.GetNodeID(),
		}, nil
	}
	return nil, nil
}

// CreateGenprotoPR creates a PR for a given genproto change.
//
// hasCorrespondingPR indicates that there is a corresponding google-cloud-go PR.
func (gc *GithubClient) CreateGenprotoPR(ctx context.Context, genprotoDir string, hasCorrespondingPR bool, changes []*ChangeInfo) (prNumber int, _ error) {
	log.Println("creating genproto PR")
	var sb strings.Builder
	sb.WriteString(genprotoCommitBody)
	if !hasCorrespondingPR {
		sb.WriteString(noPRCommitBody)
		sb.WriteString(FormatChanges(changes, false))
	}
	body := sb.String()

	c := execv.Command("/bin/bash", "-c", `
set -ex

git config credential.helper store # cache creds from ~/.git-credentials

git branch -D $BRANCH_NAME || true
git push -d origin $BRANCH_NAME || true

git add -A
git checkout -b $BRANCH_NAME
git commit -m "$COMMIT_TITLE" -m "$COMMIT_BODY"
git push origin $BRANCH_NAME
`)
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
	base := "main"
	t := genprotoCommitTitle // Because we have to take the address.
	pr, _, err := gc.cV3.PullRequests.Create(ctx, "googleapis", "go-genproto", &github.NewPullRequest{
		Title: &t,
		Body:  &body,
		Head:  &head,
		Base:  &base,
	})
	if err != nil {
		return 0, err
	}

	log.Printf("creating genproto PR... done %s\n", pr.GetHTMLURL())

	return pr.GetNumber(), nil
}

// parsePackage parses a package name from the conventional commit scope of a
// commit message.
func parsePackage(msg string) string {
	matches := conventionalCommitScopeRe.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]

}

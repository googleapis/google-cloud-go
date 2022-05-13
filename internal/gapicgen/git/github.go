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
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/internal/gapicgen/execv"
	"cloud.google.com/go/internal/gapicgen/execv/gocmd"
	"github.com/google/go-github/v35/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const (
	gocloudBranchName  = "regen_gocloud"
	gocloudCommitTitle = "chore(all): auto-regenerate gapics"
	gocloudCommitBody  = `
This is an auto-generated regeneration of the gapic clients by
cloud.google.com/go/internal/gapicgen. Once the corresponding genproto PR is
submitted, genbot will update this PR with a newer dependency to the newer
version of genproto and assign reviewers to this PR.

If you have been assigned to review this PR, please:

- Ensure that the version of genproto in go.mod has been updated.
- Ensure that CI is passing. If it's failing, it requires your manual attention.
- Approve and submit this PR if you believe it's ready to ship.
`

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
)

var conventionalCommitScopeRe = regexp.MustCompile(`.*\((.*)\): .*`)

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
	cV4 *githubv4.Client
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
	return &GithubClient{cV3: github.NewClient(tc), cV4: githubv4.NewClient(tc), Username: username}, nil
}

// setGitCreds configures credentials for GitHub.
func setGitCreds(githubName, githubEmail, githubUsername, accessToken string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	gitCredentials := []byte(fmt.Sprintf("https://%s:%s@github.com", githubUsername, accessToken))
	if err := ioutil.WriteFile(path.Join(u.HomeDir, ".git-credentials"), gitCredentials, 0644); err != nil {
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
func (gc *GithubClient) GetRegenPR(ctx context.Context, repo string, status string) (*PullRequest, error) {
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
		if !strings.Contains(pr.GetTitle(), "auto-regenerate") {
			continue
		}
		if pr.GetUser().GetLogin() != gc.Username {
			continue
		}
		return &PullRequest{
			Author:  pr.GetUser().GetLogin(),
			Title:   pr.GetTitle(),
			URL:     pr.GetHTMLURL(),
			Created: pr.GetCreatedAt(),
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
		sb.WriteString("\n\nThere is no corresponding google-cloud-go PR.\n")
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

// CreateGocloudPR creates a PR for a given google-cloud-go change.
func (gc *GithubClient) CreateGocloudPR(ctx context.Context, gocloudDir string, genprotoPRNum int, changes []*ChangeInfo) (prNumber int, _ error) {
	log.Println("creating google-cloud-go PR")

	var sb strings.Builder
	var draft bool
	sb.WriteString(gocloudCommitBody)
	if genprotoPRNum > 0 {
		sb.WriteString(fmt.Sprintf("\n\nCorresponding genproto PR: https://github.com/googleapis/go-genproto/pull/%d\n", genprotoPRNum))
		draft = true
	} else {
		sb.WriteString("\n\nThere is no corresponding genproto PR.\n")
	}
	sb.WriteString(FormatChanges(changes, true))
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
		fmt.Sprintf("COMMIT_TITLE=%s", gocloudCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", body),
		fmt.Sprintf("BRANCH_NAME=%s", gocloudBranchName),
	}
	c.Dir = gocloudDir
	if err := c.Run(); err != nil {
		return 0, err
	}

	t := gocloudCommitTitle // Because we have to take the address.
	pr, _, err := gc.cV3.PullRequests.Create(ctx, "googleapis", "google-cloud-go", &github.NewPullRequest{
		Title: &t,
		Body:  &body,
		Head:  github.String(fmt.Sprintf("googleapis:" + gocloudBranchName)),
		Base:  github.String("main"),
		Draft: github.Bool(draft),
	})
	if err != nil {
		return 0, err
	}

	log.Printf("creating google-cloud-go PR... done %s\n", pr.GetHTMLURL())

	return pr.GetNumber(), nil
}

// AmendGenprotoPR amends the given genproto PR with a link to the given
// google-cloud-go PR.
func (gc *GithubClient) AmendGenprotoPR(ctx context.Context, genprotoPRNum int, genprotoDir string, gocloudPRNum int, changes []*ChangeInfo) error {
	var body strings.Builder
	body.WriteString(genprotoCommitBody)
	body.WriteString(fmt.Sprintf("\n\nCorresponding google-cloud-go PR: googleapis/google-cloud-go#%d\n", gocloudPRNum))
	body.WriteString(FormatChanges(changes, false))
	sBody := body.String()
	c := execv.Command("/bin/bash", "-c", `
set -ex

git config credential.helper store # cache creds from ~/.git-credentials

git checkout $BRANCH_NAME
git commit --amend -m "$COMMIT_TITLE" -m "$COMMIT_BODY"
git push -f origin $BRANCH_NAME
`)
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("COMMIT_TITLE=%s", genprotoCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", sBody),
		fmt.Sprintf("BRANCH_NAME=%s", genprotoBranchName),
	}
	c.Dir = genprotoDir
	if err := c.Run(); err != nil {
		return err
	}
	_, _, err := gc.cV3.PullRequests.Edit(ctx, "googleapis", "go-genproto", genprotoPRNum, &github.PullRequest{
		Body: &sBody,
	})
	return err
}

// MarkPRReadyForReview switches a draft pull request to a reviewable pull
// request.
func (gc *GithubClient) MarkPRReadyForReview(ctx context.Context, repo string, nodeID string) error {
	var m struct {
		MarkPullRequestReadyForReview struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"markPullRequestReadyForReview(input: $input)"`
	}
	input := githubv4.MarkPullRequestReadyForReviewInput{
		PullRequestID: nodeID,
	}
	if err := gc.cV4.Mutate(ctx, &m, input, nil); err != nil {
		return err
	}
	return nil
}

// UpdateGocloudGoMod updates the go.mod to include latest version of genproto
// for the given gocloud ref.
func (gc *GithubClient) UpdateGocloudGoMod() error {
	tmpDir, err := ioutil.TempDir("", "finalize-github-pr")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := checkoutCode(tmpDir); err != nil {
		return err
	}
	if err := updateDeps(tmpDir); err != nil {
		return err
	}
	if err := addAndPushCode(tmpDir); err != nil {
		return err
	}

	return nil
}

func checkoutCode(tmpDir string) error {
	c := execv.Command("/bin/bash", "-c", `
set -ex

git init
git remote add origin https://github.com/googleapis/google-cloud-go
git fetch --all
git checkout $BRANCH_NAME
`)
	c.Env = []string{
		fmt.Sprintf("BRANCH_NAME=%s", gocloudBranchName),
	}
	c.Dir = tmpDir
	return c.Run()
}

func updateDeps(tmpDir string) error {
	// Find directories that had code changes.
	c := execv.Command("git", "diff", "--name-only", "HEAD", "HEAD~1")
	c.Dir = tmpDir
	out, err := c.Output()
	if err != nil {
		return err
	}
	files := strings.Split(string(out), "\n")
	dirs := map[string]bool{}
	for _, file := range files {
		if strings.HasPrefix(file, "internal") {
			continue
		}
		dir := filepath.Dir(file)
		dirs[filepath.Join(tmpDir, dir)] = true
	}

	// Find which modules had code changes.
	updatedModDirs := map[string]bool{}
	for dir := range dirs {
		modDir, err := gocmd.ListModDirName(dir)
		if err != nil {
			if errors.Is(err, gocmd.ErrBuildConstraint) {
				continue
			}
			return err
		}
		updatedModDirs[modDir] = true
	}

	// Find modules based on conventional commit messages.
	commitModDirs, err := processCommitMessage(tmpDir)
	if err != nil {
		return err
	}
	for _, v := range commitModDirs {
		updatedModDirs[v] = true
	}

	// Update required modules.
	for modDir := range updatedModDirs {
		log.Printf("Updating module dir %q", modDir)
		c := execv.Command("/bin/bash", "-c", `
set -ex

go get -d google.golang.org/api | true # We don't care that there's no files at root.
go get -d google.golang.org/genproto | true # We don't care that there's no files at root.
`)
		c.Dir = modDir
		if err := c.Run(); err != nil {
			return err
		}
	}

	// Tidy all modules
	return gocmd.ModTidyAll(tmpDir)
}

func addAndPushCode(tmpDir string) error {
	c := execv.Command("/bin/bash", "-c", `
set -ex

git add -A
filesUpdated=$( git status --short | wc -l )
if [ $filesUpdated -gt 0 ];
then
    git config credential.helper store # cache creds from ~/.git-credentials
   	git commit --amend --no-edit
	git push -f origin $BRANCH_NAME
fi
`)
	c.Env = []string{
		fmt.Sprintf("BRANCH_NAME=%s", gocloudBranchName),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	c.Dir = tmpDir
	return c.Run()
}

// processCommitMessage process the context aware commits mentioned in the PR
// body to determine what modules need to have dependency bumps.
func processCommitMessage(dir string) ([]string, error) {
	c := execv.Command("git", "log", "-1", "--pretty=%B")
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return nil, err
	}
	outStr := string(out)
	ss := strings.Split(outStr, "Changes:\n\n")
	if len(ss) != 2 {
		return nil, fmt.Errorf("unable to process commit msg")
	}
	commits := strings.Split(ss[1], "\n\n")
	var modDirs []string
	for _, v := range commits {
		pkg := parsePackage(v)
		if pkg == "" {
			continue
		}
		modDirs = append(modDirs, filepath.Join(dir, pkg))
	}
	return modDirs, nil
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

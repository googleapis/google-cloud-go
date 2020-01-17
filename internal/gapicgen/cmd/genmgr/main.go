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

// genmgr is a binary used to apply reviewers and update go.mod in a gocloud regen
// CL once the corresponding genproto PR is submitted.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"cloud.google.com/go/internal/gapicgen"
	"cloud.google.com/go/internal/gapicgen/db"
	"github.com/andygrunwald/go-gerrit"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	toolsNeeded = []string{"git", "go"}

	gocloudReviewers = []string{"codyoss@google.com", "tbp@google.com", "cbro@google.com", "hongalex@google.com", "ndietz@google.com", "cjcotter@google.com"}

	githubAccessToken = flag.String("githubAccessToken", "", "Get an access token at https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line")
	githubName        = flag.String("githubName", "", "ex -githubName=\"Jean de Klerk\"")
	githubEmail       = flag.String("githubEmail", "", "ex -githubEmail=deklerk@google.com")
	gerritCookieName  = flag.String("gerritCookieName", "o", "ex: -gerritCookieName=o")
	gerritCookieValue = flag.String("gerritCookieValue", "", "ex: -gerritCookieValue=git-your@email.com=SomeHash....")

	usage = func() {
		fmt.Fprintln(os.Stderr, `genmgr \
	-githubAccessToken=11223344556677889900aabbccddeeff11223344 \
	-gerritCookieValue=git-your@email.com=SomeHash....

-githubAccessToken
	The access token to authenticate to github. Get this at https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line.

-githubName
	The name to use in the github commit.

-githubEmail
	The email to use in the github commit.

-gerritCookieName
	The name of the cookie. Almost certainly "o" (the default).

-gerritCookieValue
	The value of the gerrit cookie. Probably looks like "git-your@email.com=SomeHash....". Get this at https://code-review.googlesource.com/settings/#HTTPCredentials > Obtain password > "git-your@email.com=SomeHash....".`)
		os.Exit(2)
	}
)

func main() {
	log.SetFlags(0)

	flag.Usage = usage
	flag.Parse()

	for k, v := range map[string]string{
		"githubAccessToken": *githubAccessToken,
		"githubName":        *githubName,
		"githubEmail":       *githubEmail,
		"gerritCookieName":  *gerritCookieName,
		"gerritCookieValue": *gerritCookieValue,
	} {
		if v == "" {
			log.Printf("missing or empty value for required flag --%s\n", k)
			usage()
		}
	}

	ctx := context.Background()

	if err := gapicgen.VerifyAllToolsExist(toolsNeeded); err != nil {
		log.Fatal(err)
	}

	// Set up clients.

	if err := gapicgen.SetGitCreds(*githubName, *githubEmail, *gerritCookieName, *gerritCookieValue); err != nil {
		log.Fatal(err)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *githubAccessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)

	gerritClient, err := gerrit.NewClient("https://code-review.googlesource.com", nil)
	if err != nil {
		log.Fatal(err)
	}
	gerritClient.Authentication.SetCookieAuth(*gerritCookieName, *gerritCookieValue)

	cache := db.New(ctx, githubClient, gerritClient)

	// Get cache.

	prs, err := cache.GetPRs(ctx)
	if err != nil {
		log.Fatal(err)
	}

	cls, err := cache.GetCLs(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// If there's an open PR: no-op! Waiting for someone to submit it.
	if pr, ok := db.FirstOpen(prs); ok {
		log.Printf("no work - there's a PR open %s (once it's submitted, we'll have work to do)\n", pr.URL())
		return
	}

	cl, ok := db.FirstOpen(cls)
	if !ok {
		log.Println("there are no open CLs - no work to do!")
		return
	}

	gerritRA, ok := cl.(*db.GerritRegenAttempt)
	if !ok {
		log.Fatalf("got %T, expected GerritRegenAttempt", cl)
	}

	// The gerrit cookie encodes username as foo.google.com instead of
	// foo@google.com. So, if the author is an email, let's strip out
	// the username part of the email and user that to check for
	// existence in the cookie.
	author := cl.Author()
	if strings.Contains(author, "@") {
		parts := strings.Split(author, "@")
		author = parts[0]
	}

	// If the CL author does not belong to the person running gapicgen,
	// we can't action on it. So: no-op.
	if !strings.Contains(*gerritCookieValue, author) {
		log.Printf("there's an open CL (%s) but it doesn't belong to the author running this program\n", cl.URL())
		return
	}

	// Update go.mod.

	ci, _, err := gerritClient.Changes.GetChange(gerritRA.ChangeID, &gerrit.ChangeOptions{
		AdditionalFields: []string{"CURRENT_REVISION"}, // Required to have the CurrentRevision field populated.
	})
	if err != nil {
		log.Fatal(err)
	}

	cr, ok := ci.Revisions[ci.CurrentRevision]
	if !ok {
		log.Fatalf("couldn't find current revision %q", ci.CurrentRevision)
	}

	if err := updateGocloudGoMod(cr.Ref); err != nil {
		log.Fatal(err)
	}

	// If the CL has no reviewers, add them.

	hasReviewers, err := hasReviewers(gerritClient, gerritRA.ChangeID)
	if err != nil {
		log.Fatal(err)
	}

	if !hasReviewers {
		if err := addGocloudReviewers(gerritClient, gerritRA.ChangeID); err != nil {
			log.Fatal(err)
		}
	}

	// Done!

	log.Printf("done updating gocloud CL (%s)!\n", cl.URL())
}

// hasReviewers checks if a given CL has reviewers.
func hasReviewers(gerritClient *gerrit.Client, changeID string) (bool, error) {
	ci, _, err := gerritClient.Changes.GetChange(changeID, &gerrit.ChangeOptions{
		AdditionalFields: []string{
			"DETAILED_LABELS",   // Required to have the Reviewers field populated.
			"DETAILED_ACCOUNTS", // Required to have Email field populated.
		},
	})
	if err != nil {
		return false, err
	}

	// We want to check for any reviewers except kokoro.
	var reviewersExcludingKokoro []string
	for _, r := range ci.Reviewers["REVIEWER"] {
		if strings.Contains(r.Email, "kokoro") {
			continue
		}
		reviewersExcludingKokoro = append(reviewersExcludingKokoro, r.Email)
	}

	return len(reviewersExcludingKokoro) > 0, nil
}

// updateGocloudGoMod updates the go.mod to include latest version of genproto
// for the given gocloud ref.
func updateGocloudGoMod(ref string) error {
	tmpDir, err := ioutil.TempDir("", "finalize-gerrit-cl")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	c := exec.Command("/bin/bash", "-c", `
set -ex

git init
git remote add origin https://code.googlesource.com/gocloud
git fetch --all
git checkout -b finalize_gerrit
git pull "https://code.googlesource.com/gocloud" $REF

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
	git-codereview mail
fi
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("REF=%s", ref),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	c.Dir = tmpDir
	return c.Run()
}

// addGocloudReviewers adds reviewers to the given gocloud CL.
func addGocloudReviewers(gerritClient *gerrit.Client, changeID string) error {
	for _, r := range gocloudReviewers {
		// Can't assign the submitter of the CL as a reviewer.
		if strings.Contains(*gerritCookieValue, r) {
			continue
		}
		_, _, err := gerritClient.Changes.AddReviewer(changeID, &gerrit.ReviewerInput{Reviewer: r})
		if err != nil {
			return err
		}
	}
	return nil
}

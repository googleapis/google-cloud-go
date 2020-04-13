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

// genbot is a binary for generating gapics and creating CLs/PRs with the results.
// It is intended to be used as a bot, though it can be run locally too.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen"
	"cloud.google.com/go/internal/gapicgen/db"
	"cloud.google.com/go/internal/gapicgen/generator"
	"github.com/andygrunwald/go-gerrit"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"gopkg.in/src-d/go-git.v4"
)

var (
	toolsNeeded = []string{"git", "go", "protoc"}

	githubAccessToken = flag.String("githubAccessToken", "", "Get an access token at https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line")
	githubUsername    = flag.String("githubUsername", "", "ex -githubUsername=jadekler")
	githubName        = flag.String("githubName", "", "ex -githubName=\"Jean de Klerk\"")
	githubEmail       = flag.String("githubEmail", "", "ex -githubEmail=deklerk@google.com")
	gerritCookieName  = flag.String("gerritCookieName", "o", "ex: -gerritCookieName=o")
	gerritCookieValue = flag.String("gerritCookieValue", "", "ex: -gerritCookieValue=git-your@email.com=SomeHash....")

	usage = func() {
		fmt.Fprintln(os.Stderr, `genbot \
	-githubAccessToken=11223344556677889900aabbccddeeff11223344 \
	-githubUsername=jadekler \
	-githubEmail=deklerk@google.com \
	-githubName="Jean de Klerk" \
	-gerritCookieValue=git-your@email.com=SomeHash....

-githubAccessToken
	The access token to authenticate to github. Get this at https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line.

-githubUsername
	The username to use in the github commit.

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
		"githubUsername":    *githubUsername,
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

	// Check if a regen is already underway.

	if pr, ok := db.FirstOpen(prs); ok {
		log.Printf("there's already a regen underway: %s", pr.URL())
		return
	}

	if cl, ok := db.FirstOpen(cls); ok {
		log.Printf("there's already a regen underway: %s", cl.URL())
		return
	}

	// Create temp dirs.

	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("working out %s\n", tmpDir)

	googleapisDir := filepath.Join(tmpDir, "googleapis")
	gocloudDir := filepath.Join(tmpDir, "gocloud")
	genprotoDir := filepath.Join(tmpDir, "genproto")
	protoDir := filepath.Join(tmpDir, "proto")

	// Clone repos.

	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/googleapis", googleapisDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/googleapis/go-genproto", genprotoDir)
	})
	grp.Go(func() error {
		return gitClone("https://code.googlesource.com/gocloud", gocloudDir)
	})
	grp.Go(func() error {
		return gitClone("https://github.com/google/protobuf", protoDir)
	})
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}

	// Regen.

	if err := generator.Generate(ctx, googleapisDir, genprotoDir, gocloudDir, protoDir); err != nil {
		log.Fatal(err)
	}

	// Create PRs/CLs.

	genprotoHasChanges, err := hasChanges(genprotoDir)
	if err != nil {
		log.Fatal(err)
	}

	gocloudHasChanges, err := hasChanges(gocloudDir)
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case genprotoHasChanges && gocloudHasChanges:
		// Both have changes.

		genprotoPRNum, err := prGenproto(ctx, githubClient, genprotoDir, true)
		if err != nil {
			log.Fatalf("error creating PR for genproto (may need to check logs for more errors): %v", err)
		}

		gocloudCL, err := clGocloud(ctx, gocloudDir, genprotoPRNum)
		if err != nil {
			log.Fatalf("error creating CL for veneers (may need to check logs for more errors): %v", err)
		}

		if err := amendPRWithCLURL(ctx, githubClient, genprotoPRNum, genprotoDir, gocloudCL); err != nil {
			log.Fatalf("error amending genproto PR: %v", err)
		}

		genprotoPRURL := fmt.Sprintf("https://github.com/googleapis/go-genproto/pull/%d", genprotoPRNum)
		log.Println(genprotoPRURL)
		log.Println(gocloudCL)
	case genprotoHasChanges:
		// Only genproto has changes.

		genprotoPRNum, err := prGenproto(ctx, githubClient, genprotoDir, false)
		if err != nil {
			log.Fatalf("error creating PR for genproto (may need to check logs for more errors): %v", err)
		}

		genprotoPRURL := fmt.Sprintf("https://github.com/googleapis/go-genproto/pull/%d", genprotoPRNum)
		log.Println(genprotoPRURL)
		log.Println("gocloud had no changes")
	case gocloudHasChanges:
		// Only gocloud has changes.

		gocloudCL, err := clGocloud(ctx, gocloudDir, -1)
		if err != nil {
			log.Fatalf("error creating CL for veneers (may need to check logs for more errors): %v", err)
		}

		log.Println("genproto had no changes")
		log.Println(gocloudCL)
	default:
		// Neither have changes.

		log.Println("Neither genproto nor gocloud had changes")
	}
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
	inmem := bytes.NewBuffer([]byte{}) // TODO(deklerk): Try `var inmem bytes.Buffer`.
	w := io.MultiWriter(os.Stderr, inmem)

	c := exec.Command("bash", "-c", "git status --short")
	c.Dir = dir
	c.Stdout = w
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	err := c.Run()

	return inmem.Len() > 0, err
}

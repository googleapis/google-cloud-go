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

// +build !windows

// genbot is a binary for generating gapics and creating CLs/PRs with the results.
// It is intended to be used as a bot, though it can be run locally too.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/internal/gapicgen"
)

var (
	toolsNeeded = []string{"git", "go", "protoc"}

	githubAccessToken = flag.String("githubAccessToken", "", "Get an access token at https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line")
	githubUsername    = flag.String("githubUsername", "", "ex -githubUsername=jadekler")
	githubName        = flag.String("githubName", "", "ex -githubName=\"Jean de Klerk\"")
	githubEmail       = flag.String("githubEmail", "", "ex -githubEmail=deklerk@google.com")

	usage = func() {
		fmt.Fprintln(os.Stderr, `genbot \
	-githubAccessToken=11223344556677889900aabbccddeeff11223344 \
	-githubUsername=jadekler \
	-githubEmail=deklerk@google.com \
	-githubName="Jean de Klerk" \

-githubAccessToken
	The access token to authenticate to github. Get this at https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line.

-githubUsername
	The username to use in the github commit.

-githubName
	The name to use in the github commit.

-githubEmail
	The email to use in the github commit.`)
		os.Exit(2)
	}
)

type prStatus uint8

func (ps prStatus) Has(status prStatus) bool { return ps&status != 0 }

const (
	noOpenPRs prStatus = 1 << iota
	openGenprotoPR
	openGocloudPR
)

func main() {
	log.SetFlags(0)

	flag.Usage = usage
	flag.Parse()
	if *githubAccessToken == "" {
		*githubAccessToken = os.Getenv("GITHUB_ACCESS_TOKEN")
	}
	if *githubUsername == "" {
		*githubUsername = os.Getenv("GITHUB_USERNAME")
	}
	if *githubName == "" {
		*githubName = os.Getenv("GITHUB_NAME")
	}
	if *githubEmail == "" {
		*githubEmail = os.Getenv("GITHUB_EMAIL")
	}

	for k, v := range map[string]string{
		"githubAccessToken": *githubAccessToken,
		"githubUsername":    *githubUsername,
		"githubName":        *githubName,
		"githubEmail":       *githubEmail,
	} {
		if v == "" {
			log.Printf("missing or empty value for required flag --%s\n", k)
			usage()
		}
	}

	ctx := context.Background()

	// Setup the client and git environment.
	if err := gapicgen.VerifyAllToolsExist(toolsNeeded); err != nil {
		log.Fatal(err)
	}
	githubClient, err := NewGithubClient(ctx, *githubUsername, *githubName, *githubEmail, *githubAccessToken)
	if err != nil {
		log.Fatal(err)
	}

	// Check current regen status.
	if pr, err := githubClient.GetRegenPR(ctx, "go-genproto", "open"); err != nil {
		log.Fatal(err)
	} else if pr != nil {
		log.Println("There is already a re-generation in progress")
		return
	}
	if pr, err := githubClient.GetRegenPR(ctx, "google-cloud-go", "open"); err != nil {
		log.Fatal(err)
	} else if pr != nil {
		if err := updateGocloudPR(ctx, githubClient, pr); err != nil {
			log.Fatal(err)
		}
		return
	}
	log.Println("checking if a pull request was already opened and merged today")
	if pr, err := githubClient.GetRegenPR(ctx, "go-genproto", "closed"); err != nil {
		log.Fatal(err)
	} else if pr != nil && hasCreatedPRToday(pr.Created) {
		log.Println("skipping generation, already created and merged a go-genproto PR today")
		return
	}
	if pr, err := githubClient.GetRegenPR(ctx, "google-cloud-go", "closed"); err != nil {
		log.Fatal(err)
	} else if pr != nil && hasCreatedPRToday(pr.Created) {
		log.Println("skipping generation, already created and merged a google-cloud-go PR today")
		return
	}

	if err := generate(ctx, githubClient); err != nil {
		log.Fatal(err)
	}
}

// hasCreatedPRToday checks if the created time of a PR is from today.
func hasCreatedPRToday(created time.Time) bool {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	log.Printf("Times -- Now: %v\tToday: %v\tPR Created: %v", now, today, created)
	return created.After(today)
}

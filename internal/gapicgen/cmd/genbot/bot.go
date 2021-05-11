// Copyright 2021 Google LLC
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
	"flag"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/internal/gapicgen/git"
)

func genBot(ctx context.Context, githubAccessToken, githubUsername, githubName, githubEmail string) error {
	for k, v := range map[string]string{
		"githubAccessToken": githubAccessToken,
		"githubUsername":    githubUsername,
		"githubName":        githubName,
		"githubEmail":       githubEmail,
	} {
		if v == "" {
			log.Printf("missing or empty value for required flag --%s\n", k)
			flag.PrintDefaults()
		}
	}

	// Setup the client and git environment.
	githubClient, err := git.NewGithubClient(ctx, githubUsername, githubName, githubEmail, githubAccessToken)
	if err != nil {
		return err
	}

	// Check current regen status.
	if pr, err := githubClient.GetRegenPR(ctx, "go-genproto", "open"); err != nil {
		return err
	} else if pr != nil {
		return fmt.Errorf("there is already a re-generation in progress")
	}
	if pr, err := githubClient.GetRegenPR(ctx, "google-cloud-go", "open"); err != nil {
		return err
	} else if pr != nil {
		if err := updateGocloudPR(ctx, githubClient, pr); err != nil {
			return err
		}
		return nil
	}
	log.Println("checking if a pull request was already opened and merged today")
	if pr, err := githubClient.GetRegenPR(ctx, "go-genproto", "closed"); err != nil {
		return err
	} else if pr != nil && hasCreatedPRToday(pr.Created) {
		log.Println("skipping generation, already created and merged a go-genproto PR today")
		return nil
	}
	if pr, err := githubClient.GetRegenPR(ctx, "google-cloud-go", "closed"); err != nil {
		return err
	} else if pr != nil && hasCreatedPRToday(pr.Created) {
		log.Println("skipping generation, already created and merged a google-cloud-go PR today")
		return nil
	}

	return generate(ctx, githubClient)
}

// hasCreatedPRToday checks if the created time of a PR is from today.
func hasCreatedPRToday(created time.Time) bool {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	log.Printf("Times -- Now: %v\tToday: %v\tPR Created: %v", now, today, created)
	return created.After(today)
}

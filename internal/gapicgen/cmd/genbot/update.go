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

//go:build !windows
// +build !windows

package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/internal/gapicgen/git"
)

func updateGocloudPR(ctx context.Context, githubClient *git.GithubClient, pr *git.PullRequest) error {
	if pr.Author != githubClient.Username {
		return fmt.Errorf("pull request author %q does not match authenticated user %q", pr.Author, githubClient.Username)
	}

	// Checkout PR and update go.mod
	if err := githubClient.UpdateGocloudGoMod(); err != nil {
		return err
	}

	if pr.IsDraft {
		if err := githubClient.MarkPRReadyForReview(ctx, pr.Repo, pr.NodeID); err != nil {
			return fmt.Errorf("unable to mark PR %v ready for review: %v", pr.Number, err)
		}
	}

	// Done!
	log.Printf("done updating google-cloud-go PR: %s\n", pr.URL)
	return nil
}

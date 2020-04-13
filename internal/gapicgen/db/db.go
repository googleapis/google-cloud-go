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

// Package db is responsible for getting information about CLs and PRs in Gerrit
// and GitHub respectively.
package db

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/google/go-github/github"
)

// RegenAttempt represents either a genproto regen PR or a gocloud gapic regen
// CL.
type RegenAttempt interface {
	Author() string
	Title() string
	URL() string
	Created() time.Time
	Open() bool
}

// ByCreated allows RegenAttempt to be sorted by Created field.
type ByCreated []RegenAttempt

func (a ByCreated) Len() int           { return len(a) }
func (a ByCreated) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByCreated) Less(i, j int) bool { return a[i].Created().After(a[j].Created()) }

// regenAttempt represents either a genproto regen PR or a gocloud gapic regen
// CL.
type regenAttempt struct {
	author  string
	title   string
	url     string
	created time.Time
	open    bool
}

func (ra *regenAttempt) Author() string     { return ra.author }
func (ra *regenAttempt) Title() string      { return ra.title }
func (ra *regenAttempt) URL() string        { return ra.url }
func (ra *regenAttempt) Created() time.Time { return ra.created }
func (ra *regenAttempt) Open() bool         { return ra.open }

// GerritRegenAttempt is a gerrit regen attempt (a CL).
type GerritRegenAttempt struct {
	regenAttempt
	ChangeID string
}

// GenprotoRegenAttempt is a genproto regen attempt (a PR).
type GenprotoRegenAttempt struct {
	regenAttempt
}

// Db can communicate with GitHub and Gerrit to get PRs / CLs.
type Db struct {
	gerritClient *gerrit.Client
	githubClient *github.Client

	cacheMu sync.Mutex
	// For some reason, the Changes API only returns AccountID. So we cache the
	// accountID->name to improve performance / reduce adtl calls.
	cachedGerritAccounts map[int]string // accountid -> name
}

// New returns a new Db.
func New(ctx context.Context, githubClient *github.Client, gerritClient *gerrit.Client) *Db {
	db := &Db{
		githubClient: githubClient,
		gerritClient: gerritClient,

		cacheMu:              sync.Mutex{},
		cachedGerritAccounts: map[int]string{},
	}

	return db
}

// GetPRs fetches regen PRs from genproto.
func (db *Db) GetPRs(ctx context.Context) ([]RegenAttempt, error) {
	log.Println("getting genproto changes")
	genprotoPRs := []RegenAttempt{}

	// We don't bother paginating, because it hurts our requests quota and makes
	// the page slower without a lot of value.
	opt := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 50},
		State:       "all",
	}
	prs, _, err := db.githubClient.PullRequests.List(ctx, "googleapis", "go-genproto", opt)
	if err != nil {
		return nil, err
	}
	for _, pr := range prs {
		if !strings.Contains(pr.GetTitle(), "regen") {
			continue
		}
		genprotoPRs = append(genprotoPRs, &GenprotoRegenAttempt{
			regenAttempt: regenAttempt{
				author:  pr.GetUser().GetLogin(),
				title:   pr.GetTitle(),
				url:     pr.GetHTMLURL(),
				created: pr.GetCreatedAt(),
				open:    pr.GetState() == "open",
			},
		})
	}
	return genprotoPRs, nil
}

// GetCLs fetches regen CLs from Gerrit.
func (db *Db) GetCLs(ctx context.Context) ([]RegenAttempt, error) {
	log.Println("getting gocloud changes")
	gocloudCLs := []RegenAttempt{}

	changes, _, err := db.gerritClient.Changes.QueryChanges(&gerrit.QueryChangeOptions{
		QueryOptions: gerrit.QueryOptions{Query: []string{"project:gocloud"}, Limit: 200},
	})
	if err != nil {
		return nil, err
	}

	for _, c := range *changes {
		if !strings.Contains(c.Subject, "regen") {
			continue
		}

		// For some reason, the Changes API only returns AccountID. So now we
		// have to go get the name.
		db.cacheMu.Lock()
		if _, ok := db.cachedGerritAccounts[c.Owner.AccountID]; !ok {
			log.Println("looking up user", c.Owner.AccountID)
			ai, resp, err := db.gerritClient.Accounts.GetAccount(strconv.Itoa(c.Owner.AccountID))
			if err != nil {
				if resp.StatusCode == http.StatusNotFound {
					db.cachedGerritAccounts[c.Owner.AccountID] = fmt.Sprintf("unknown user account ID: %d\n", c.Owner.AccountID)
				} else {
					db.cacheMu.Unlock()
					return nil, err
				}
			} else {
				db.cachedGerritAccounts[c.Owner.AccountID] = ai.Email
			}
		}

		gocloudCLs = append(gocloudCLs, &GerritRegenAttempt{
			regenAttempt: regenAttempt{
				author:  db.cachedGerritAccounts[c.Owner.AccountID],
				title:   c.Subject,
				url:     fmt.Sprintf("https://code-review.googlesource.com/q/%s", c.ChangeID),
				created: c.Created.Time,
				open:    c.Status == "NEW",
			},
			ChangeID: c.ChangeID,
		})
		db.cacheMu.Unlock()
	}

	return gocloudCLs, nil
}

// FirstOpen returns the first open regen attempt.
func FirstOpen(ras []RegenAttempt) (RegenAttempt, bool) {
	for _, ra := range ras {
		if ra.Open() {
			return ra, true
		}
	}
	return nil, false
}

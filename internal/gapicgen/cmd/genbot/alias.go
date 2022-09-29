// Copyright 2022 Google LLC
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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/internal/aliasfix"
	"cloud.google.com/go/internal/aliasgen"
	"cloud.google.com/go/internal/gapicgen/execv/gocmd"
	"cloud.google.com/go/internal/gapicgen/git"
	"golang.org/x/sync/errgroup"
)

type aliasConfig struct {
	githubAccessToken string
	githubUsername    string
	githubName        string
	githubEmail       string
	gocloudDir        string
	genprotoDir       string
}

func genAliasMode(ctx context.Context, c aliasConfig) error {
	for k, v := range map[string]string{
		"githubAccessToken": c.githubAccessToken,
		"githubUsername":    c.githubUsername,
		"githubName":        c.githubName,
		"githubEmail":       c.githubEmail,
	} {
		if v == "" {
			log.Printf("missing or empty value for required flag --%s\n", k)
			flag.PrintDefaults()
		}
	}

	// Setup the client and git environment.
	githubClient, err := git.NewGithubClient(ctx, c.githubUsername, c.githubName, c.githubEmail, c.githubAccessToken)
	if err != nil {
		return err
	}

	// Check current alias regen status.
	if pr, err := githubClient.GetPRWithTitle(ctx, "go-genproto", "", "auto-regenerate alias files"); err != nil {
		return err
	} else if pr != nil && pr.IsOpen {
		log.Println("there is already an alias re-generation in progress")
		return nil
	} else if pr != nil && !pr.IsOpen {
		lastWeek := time.Now().Add(-7 * 24 * time.Hour)
		if pr.Created.After(lastWeek) {
			log.Println("there is already an alias re-generation already this week")
			return nil
		}
	}

	// Clone repositories if needed.
	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("temp dir created at %s\n", tmpDir)
	grp, _ := errgroup.WithContext(ctx)
	grp.Go(func() error {
		if c.genprotoDir != "" {
			return nil
		}
		return git.DeepClone("https://github.com/googleapis/go-genproto", filepath.Join(tmpDir, "genproto"))
	})
	grp.Go(func() error {
		if c.gocloudDir != "" {
			return nil
		}
		return git.DeepClone("https://github.com/googleapis/google-cloud-go", filepath.Join(tmpDir, "gocloud"))
	})
	if err := grp.Wait(); err != nil {
		return err
	}
	genprotoDir := defaultDir(filepath.Join(tmpDir, "genproto"), c.genprotoDir)
	gocloudDir := defaultDir(filepath.Join(tmpDir, "gocloud"), c.gocloudDir)

	// Generate aliases
	if err := generateAliases(ctx, gocloudDir, genprotoDir); err != nil {
		return err
	}

	// Create PR if needed
	genprotoHasChanges, err := git.HasChanges(genprotoDir)
	if err != nil {
		return err
	}
	if !genprotoHasChanges {
		log.Println("no files needed updating this week")
		return nil
	}
	if err := githubClient.CreateAliasPR(ctx, genprotoDir); err != nil {
		return err
	}

	return nil
}

func generateAliases(ctx context.Context, gocloudDir, genprotoDir string) error {
	for k, v := range aliasfix.GenprotoPkgMigration {
		if v.Status != aliasfix.StatusMigrated {
			continue
		}
		log.Printf("Generating aliases for: %s", k)
		gapicSrcDir := filepath.Join(gocloudDir, strings.TrimPrefix(v.ImportPath, "cloud.google.com/go/"))
		genprotoStubsDir := filepath.Join(genprotoDir, strings.TrimPrefix(k, "google.golang.org/genproto/"))

		// Find out what is the latest version of cloud client library
		module, version, err := gocmd.ListModVersion(gapicSrcDir)
		if err != nil {
			return err
		}

		// checkout said version
		if err := checkoutRef(gocloudDir, module, version); err != nil {
			return err
		}
		// Try to upgrade dependency to said version
		if err := gocmd.Get(genprotoDir, module, version); err != nil {
			return err
		}

		if err := aliasgen.Run(gapicSrcDir, genprotoStubsDir); err != nil {
			return err
		}

		// checkout HEAD of cloud client library
		if err := checkoutRef(gocloudDir, "", ""); err != nil {
			return err
		}
	}
	return nil
}

// checkoutRef checks out the ref that is constructed by using the module
// name and version. If the version provided is empty the main branch will be
// checked out.
func checkoutRef(dir string, modName string, version string) error {
	var ref string
	if version == "" {
		ref = "main"
	} else {
		// Transform cloud.google.com/storage/v2 into storage for example
		vPrefix := strings.Split(strings.TrimPrefix(modName, "cloud.google.com/go/"), "/")[0]
		ref = fmt.Sprintf("%s/%s", vPrefix, version)

	}
	return git.CheckoutRef(dir, ref)
}

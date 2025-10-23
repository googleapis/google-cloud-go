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

//go:build !windows
// +build !windows

// Package generator provides tools for generating clients.
package generator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cloud.google.com/go/internal/gapicgen/git"
)

// Config contains inputs needed to generate sources.
type Config struct {
	GoogleapisDir string
	GenprotoDir   string
	ProtoDir      string
	LocalMode     bool
	RegenOnly     bool
	ForceAll      bool
	GenAlias      bool
}

// Generate generates genproto and gapics.
func Generate(ctx context.Context, conf *Config) ([]*git.ChangeInfo, error) {
	protoGenerator := NewGenprotoGenerator(conf)
	if err := protoGenerator.Regen(ctx); err != nil {
		return nil, fmt.Errorf("error generating genproto (may need to check logs for more errors): %w", err)
	}

	var changes []*git.ChangeInfo
	if !conf.LocalMode {
		var err error
		changes, err = gatherChanges(conf.GoogleapisDir, conf.GenprotoDir)
		if err != nil {
			return nil, fmt.Errorf("error gathering commit info: %w", err)
		}
		if err := recordGoogleapisHash(conf.GoogleapisDir, conf.GenprotoDir); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

func gatherChanges(googleapisDir, genprotoDir string) ([]*git.ChangeInfo, error) {
	// Get the last processed googleapis hash.
	lastHash, err := os.ReadFile(filepath.Join(genprotoDir, "regen.txt"))
	if err != nil {
		return nil, err
	}
	commits, err := git.CommitsSinceHash(googleapisDir, string(lastHash), false)
	if err != nil {
		return nil, err
	}
	affectedProtos := make(map[string][]string)
	var relevantCommits []string
	for _, commit := range commits {
		files, err := git.FilesChanged(googleapisDir, commit)
		if err != nil {
			return nil, err
		}
		protos := make(map[string]struct{})
		for _, file := range files {
			if file == "" {
				continue
			}
			if !strings.HasSuffix(file, ".proto") {
				continue
			}
			content, err := git.GetFileContentAtCommit(googleapisDir, commit, file)
			if err != nil {
				// It's possible the file was deleted in this commit, so we check the parent.
				originalErr := err
				content, err = git.GetFileContentAtCommit(googleapisDir, commit+"^", file)
				if err != nil {
					// We don't want to fail here, just log the error and continue.
					log.Printf("could not get content for %s at commit %s (%v) or its parent (%v)", file, commit, originalErr, err)
					continue
				}
			}
			pkg, err := parseGoPkg(content)
			if err != nil {
				return nil, err
			}
			var onWatchlist bool
			for _, watchedPkg := range generateList {
				if pkg == watchedPkg {
					onWatchlist = true
					break
				}
			}
			if onWatchlist {
				if _, ok := affectedProtos[commit]; !ok {
					relevantCommits = append(relevantCommits, commit)
				}
				protos[file] = struct{}{}
			}
		}
		for proto := range protos {
			affectedProtos[commit] = append(affectedProtos[commit], proto)
		}
		sort.Strings(affectedProtos[commit])
	}

	changes, err := git.ParseChangeInfo(googleapisDir, relevantCommits, affectedProtos)
	if err != nil {
		return nil, err
	}

	return changes, nil
}

// recordGoogleapisHash parses the latest commit in googleapis and records it to
// regen.txt in go-genproto.
func recordGoogleapisHash(googleapisDir, genprotoDir string) error {
	commits, err := git.CommitsSinceHash(googleapisDir, "HEAD", true)
	if err != nil {
		return err
	}
	if len(commits) != 1 {
		return fmt.Errorf("only expected one commit, got %d", len(commits))
	}

	f, err := os.OpenFile(filepath.Join(genprotoDir, "regen.txt"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(commits[0]); err != nil {
		return err
	}
	return nil
}

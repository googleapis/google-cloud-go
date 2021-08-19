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

// Package generator provides tools for generating clients.
package generator

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"cloud.google.com/go/internal/gapicgen/git"
)

// Config contains inputs needed to generate sources.
type Config struct {
	GoogleapisDir      string
	GoogleapisDiscoDir string
	GenprotoDir        string
	GapicDir           string
	ProtoDir           string
	GapicToGenerate    string
	OnlyGenerateGapic  bool
	LocalMode          bool
	RegenOnly          bool
	ForceAll           bool
}

// Generate generates genproto and gapics.
func Generate(ctx context.Context, conf *Config) ([]*git.ChangeInfo, error) {
	if !conf.OnlyGenerateGapic {
		protoGenerator := NewGenprotoGenerator(conf)
		if err := protoGenerator.Regen(ctx); err != nil {
			return nil, fmt.Errorf("error generating genproto (may need to check logs for more errors): %v", err)
		}
	}

	var changes []*git.ChangeInfo
	if !conf.LocalMode {
		var err error
		changes, err = gatherChanges(conf.GoogleapisDir, conf.GenprotoDir)
		if err != nil {
			return nil, fmt.Errorf("error gathering commit info")
		}
		if err := recordGoogleapisHash(conf.GoogleapisDir, conf.GenprotoDir); err != nil {
			return nil, err
		}
	}
	var modifiedPkgs []string
	for _, v := range changes {
		if v.Package != "" {
			modifiedPkgs = append(modifiedPkgs, v.PackageDir)
		}
	}

	gapicGenerator := NewGapicGenerator(conf, modifiedPkgs)
	if err := gapicGenerator.Regen(ctx); err != nil {
		return nil, fmt.Errorf("error generating gapics (may need to check logs for more errors): %v", err)
	}

	return changes, nil
}

func gatherChanges(googleapisDir, genprotoDir string) ([]*git.ChangeInfo, error) {
	// Get the last processed googleapis hash.
	lastHash, err := ioutil.ReadFile(filepath.Join(genprotoDir, "regen.txt"))
	if err != nil {
		return nil, err
	}
	commits, err := git.CommitsSinceHash(googleapisDir, string(lastHash), false)
	if err != nil {
		return nil, err
	}
	gapicPkgs := make(map[string]string)
	for _, v := range microgenGapicConfigs {
		gapicPkgs[v.inputDirectoryPath] = v.importPath
	}
	changes, err := git.ParseChangeInfo(googleapisDir, commits, gapicPkgs)
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

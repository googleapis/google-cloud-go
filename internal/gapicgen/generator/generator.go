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

// Package generator provides tools for generating clients.
package generator

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Generate generates genproto and gapics.
func Generate(ctx context.Context, googleapisDir, genprotoDir, gocloudDir, protoDir string, gapicToGenerate string) ([]*ChangeInfo, error) {
	protoGenerator := NewGenprotoGenerator(genprotoDir, googleapisDir, protoDir)
	gapicGenerator := NewGapicGenerator(googleapisDir, protoDir, gocloudDir, genprotoDir)
	if err := protoGenerator.Regen(ctx); err != nil {
		return nil, fmt.Errorf("error generating genproto (may need to check logs for more errors): %v", err)
	}
	if err := gapicGenerator.Regen(ctx, gapicToGenerate); err != nil {
		return nil, fmt.Errorf("error generating gapics (may need to check logs for more errors): %v", err)
	}

	changes, err := gatherChanges(googleapisDir, genprotoDir)
	if err != nil {
		return nil, fmt.Errorf("error gathering commit info")
	}

	if err := recordGoogleapisHash(googleapisDir, genprotoDir); err != nil {
		return nil, err
	}

	return changes, nil
}

func gatherChanges(googleapisDir, genprotoDir string) ([]*ChangeInfo, error) {
	// Get the last processed googleapis hash.
	lastHash, err := ioutil.ReadFile(filepath.Join(genprotoDir, "regen.txt"))
	if err != nil {
		return nil, err
	}
	commits, err := CommitsSinceHash(googleapisDir, string(lastHash), true)
	if err != nil {
		return nil, err
	}
	changes, err := ParseChangeInfo(googleapisDir, commits)
	if err != nil {
		return nil, err
	}

	return changes, nil
}

// recordGoogleapisHash parses the latest commit in googleapis and records it to
// regen.txt in go-genproto.
func recordGoogleapisHash(googleapisDir, genprotoDir string) error {
	commits, err := CommitsSinceHash(googleapisDir, "HEAD", true)
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

// build attempts to build all packages recursively from the given directory.
func build(dir string) error {
	log.Println("building generated code")
	c := command("go", "build", "./...")
	c.Dir = dir
	return c.Run()
}

// vet runs linters on all .go files recursively from the given directory.
func vet(dir string) error {
	log.Println("vetting generated code")
	c := command("goimports", "-w", ".")
	c.Dir = dir
	if err := c.Run(); err != nil {
		return err
	}

	c = command("gofmt", "-s", "-d", "-w", "-l", ".")
	c.Dir = dir
	return c.Run()
}

type cmdWrapper struct {
	*exec.Cmd
}

// command wraps a exec.Command to add some logging about commands being run.
// The commands stdout/stderr default to os.Stdout/os.Stderr respectfully.
func command(name string, arg ...string) *cmdWrapper {
	c := &cmdWrapper{exec.Command(name, arg...)}
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

func (cw *cmdWrapper) Run() error {
	log.Printf(">>>> %v <<<<", strings.Join(cw.Cmd.Args, " ")) // NOTE: we have some multi-line commands, make it clear where the command starts and ends
	return cw.Cmd.Run()
}

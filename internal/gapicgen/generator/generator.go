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
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Generate generates genproto and gapics.
func Generate(ctx context.Context, googleapisDir, genprotoDir, gocloudDir, protoDir string, gapicToGenerate string) error {
	if err := regenGenproto(ctx, genprotoDir, googleapisDir, protoDir); err != nil {
		return fmt.Errorf("error generating genproto (may need to check logs for more errors): %v", err)
	}

	if err := generateGapics(ctx, googleapisDir, protoDir, gocloudDir, genprotoDir, gapicToGenerate); err != nil {
		return fmt.Errorf("error generating gapics (may need to check logs for more errors): %v", err)
	}

	if err := recordGoogleapisHash(googleapisDir, genprotoDir); err != nil {
		return fmt.Errorf("error recording most recent googleapis hash: %v", err)
	}

	return nil
}

// recordGoogleapisHash parses the latest commit in googleapis and records it to
// regen.txt in go-genproto.
func recordGoogleapisHash(googleapisDir, genprotoDir string) error {
	out := bytes.NewBuffer(nil)
	c := command("git", "rev-list", "HEAD^..")
	c.Stdout = out
	c.Stderr = os.Stderr
	c.Dir = googleapisDir
	if err := c.Run(); err != nil {
		return err
	}
	commits := strings.Split(strings.TrimSpace(out.String()), "\n")
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
	c := command("go", "build", "./...")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = dir
	return c.Run()
}

// vet runs linters on all .go files recursively from the given directory.
func vet(dir string) error {
	c := command("goimports", "-w", ".")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = dir
	if err := c.Run(); err != nil {
		return err
	}

	c = command("gofmt", "-s", "-d", "-w", "-l", ".")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = dir
	return c.Run()
}

type cmdWrapper struct {
	*exec.Cmd
}

func command(name string, arg ...string) *cmdWrapper {
	return &cmdWrapper{exec.Command(name, arg...)}
}

func (cw *cmdWrapper) Run() error {
	log.Printf(">>>> %v <<<<", strings.Join(cw.Cmd.Args, " ")) // NOTE: we have some multi-line commands, make it clear where the command starts and ends
	return cw.Cmd.Run()
}

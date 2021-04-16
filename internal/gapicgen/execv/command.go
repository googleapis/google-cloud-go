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

// Package execv provides a wrapper around exec.Cmd for debugging purposes.
package execv

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

// CmdWrapper is a wrapper around exec.Cmd for debugging purposes.
type CmdWrapper struct {
	*exec.Cmd
}

// Command wraps a exec.Command to add some logging about commands being run.
// The commands stdout/stderr default to os.Stdout/os.Stderr respectfully.
func Command(name string, arg ...string) *CmdWrapper {
	c := &CmdWrapper{exec.Command(name, arg...)}
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return &CmdWrapper{exec.Command(name, arg...)}
}

// Run a command.
func (c *CmdWrapper) Run() error {
	log.Printf("[%s] >>>> %v <<<<", c.Dir, strings.Join(c.Args, " ")) // NOTE: we have some multi-line commands, make it clear where the command starts and ends
	return c.Cmd.Run()
}

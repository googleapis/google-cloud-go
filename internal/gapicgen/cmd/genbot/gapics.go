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

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
)

// gerritRegex is used to find gerrit links.
var gerritRegex = regexp.MustCompile(`https://code-review.googlesource.com.+[0-9]+`)

const (
	gerritCommitTitle = "all: auto-regenerate gapics"
	gerritCommitBody  = `
This is an auto-generated regeneration of the gapic clients by
cloud.google.com/go/internal/gapicgen. Once the corresponding genproto PR is
submitted, genmgr will update this CL with a newer dependency to the newer
version of genproto and assign reviewers to this CL.

If you have been assigned to review this CL, please:

- Ensure that the version of genproto in go.mod has been updated.
- Ensure that CI is passing. If it's failing, it requires your manual attention.
- +2 and submit this CL if you believe it's ready to ship.
`
)

// clGocloud creates a CL for the given gocloud change (including a link to
// the given genproto PR).
//
// genprotoPRNum may be -1 to indicate there is no corresponding genproto PR.
func clGocloud(ctx context.Context, gocloudDir string, genprotoPRNum int) (url string, _ error) {
	log.Println("creating gocloud CL")

	var newBody string
	if genprotoPRNum > 0 {
		newBody = gerritCommitBody + fmt.Sprintf("\n\nCorresponding genproto PR: https://github.com/googleapis/go-genproto/pull/%d\n", genprotoPRNum)
	} else {
		newBody = gerritCommitBody + "\n\nThere is no corresponding genproto PR.\n"
	}

	// Write command output to both os.Stderr and local, so that we can check
	// for gerrit URL.
	inmem := bytes.NewBuffer([]byte{}) // TODO(deklerk): Try `var inmem bytes.Buffer`.
	w := io.MultiWriter(os.Stderr, inmem)

	c := exec.Command("/bin/bash", "-c", `
set -ex

CHANGE_ID=$(echo $RANDOM | git hash-object --stdin)
git checkout master
git branch -d regen_gapics || true
git add -A
git checkout -b regen_gapics
git commit -m "$COMMIT_TITLE" -m "$COMMIT_BODY" -m "Change-Id: I$CHANGE_ID"
git-codereview mail
`)
	c.Stdout = os.Stdout
	c.Stderr = w
	c.Stdin = os.Stdin // Prevents "the input device is not a TTY" error.
	c.Env = []string{
		fmt.Sprintf("COMMIT_TITLE=%s", gerritCommitTitle),
		fmt.Sprintf("COMMIT_BODY=%s", newBody),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	c.Dir = gocloudDir
	if err := c.Run(); err != nil {
		return "", err
	}

	b := inmem.Bytes()

	clURL := gerritRegex.FindString(string(b))
	if clURL == "" {
		return "", errors.New("couldn't get CL URL from gerrit push message")
	}

	log.Printf("creating gocloud CL... done %s\n", clURL)
	return clURL, nil
}

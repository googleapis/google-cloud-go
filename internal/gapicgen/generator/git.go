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

package generator

import (
	"bytes"
	"fmt"
	"strings"
)

// ChangeInfo represents a change and its associated metadata.
type ChangeInfo struct {
	Body            string
	GoogleapisHash  string
	HasGapicChanges bool
}

// ParseChangeInfo gets the ChangeInfo for a given googleapis hash.
func ParseChangeInfo(googleapisDir string, hashes []string) ([]*ChangeInfo, error) {
	var changes []*ChangeInfo
	// gapicPkgs is a map of googleapis inputDirectoryPath to the gapic package
	// name used for conventional commits.
	gapicPkgs := make(map[string]string)
	for _, v := range microgenGapicConfigs {
		gapicPkgs[v.inputDirectoryPath] = parseConventionalCommitPkg(v.importPath)
	}

	for _, hash := range hashes {
		// Get commit title and body
		rawBody := bytes.NewBuffer(nil)
		c := command("git", "show", "--pretty=format:%B", "-s", hash)
		c.Stdout = rawBody
		c.Dir = googleapisDir
		if err := c.Run(); err != nil {
			return nil, err
		}

		// Add link so corresponding googleapis commit.
		body := fmt.Sprintf("%s\nSource-Link: https://github.com/googleapis/googleapis/commit/%s", strings.TrimSpace(rawBody.String()), hash)

		// Try to map files updated to a package in google-cloud-go. Assumes only
		// one servies protos are updated per commit. Multile versions are okay.
		files, err := filesChanged(googleapisDir, hash)
		if err != nil {
			return nil, err
		}
		var pkg string
		for _, file := range files {
			ss := strings.Split(file, "/")
			if len(ss) == 0 {
				continue
			}
			// remove filename from path
			strings.Join(ss[:len(ss)-1], "/")
			tempPkg := gapicPkgs[strings.Join(ss[:len(ss)-1], "/")]
			if tempPkg != "" {
				pkg = tempPkg
				break
			}
		}
		if pkg == "" {
			changes = append(changes, &ChangeInfo{
				Body:           body,
				GoogleapisHash: hash,
			})
			continue
		}

		// Try to add in pkg affected into conventional commit scope.
		bodyParts := strings.SplitN(body, "\n", 2)
		if len(bodyParts) > 0 {
			titleParts := strings.SplitN(bodyParts[0], ":", 2)
			if len(titleParts) == 2 {
				// If a scope is already provided, remove it.
				if i := strings.Index(titleParts[0], "("); i > 0 {
					titleParts[0] = titleParts[0][:i]
				}
				titleParts[0] = fmt.Sprintf("%s(%s)", titleParts[0], pkg)
				bodyParts[0] = strings.Join(titleParts, ":")
			}
			body = strings.Join(bodyParts, "\n")
		}

		changes = append(changes, &ChangeInfo{
			Body:            body,
			GoogleapisHash:  hash,
			HasGapicChanges: true,
		})
	}
	return changes, nil
}

func parseConventionalCommitPkg(importPath string) string {
	s := strings.TrimPrefix(importPath, "cloud.google.com/go/")
	ss := strings.Split(s, "/")
	// remove the version, i.e /apiv1
	return strings.Join(ss[:len(ss)-1], "/")
}

// CommitsSinceHash gathers all of the commits since the provided hash for the
// given gitDir. The inclusive parameter tells if the provided hash should also
// be returned in the slice.
func CommitsSinceHash(gitDir, hash string, inclusive bool) ([]string, error) {
	var commitRange string
	if inclusive {
		commitRange = fmt.Sprintf("%s^..", hash)
	} else {
		commitRange = fmt.Sprintf("%s..", hash)
	}

	out := bytes.NewBuffer(nil)
	c := command("git", "rev-list", commitRange)
	c.Stdout = out
	c.Dir = gitDir
	if err := c.Run(); err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(out.String()), "\n"), nil
}

// UpdateFilesSinceHash returns a listed of files updated since the provided
// hash for the given gitDir.
func UpdateFilesSinceHash(gitDir, hash string) ([]string, error) {
	out := bytes.NewBuffer(nil)
	// The provided diff-filter flags restricts to files that have been:
	// - (A) Added
	// - (C) Copied
	// - (M) Modified
	// - (R) Renamed
	c := command("git", "diff-tree", "--no-commit-id", "--name-only", "--diff-filter=ACMR", "-r", fmt.Sprintf("%s..HEAD", hash))
	c.Stdout = out
	c.Dir = gitDir
	if err := c.Run(); err != nil {
		return nil, err
	}
	return strings.Split(out.String(), "\n"), nil
}

// filesChanged returns a list of files changed in a commit for the provdied
// hash in the given gitDir.
func filesChanged(gitDir, hash string) ([]string, error) {
	out := bytes.NewBuffer(nil)
	c := command("git", "show", "--pretty=format:", "--name-only", hash)
	c.Stdout = out
	c.Dir = gitDir
	if err := c.Run(); err != nil {
		return nil, err
	}
	return strings.Split(out.String(), "\n"), nil
}

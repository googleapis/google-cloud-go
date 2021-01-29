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
	Body           string
	Title          string
	Package        string
	GoogleapisHash string
}

// FormatChanges turns a slice of changes into formatted string that will match
// the conventional commit footer pattern. This will allow these changes to be
// parsed into the changelog.
func FormatChanges(changes []*ChangeInfo, onlyGapicChanges bool) string {
	if len(changes) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nChanges:\n\n")
	for _, c := range changes {
		if onlyGapicChanges && c.Package == "" {
			continue
		}

		title := c.Title
		if c.Package != "" {
			// Try to add in pkg affected into conventional commit scope.
			titleParts := strings.SplitN(c.Title, ":", 2)
			if len(titleParts) == 2 {
				// If a scope is already provided, remove it.
				if i := strings.Index(titleParts[0], "("); i > 0 {
					titleParts[0] = titleParts[0][:i]
				}
				titleParts[0] = fmt.Sprintf("%s(%s)", titleParts[0], c.Package)
			}
			title = strings.Join(titleParts, ":")
		}
		sb.WriteString(fmt.Sprintf("%s\n", title))

		// Format the commit body to conventional commit footer standards.
		splitBody := strings.Split(c.Body, "\n")
		for i := range splitBody {
			splitBody[i] = fmt.Sprintf("  %s", splitBody[i])
		}
		body := strings.Join(splitBody, "\n")
		sb.WriteString(fmt.Sprintf("%s\n\n", body))
	}
	// If the buffer is empty except for the "Changes:" text return an empty
	// string.
	if sb.Len() == 11 {
		return ""
	}
	return sb.String()
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
		c := command("git", "show", "--pretty=format:%s~~%b", "-s", hash)
		c.Stdout = rawBody
		c.Dir = googleapisDir
		if err := c.Run(); err != nil {
			return nil, err
		}

		ss := strings.Split(rawBody.String(), "~~")
		if len(ss) != 2 {
			return nil, fmt.Errorf("expected two segments for commit, got %d: %q", len(ss), rawBody.String())
		}
		title, body := strings.TrimSpace(ss[0]), strings.TrimSpace(ss[1])

		// Add link so corresponding googleapis commit.
		body = fmt.Sprintf("%s\nSource-Link: https://github.com/googleapis/googleapis/commit/%s", body, hash)

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

		changes = append(changes, &ChangeInfo{
			Title:          title,
			Body:           body,
			Package:        pkg,
			GoogleapisHash: hash,
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

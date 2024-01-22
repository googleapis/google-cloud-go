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

package git

import (
	"fmt"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/internal/gapicgen/execv"
	"github.com/go-git/go-git/v5"
)

const (
	maxTruncatedTitleLen = 22
)

// ChangeInfo represents a change and its associated metadata.
type ChangeInfo struct {
	Body           string
	Title          string
	GoogleapisHash string
}

// FormatChanges turns a slice of changes into formatted string that will match
// the conventional commit footer pattern. This will allow these changes to be
// parsed into the changelog.
func FormatChanges(changes []*ChangeInfo, onlyGapicChanges bool) string {
	formatted := truncateAndFormatChanges(changes, onlyGapicChanges, false)
	if len(formatted) > maxChangesLen {
		// Retry formatting by truncating
		return truncateAndFormatChanges(changes, onlyGapicChanges, true)
	}
	return formatted
}

func truncateAndFormatChanges(changes []*ChangeInfo, onlyGapicChanges, truncate bool) string {
	if len(changes) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nChanges:\n\n")
	for _, c := range changes {
		if onlyGapicChanges {
			continue
		}
		title := c.Title
		if truncate && len(title) > maxTruncatedTitleLen {
			title = fmt.Sprintf("%v...", title[:maxTruncatedTitleLen])
		}
		sb.WriteString(fmt.Sprintf("%s\n", title))

		// Format the commit body to conventional commit footer standards.
		splitBody := strings.Split(c.Body, "\n")
		for i := range splitBody {
			splitBody[i] = fmt.Sprintf("  %s", splitBody[i])
		}
		body := strings.Join(splitBody, "\n")

		if truncate {
			startBody := strings.Index(body, "PiperOrigin-RevId")
			if startBody != -1 {
				body = fmt.Sprintf("  %s", body[startBody:])
			}
		}

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
	for _, hash := range hashes {
		// Get commit title and body
		c := execv.Command("git", "show", "--pretty=format:%s~~%b", "-s", hash)
		c.Dir = googleapisDir
		b, err := c.Output()
		if err != nil {
			return nil, err
		}

		ss := strings.Split(string(b), "~~")
		if len(ss) != 2 {
			return nil, fmt.Errorf("expected two segments for commit, got %d: %s", len(ss), b)
		}
		title, body := strings.TrimSpace(ss[0]), strings.TrimSpace(ss[1])

		// Add link so corresponding googleapis commit.
		body = fmt.Sprintf("%s\nSource-Link: https://github.com/googleapis/googleapis/commit/%s", body, hash)

		changes = append(changes, &ChangeInfo{
			Title:          title,
			Body:           body,
			GoogleapisHash: hash,
		})
	}
	return changes, nil
}

// parseConventionalCommitPkg parses the package context for conventional commit
// messages.
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

	c := execv.Command("git", "rev-list", commitRange)
	c.Dir = gitDir
	b, err := c.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n"), nil
}

// UpdateFilesSinceHash returns a listed of files updated since the provided
// hash for the given gitDir.
func UpdateFilesSinceHash(gitDir, hash string) ([]string, error) {
	// The provided diff-filter flags restricts to files that have been:
	// - (A) Added
	// - (C) Copied
	// - (M) Modified
	// - (R) Renamed
	c := execv.Command("git", "diff-tree", "--no-commit-id", "--name-only", "--diff-filter=ACMR", "-r", fmt.Sprintf("%s..HEAD", hash))
	c.Dir = gitDir
	b, err := c.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

// HasChanges reports whether the given directory has uncommitted git changes.
func HasChanges(dir string) (bool, error) {
	c := execv.Command("bash", "-c", "git status --short")
	c.Dir = dir
	b, err := c.Output()
	return len(b) > 0, err
}

// DeepClone clones a repository in the given directory.
func DeepClone(repo, dir string) error {
	log.Printf("cloning %s\n", repo)

	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:      repo,
		Progress: os.Stdout,
	})
	return err
}

// filesChanged returns a list of files changed in a commit for the provdied
// hash in the given gitDir.
func filesChanged(gitDir, hash string) ([]string, error) {
	c := execv.Command("git", "show", "--pretty=format:", "--name-only", hash)
	c.Dir = gitDir
	b, err := c.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

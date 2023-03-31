package main

import (
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv"
)

// filesChanged returns a list of files changed in a commit for the provdied
// hash in the given gitDir. Copied fromm google-cloud-go/gapicgen/git/git.go
func filesChanged(dir, hash string) ([]string, error) {
	out := execv.Command("git", "show", "--pretty=format:", "--name-only", hash)
	out.Dir = dir
	b, err := out.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

// runAll uses git to tell if the PR being updated should run all post
// processing logic.
func runAll(dir, branchOverride string) (bool, error) {
	if branchOverride != "" {
		// This means we are running the post processor locally and want it to
		// fully function -- so we lie.
		return true, nil
	}
	c := execv.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return false, err
	}
	branchName := strings.TrimSpace(string(b))
	return strings.HasPrefix(branchName, owlBotBranchPrefix), nil
}

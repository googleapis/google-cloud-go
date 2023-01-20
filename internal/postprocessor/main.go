// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cloud.google.com/go/internal/gapicgen/generator"
	"cloud.google.com/go/internal/gapicgen/git"
	"cloud.google.com/go/internal/gensnippets"
	"cloud.google.com/go/internal/postprocessor/execv"
	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"github.com/google/go-github/v35/github"

	"gopkg.in/yaml.v2"
)

const (
	owlBotBranchPrefix = "owl-bot-copy"
	apiNameOwlBotScope = "[REPLACEME]"
)

var (
	// hashFromLinePattern grabs the hash from the end of a github commit URL
	hashFromLinePattern = regexp.MustCompile(`.*/(?P<hash>[a-zA-Z0-9]*).*`)
	// firstPartTitlePattern grabs the existing commit title before the ': [REPLACEME]'
	firstPartTitlePattern = regexp.MustCompile(`(?P<titleFirstPart>)(\: *\` + apiNameOwlBotScope + `)(.*)`)
	// secondPartTitlePattern grabs the commit title after the ': [REPLACME]'
	secondPartTitlePattern = regexp.MustCompile(`.*\: *\` + apiNameOwlBotScope + ` *(?P<titleSecondPart>.*)`)
)

func main() {
	clientRoot := flag.String("client-root", "/workspace/google-cloud-go", "Path to clients.")
	googleapisDir := flag.String("googleapis-dir", "", "Path to googleapis/googleapis repo.")
	directories := flag.String("dirs", "", "Comma-separated list of module names to run (not paths).")
	branchOverride := flag.String("branch", "", "The branch that should be processed by this code")
	githubUsername := flag.String("gh-user", "googleapis", "GitHub username where repo lives.")
	prFilepath := flag.String("pr-file", "/workspace/new_pull_request_text.txt", "Path at which to write text file if changing PR title or body.")

	flag.Parse()

	runAll, err := runAll(*clientRoot, *branchOverride)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	log.Println("client-root set to", *clientRoot)
	log.Println("googleapis-dir set to", *googleapisDir)
	log.Println("branch set to", *branchOverride)
	log.Println("prFilepath is", *prFilepath)

	var modules []string
	if *directories != "" {
		dirSlice := strings.Split(*directories, ",")
		for _, dir := range dirSlice {
			modules = append(modules, filepath.Join(*clientRoot, dir))
		}
		log.Println("Postprocessor running on", modules)
	} else {
		log.Println("Postprocessor running on all modules.")
	}

	if *googleapisDir == "" {
		log.Println("creating temp dir")
		tmpDir, err := os.MkdirTemp("", "update-postprocessor")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		log.Printf("working out %s\n", tmpDir)
		*googleapisDir = filepath.Join(tmpDir, "googleapis")

		if err := git.DeepClone("https://github.com/googleapis/googleapis", *googleapisDir); err != nil {
			log.Fatal(err)
		}
	}

	c := &config{
		googleapisDir:  *googleapisDir,
		googleCloudDir: *clientRoot,
		modules:        modules,
		branchOverride: *branchOverride,
		githubUsername: *githubUsername,
		prFilepath:     *prFilepath,
		runAll:         runAll,
	}

	if err := c.run(ctx); err != nil {
		log.Fatal(err)
	}

	log.Println("Completed successfully.")
}

type config struct {
	googleapisDir  string
	googleCloudDir string
	modules        []string
	branchOverride string
	githubUsername string
	prFilepath     string
	runAll         bool
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

func (c *config) run(ctx context.Context) error {
	if !c.runAll {
		log.Println("exiting post processing early")
		return nil
	}
	if err := gocmd.ModTidyAll(c.googleCloudDir); err != nil {
		return err
	}
	if err := gocmd.Vet(c.googleCloudDir); err != nil {
		return err
	}
	if err := c.RegenSnippets(); err != nil {
		return err
	}
	if _, err := c.Manifest(generator.MicrogenGapicConfigs); err != nil {
		return err
	}
	// TODO(codyoss): In the future we may want to make it possible to be able
	// to run this locally with a user defined remote branch.
	if err := c.AmendPRDescription(ctx); err != nil {
		return err
	}
	return nil
}

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (c *config) RegenSnippets() error {
	log.Println("regenerating snippets")

	snippetDir := filepath.Join(c.googleCloudDir, "internal", "generated", "snippets")
	apiShortnames, err := generator.ParseAPIShortnames(c.googleapisDir, generator.MicrogenGapicConfigs, generator.ManualEntries)

	if err != nil {
		return err
	}
	if err := gensnippets.GenerateSnippetsDirs(c.googleCloudDir, snippetDir, apiShortnames, c.modules); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := c.replaceAllForSnippets(c.googleCloudDir, snippetDir); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}

	return nil
}

func (c *config) replaceAllForSnippets(googleCloudDir, snippetDir string) error {
	return execv.ForEachMod(googleCloudDir, func(dir string) error {
		if c.modules != nil {
			for _, mod := range c.modules {
				if !strings.Contains(dir, mod) {
					return nil
				}
			}
		}
		if dir == snippetDir {
			return nil
		}
		mod, err := gocmd.ListModName(dir)
		if err != nil {
			return err
		}
		// Replace it. Use a relative path to avoid issues on different systems.
		rel, err := filepath.Rel(snippetDir, dir)
		if err != nil {
			return err
		}
		c := execv.Command("bash", "-c", `go mod edit -replace "$MODULE=$MODULE_PATH"`)
		c.Dir = snippetDir
		c.Env = []string{
			fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
			fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
			fmt.Sprintf("MODULE=%s", mod),
			fmt.Sprintf("MODULE_PATH=%s", rel),
		}
		return c.Run()
	})
}

// manifest writes a manifest file with info about all of the confs.
func (c *config) Manifest(confs []*generator.MicrogenConfig) (map[string]generator.ManifestEntry, error) {
	log.Println("updating gapic manifest")
	entries := map[string]generator.ManifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(c.googleCloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for _, manual := range generator.ManualEntries {
		entries[manual.DistributionName] = manual
	}
	for _, conf := range confs {
		yamlPath := filepath.Join(c.googleapisDir, conf.InputDirectoryPath, conf.ApiServiceConfigPath)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		yamlConfig := struct {
			Title string `yaml:"title"` // We only need the title field.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		docURL, err := docURL(c.googleCloudDir, conf.ImportPath)
		if err != nil {
			return nil, fmt.Errorf("unable to build docs URL: %v", err)
		}
		entry := generator.ManifestEntry{
			DistributionName:  conf.ImportPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           docURL,
			ReleaseLevel:      conf.ReleaseLevel,
			LibraryType:       generator.GapicAutoLibraryType,
		}
		entries[conf.ImportPath] = entry
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return entries, enc.Encode(entries)
}

func docURL(cloudDir, importPath string) (string, error) {
	suffix := strings.TrimPrefix(importPath, "cloud.google.com/go/")
	mod, err := gocmd.CurrentMod(filepath.Join(cloudDir, suffix))
	if err != nil {
		return "", err
	}
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
	return "https://cloud.google.com/go/docs/reference/" + mod + "/latest/" + pkgPath, nil
}

func (c *config) AmendPRDescription(ctx context.Context) error {
	log.Println("Amending PR title and body")
	pr, err := c.getPR(ctx)
	if err != nil {
		return err
	}
	newPRTitle, newPRBody, err := c.processCommit(*pr.Title, *pr.Body)
	if err != nil {
		return err
	}
	return c.writePRCommitToFile(newPRTitle, newPRBody)
}

func (c *config) processCommit(title, body string) (string, string, error) {
	var newPRTitle string
	var commitTitle string
	var commitTitleIndex int

	bodySlice := strings.Split(body, "\n")
	for index, line := range bodySlice {
		if strings.Contains(line, apiNameOwlBotScope) {
			commitTitle = line
			commitTitleIndex = index
			continue
		}
		// When OwlBot generates the commit body, after commit titles it provides 'Source-Link's.
		// The source-link pointing to the googleapis/googleapis repo commit allows us to extract
		// hash and find files changed in order to identify the commit's scope.
		if !strings.Contains(line, "googleapis/googleapis/") {
			continue
		}
		hash := extractHashFromLine(line)
		scope, err := c.getScopeFromGoogleapisCommitHash(hash)
		if err != nil {
			return "", "", err
		}
		if newPRTitle == "" {
			newPRTitle = updateCommitTitle(title, scope)
			continue
		}
		newCommitTitle := updateCommitTitle(commitTitle, scope)
		bodySlice[commitTitleIndex] = newCommitTitle
	}
	body = strings.Join(bodySlice, "\n")
	return newPRTitle, body, nil
}

func (c *config) getPR(ctx context.Context) (*github.PullRequest, error) {
	client := github.NewClient(nil)
	prs, _, err := client.PullRequests.List(ctx, c.githubUsername, "google-cloud-go", nil)
	if err != nil {
		return nil, err
	}
	var owlbotPR *github.PullRequest
	branch := c.branchOverride
	if c.branchOverride == "" {
		branch = owlBotBranchPrefix
	}
	for _, pr := range prs {
		if strings.Contains(*pr.Head.Label, branch) {
			owlbotPR = pr
		}
	}
	if owlbotPR == nil {
		return nil, errors.New("no OwlBot PR found")
	}
	return owlbotPR, nil
}

func (c *config) getScopeFromGoogleapisCommitHash(commitHash string) (string, error) {
	files, err := c.filesChanged(commitHash)
	if err != nil {
		return "", err
	}
	// if no files changed, return empty string
	if len(files) == 0 {
		return "", nil
	}
	scopesMap := make(map[string]bool)
	scopes := []string{}
	for _, filePath := range files {
		for _, config := range generator.MicrogenGapicConfigs {
			if config.InputDirectoryPath == filepath.Dir(filePath) {
				scope := config.Pkg
				if _, value := scopesMap[scope]; !value {
					scopesMap[scope] = true
					scopes = append(scopes, scope)
				}
				break
			}
		}
	}
	// if no in-scope packages are found or if many packages found, return empty string
	if len(scopes) != 1 {
		return "", nil
	}
	// if single scope found, return
	return scopes[0], nil
}

// filesChanged returns a list of files changed in a commit for the provdied
// hash in the given gitDir. Copied fromm google-cloud-go/gapicgen/git/git.go
func (c *config) filesChanged(hash string) ([]string, error) {
	out := execv.Command("git", "show", "--pretty=format:", "--name-only", hash)
	out.Dir = c.googleapisDir
	b, err := out.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

func extractHashFromLine(line string) string {
	hash := fmt.Sprintf("${%s}", hashFromLinePattern.SubexpNames()[1])
	hashVal := hashFromLinePattern.ReplaceAllString(line, hash)

	return hashVal
}

func updateCommitTitle(title, titlePkg string) string {
	var newTitle string

	firstTitlePart := firstPartTitlePattern.ReplaceAllString(title, "$titleFirstPart")
	secondTitlePart := secondPartTitlePattern.ReplaceAllString(title, "$titleSecondPart")

	var breakChangeIndicator string
	if strings.HasSuffix(firstTitlePart, "!") {
		breakChangeIndicator = "!"
	}
	if titlePkg == "" {
		newTitle = fmt.Sprintf("%v%v: %v", firstTitlePart, breakChangeIndicator, secondTitlePart)
		return newTitle
	}
	newTitle = fmt.Sprintf("%v(%v)%v: %v", firstTitlePart, titlePkg, breakChangeIndicator, secondTitlePart)

	return newTitle
}

// writePRCommitToFile uses OwlBot env variable specified path to write updated PR title and body at that location
func (c *config) writePRCommitToFile(title, body string) error {
	// if file exists at location, delete
	if err := os.Remove(c.prFilepath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Println(err)
		} else {
			return err
		}
	}
	f, err := os.OpenFile(c.prFilepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(fmt.Sprintf("%s\n\n%s", title, body)); err != nil {
		return err
	}
	return nil
}

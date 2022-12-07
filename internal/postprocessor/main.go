// Copyright 2022 Google LLC
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
	"io"
	"io/fs"
	"io/ioutil"
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
	"github.com/google/go-github/github"
	"golang.org/x/sync/errgroup"

	"gopkg.in/yaml.v2"
)

func main() {
	var stagingDir string
	var clientRoot string
	var googleapisDir string
	var directories string
	flag.StringVar(&stagingDir, "stage-dir", "owl-bot-staging/src/", "Path to owl-bot-staging directory")
	flag.StringVar(&clientRoot, "client-root", "/repo", "Path to clients")
	flag.StringVar(&googleapisDir, "googleapis-dir", "", "Path to googleapis/googleapis repo")
	// The module names are relative to the client root - do not add paths. See README for example.
	flag.StringVar(&directories, "dirs", "", "Comma-separated list of modules to run")
	// For testing, specify dummy branch to edit PR title and body
	branchPrefix := flag.String("branch", "owl-bot-copy-", "The prefix of the branch that OwlBot opens when working on a PR.")
	githubAccessToken := flag.String("githubAccessToken", os.Getenv("GITHUB_ACCESS_TOKEN"), "The token used to open pull requests.")
	githubUsername := flag.String("githubUsername", os.Getenv("GITHUB_USERNAME"), "The GitHub user name for the author.")
	githubName := flag.String("githubName", os.Getenv("GITHUB_NAME"), "The name of the author for git commits.")
	githubEmail := flag.String("githubEmail", os.Getenv("GITHUB_EMAIL"), "The email address of the author.")

	flag.Parse()

	ctx := context.Background()

	log.Println("stage-dir set to", stagingDir)
	log.Println("client-root set to", clientRoot)
	log.Println("googleapis-dir set to", googleapisDir)

	cc := &clientConfig{
		githubAccessToken: *githubAccessToken,
		githubUsername:    *githubUsername,
		githubName:        *githubName,
		githubEmail:       *githubEmail,
		branchPrefix:      *branchPrefix,
	}

	log.Println("clientConfig instance is", *cc)

	var modules []string
	if directories != "" {
		dirSlice := strings.Split(directories, ",")
		for _, dir := range dirSlice {
			modules = append(modules, filepath.Join(clientRoot, dir))
		}
	}

	log.Println("modules set to", modules)
	if googleapisDir == "" {
		log.Println("creating temp dir")
		tmpDir, err := ioutil.TempDir("", "update-postprocessor")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		log.Printf("working out %s\n", tmpDir)
		googleapisDir = filepath.Join(tmpDir, "googleapis")

		// TODO: if not cloning other repos clean up
		grp, _ := errgroup.WithContext(ctx)
		grp.Go(func() error {
			return git.DeepClone("https://github.com/googleapis/googleapis", googleapisDir)
		})

		if err := grp.Wait(); err != nil {
			log.Fatal(err)
		}
	}

	c := &config{
		googleapisDir:  googleapisDir,
		googleCloudDir: clientRoot,
		stagingDir:     stagingDir,
		modules:        modules,
	}

	if err := c.run(ctx, cc); err != nil {
		log.Fatal(err)
	}

	log.Println("End of postprocessor script.")
}

type config struct {
	googleapisDir  string
	googleCloudDir string
	stagingDir     string
	modules        []string
}

type clientConfig struct {
	githubAccessToken string
	githubUsername    string
	githubName        string
	githubEmail       string
	branchPrefix      string
}

func (c *config) run(ctx context.Context, cc *clientConfig) error {
	if err := c.amendPRDescription(ctx, cc); err != nil {
		return err
	}

	filepath.WalkDir(c.stagingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		dstPath := filepath.Join(c.googleCloudDir, strings.TrimPrefix(path, c.stagingDir))
		if err := copyFiles(path, dstPath); err != nil {
			return err
		}
		return nil
	})
	if err := gocmd.ModTidyAll(c.googleCloudDir); err != nil {
		return err
	}
	if err := gocmd.Vet(c.googleCloudDir); err != nil {
		return err
	}
	if err := c.regenSnippets(); err != nil {
		return err
	}
	if _, err := c.manifest(generator.MicrogenGapicConfigs); err != nil {
		return err
	}

	return nil
}

func copyFiles(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	return nil
}

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (c *config) regenSnippets() error {
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
func (c *config) manifest(confs []*generator.MicrogenConfig) (map[string]generator.ManifestEntry, error) {
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

func (c *config) amendPRDescription(ctx context.Context, cc *clientConfig) error {
	PR, err := cc.getPR(ctx)
	if err != nil {
		return err
	}

	PRTitle := PR.Title
	PRBody := PR.Body

	newPRTitle, _, err := processCommit(*PRTitle, *PRBody, c.googleapisDir)
	if err != nil {
		return err
	}
	log.Println("newPRTitle is", newPRTitle)

	return nil
}

func processCommit(title, body, googleapisDir string) (string, string, error) {
	var newPRTitle string
	var commitTitle string
	var commitTitleIndex int

	bodySlice := strings.Split(body, "\n")
	for index, line := range bodySlice {
		if strings.Contains(line, "[REPLACEME]") {
			commitTitle = line
			commitTitleIndex = index
			continue
		}
		if !strings.Contains(line, "googleapis/googleapis/") {
			continue
		}
		commitPkg, err := analyzeLineForScope(line, googleapisDir)
		if err != nil {
			return "", "", err
		}
		if commitPkg == "outOfScope" {
			commitPkg = ""
		}
		if newPRTitle == "" {
			newPRTitle = updateCommitTitle(title, commitPkg)
			continue
		}
		newCommitTitle := updateCommitTitle(commitTitle, commitPkg)
		bodySlice[commitTitleIndex] = newCommitTitle
		}

	body = strings.Join(bodySlice, "\n")

	return newPRTitle, body, nil
}

func (cc *clientConfig) getPR(ctx context.Context) (*github.PullRequest, error) {
	client := github.NewClient(nil)

	PRs, _, err := client.PullRequests.List(ctx, cc.githubUsername, "google-cloud-go", nil)
	if err != nil {
		return nil, err
	}

	PR, err := cc.findValidPR(ctx, PRs)
	if err != nil {
		return nil, err
	}

	return PR, nil
}

func (cc *clientConfig) findValidPR(ctx context.Context, PRs []*github.PullRequest) (*github.PullRequest, error) {
	var PR *github.PullRequest
	for _, thisPR := range PRs {
		if strings.Contains(*thisPR.Head.Label, cc.branchPrefix) {
			PR = thisPR
			return PR, nil
		}
	}
	return nil, errors.New("no PR found")
}

func getScopeFromGoogleapisCommitHash(commitHash, googleapisDir string) ([]string, error) {
	c := execv.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", commitHash)
	c.Dir = googleapisDir
	fileList, err := c.Output()
	if err != nil {
		return nil, err
	}
	files := string(fileList)
	files = filepath.Dir(files)
	filesSlice := strings.Split(string(files), "\n")

	scopesMap := make(map[string]bool)
	scopes := []string{}
	for _, filePath := range filesSlice {
		for _, config := range generator.MicrogenGapicConfigs {
			if config.InputDirectoryPath == filePath {
				scope := config.Pkg
				if _, value := scopesMap[scope]; !value {
					scopesMap[scope] = true
					scopes = append(scopes, scope)
				}
				break
			}
		}
	}

	return scopes, nil
}

func extractHashFromLine(line string) string {
	pattern := regexp.MustCompile(`.*/(?P<hash>.*)`)
	hash := fmt.Sprintf("${%s}", pattern.SubexpNames()[1])
	hashVal := pattern.ReplaceAllString(line, hash)

	return hashVal
}

func updateCommitTitle(title, titlePkg string) string {
	var newTitle string

	pattern1 := regexp.MustCompile(`(?P<titleFirstPart>)(\: *\[)(.*)`)
	// a more general regex expression for pattern2 would be `.*\] *(?P<titleSecondPart>.*)`
	// but for readability and prevent removal of potentially relevant info the below may be preferable
	pattern2 := regexp.MustCompile(`.*\: *\[REPLACEME\] *(?P<titleSecondPart>.*)`)

	titleFirstPart := fmt.Sprintf("${%s}", pattern1.SubexpNames()[1])
	titleSecondPart := fmt.Sprintf("${%s}", pattern2.SubexpNames()[1])

	firstTitlePart := pattern1.ReplaceAllString(title, titleFirstPart)
	secondTitlePart := pattern2.ReplaceAllString(title, titleSecondPart)

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

func analyzeLineForScope(line, googleapisDir string) (string, error) {
	var commitPkg string

	hash := extractHashFromLine(line)
	pkgSlice, err := getScopeFromGoogleapisCommitHash(hash, googleapisDir)
	if err != nil {
		return "", err
	}
	if len(pkgSlice) == 0 {
		return "outOfScope", nil
	}
	commitPkg = pkgSlice[0]
	if len(pkgSlice) > 1 {
		for _, pkg := range pkgSlice[1:] {
			if pkg != commitPkg {
				commitPkg = "many"
			}
		}
	}
	return commitPkg, nil
}

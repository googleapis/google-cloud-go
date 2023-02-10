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
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/internal/gapicgen/generator"
	"cloud.google.com/go/internal/gapicgen/git"
	"cloud.google.com/go/internal/gensnippets"
	"cloud.google.com/go/internal/postprocessor/execv"
	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"github.com/google/go-github/v35/github"

	"gopkg.in/yaml.v2"
)

const (
	owlBotBranchPrefix         = "owl-bot-copy"
	beginNestedCommitDelimiter = "BEGIN_NESTED_COMMIT"
	endNestedCommitDelimiter   = "END_NESTED_COMMIT"
	copyTagSubstring           = "Copy-Tag:"
)

var (
	// hashFromLinePattern grabs the hash from the end of a github commit URL
	hashFromLinePattern = regexp.MustCompile(`.*/(?P<hash>[a-zA-Z0-9]*).*`)
)

var (
	//go:embed _README.md.txt
	readmeTmpl string
	//go:embed _version.go.txt
	versionTmpl string
	//go:embed _internal_version.go.txt
	internalVersionTmpl string
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
	log.Println("directories are", *directories)

	dirSlice := []string{}
	if *directories != "" {
		dirSlice := strings.Split(*directories, ",")
		log.Println("Postprocessor running on", dirSlice)
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
		modules:        dirSlice,
		branchOverride: *branchOverride,
		githubUsername: *githubUsername,
		prFilepath:     *prFilepath,
		runAll:         runAll,
		prTitle:        "",
		prBody:         "",
	}

	if err := c.run(ctx); err != nil {
		log.Fatal(err)
	}

	log.Println("Completed successfully.")
}

type config struct {
	googleapisDir  string
	googleCloudDir string

	// At this time modules are either provided at the time of invocation locally
	// and extracted from the open OwlBot PR description. If we would like
	// the postprocessor to be able to be run on non-OwlBot PRs, we would
	// need to change the method of populating this field.
	modules []string

	branchOverride string
	githubUsername string
	prFilepath     string
	runAll         bool
	prTitle        string
	prBody         string
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
	manifest, err := c.Manifest(generator.MicrogenGapicConfigs)
	if err != nil {
		return err
	}
	if err := c.InitializeNewModules(manifest); err != nil {
		return err
	}
	if err := c.SetScopesAndPRInfo(ctx); err != nil {
		return err
	}

	if err := c.TidyAffectedMods(); err != nil {
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
	if err := c.WritePRInfoToFile(); err != nil {
		return err
	}
	return nil
}

// InitializeNewModule detects new modules and clients and generates the required minimum files
// For modules, the minimum required files are internal/version.go, README.md, CHANGES.md, and go.mod
// For clients, the minimum required files are a version.go file
func (c *config) InitializeNewModules(manifest map[string]generator.ManifestEntry) error {
	log.Println("checking for new modules and clients")
	for _, moduleName := range moduleConfigs {
		modulePath := filepath.Join(c.googleCloudDir, moduleName)
		importPath := filepath.Join("cloud.google.com/go", moduleName)

		pathToModVersionFile := filepath.Join(modulePath, "internal/version.go")
		// Check if <module>/internal/version.go file exists
		if _, err := os.Stat(pathToModVersionFile); errors.Is(err, fs.ErrNotExist) {
			var serviceImportPath string
			for _, conf := range generator.MicrogenGapicConfigs {
				if strings.Contains(conf.ImportPath, importPath) {
					serviceImportPath = conf.ImportPath
					break
				}
			}
			if serviceImportPath == "" {
				return fmt.Errorf("no corresponding config found for module %s. Cannot generate min required files", moduleName)
			}
			// serviceImportPath here should be a valid ImportPath from a MicrogenGapicConfigs
			apiName := manifest[serviceImportPath].Description
			if err := c.generateMinReqFilesNewMod(moduleName, modulePath, importPath, apiName); err != nil {
				return err
			}
		}
		// Check if version.go files exist for each client
		filepath.WalkDir(modulePath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			splitPath := strings.Split(path, "/")
			lastElement := splitPath[len(splitPath)-1]
			if !strings.Contains(lastElement, "apiv") {
				return nil
			}
			pathToClientVersionFile := filepath.Join(path, "version.go")
			if _, err = os.Stat(pathToClientVersionFile); errors.Is(err, fs.ErrNotExist) {
				log.Println("generating version.go file in", path)
				if err := c.generateVersionFile(moduleName, path); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return nil
}

func (c *config) generateMinReqFilesNewMod(moduleName, modulePath, importPath, apiName string) error {
	log.Println("generating files for new module", apiName)
	if err := generateReadmeAndChanges(modulePath, importPath, apiName); err != nil {
		return err
	}
	if err := c.generateInternalVersionFile(moduleName); err != nil {
		return err
	}
	if err := c.generateModule(modulePath, importPath); err != nil {
		return err
	}
	return nil
}

func (c *config) generateModule(modPath, importPath string) error {
	if err := os.MkdirAll(modPath, os.ModePerm); err != nil {
		return err
	}
	log.Printf("Creating %s/go.mod", modPath)
	return gocmd.ModInit(modPath, importPath)
}

func (c *config) generateVersionFile(moduleName, modulePath string) error {
	// These directories are not modules on purpose, don't generate a version
	// file for them.
	if strings.Contains(modulePath, "debugger/apiv2") {
		return nil
	}
	rootPackage := filepath.Dir(modulePath)
	rootModInternal := fmt.Sprintf("cloud.google.com/go/%s/internal", rootPackage)

	f, err := os.Create(filepath.Join(modulePath, "version.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	t := template.Must(template.New("version").Parse(versionTmpl))
	versionData := struct {
		Year               int
		Package            string
		ModuleRootInternal string
	}{
		Year:               time.Now().Year(),
		Package:            moduleName,
		ModuleRootInternal: rootModInternal,
	}
	if err := t.Execute(f, versionData); err != nil {
		return err
	}
	return nil
}

func (c *config) generateInternalVersionFile(apiName string) error {
	rootModInternal := filepath.Join(apiName, "internal")
	os.MkdirAll(filepath.Join(c.googleCloudDir, rootModInternal), os.ModePerm)

	f, err := os.Create(filepath.Join(c.googleCloudDir, rootModInternal, "version.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	t := template.Must(template.New("internal_version").Parse(internalVersionTmpl))
	internalVersionData := struct {
		Year int
	}{
		Year: time.Now().Year(),
	}
	if err := t.Execute(f, internalVersionData); err != nil {
		return err
	}
	return nil
}

func (c *config) getDirs() []string {
	dirs := []string{}
	for _, module := range c.modules {
		dirs = append(dirs, filepath.Join(c.googleCloudDir, module))
	}
	return dirs
}

func (c *config) TidyAffectedMods() error {
	dirs := c.getDirs()
	for _, dir := range dirs {
		if err := gocmd.ModTidy(dir); err != nil {
			return err
		}
	}
	return nil
}

// Copied from generator package
func generateReadmeAndChanges(path, importPath, apiName string) error {
	readmePath := filepath.Join(path, "README.md")
	log.Printf("Creating %q", readmePath)
	readmeFile, err := os.Create(readmePath)
	if err != nil {
		return err
	}
	defer readmeFile.Close()
	t := template.Must(template.New("readme").Parse(readmeTmpl))
	readmeData := struct {
		Name       string
		ImportPath string
	}{
		Name:       apiName,
		ImportPath: importPath,
	}
	if err := t.Execute(readmeFile, readmeData); err != nil {
		return err
	}

	changesPath := filepath.Join(path, "CHANGES.md")
	log.Printf("Creating %q", changesPath)
	changesFile, err := os.Create(changesPath)
	if err != nil {
		return err
	}
	defer changesFile.Close()
	_, err = changesFile.WriteString("# Changes\n")
	return err
}

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (c *config) RegenSnippets() error {
	log.Println("regenerating snippets")
	snippetDir := filepath.Join(c.googleCloudDir, "internal", "generated", "snippets")
	confs := c.getChangedClientConfs()
	apiShortnames, err := generator.ParseAPIShortnames(c.googleapisDir, confs, generator.ManualEntries)
	if err != nil {
		return err
	}
	dirs := c.getDirs()
	if err := gensnippets.GenerateSnippetsDirs(c.googleCloudDir, snippetDir, apiShortnames, dirs); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := c.replaceAllForSnippets(snippetDir); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}

	return nil
}

// getChangedClientConfs iterates through the MicrogenGapicConfigs and returns
// a slice of the entries corresponding to modified modules and clients
func (c *config) getChangedClientConfs() []*generator.MicrogenConfig {
	if len(c.modules) != 0 {
		runConfs := []*generator.MicrogenConfig{}
		for _, conf := range generator.MicrogenGapicConfigs {
			for _, scope := range c.modules {
				scopePathElement := "/" + scope + "/"
				if strings.Contains(conf.InputDirectoryPath, scopePathElement) {
					runConfs = append(runConfs, conf)
				}
			}
		}
		return runConfs
	}
	return generator.MicrogenGapicConfigs
}

func (c *config) replaceAllForSnippets(snippetDir string) error {
	return execv.ForEachMod(c.googleCloudDir, func(dir string) error {
		processMod := false
		if c.modules != nil {
			// Checking each path component in its entirety prevents mistaken addition of modules whose names
			// contain the scope as a substring. For example if the scope is "video" we do not want to regenerate
			// snippets for "videointelligence"
			dirSlice := strings.Split(dir, "/")
			for _, mod := range c.modules {
				for _, dirElem := range dirSlice {
					if mod == dirElem {
						processMod = true
						break
					}
				}
			}
		}
		if !processMod {
			return nil
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

func (c *config) SetScopesAndPRInfo(ctx context.Context) error {
	log.Println("Amending PR title and body")
	pr, err := c.getPR(ctx)
	if err != nil {
		return err
	}
	newPRTitle, newPRBody, err := c.processCommit(*pr.Title, *pr.Body)
	if err != nil {
		return err
	}
	c.prTitle = newPRTitle
	c.prBody = newPRBody
	return nil
}

func contains(s []string, str string) bool {
	for _, elem := range s {
		if elem == str {
			return true
		}
	}
	return false
}

func (c *config) processCommit(title, body string) (string, string, error) {
	var newPRTitle string
	var newPRBodySlice []string
	var commitsSlice []string
	startCommitIndex := 0

	bodySlice := strings.Split(body, "\n")

	// Split body into separate commits, stripping nested commit delimiters
	for index, line := range bodySlice {
		if strings.Contains(line, beginNestedCommitDelimiter) || strings.Contains(line, endNestedCommitDelimiter) {
			startCommitIndex = index + 1
		}
		if strings.Contains(line, copyTagSubstring) {
			thisCommit := strings.Join(bodySlice[startCommitIndex:index+1], "\n")
			commitsSlice = append(commitsSlice, thisCommit)
			startCommitIndex = index + 1
		}
	}

	// Add scope to each commit
	for commitIndex, commit := range commitsSlice {
		commitLines := strings.Split(commit, "\n")
		var currTitle string
		if commitIndex == 0 {
			currTitle = title
		} else {
			currTitle = commitLines[0]
			commitLines = commitLines[1:]
			newPRBodySlice = append(newPRBodySlice, "")
			newPRBodySlice = append(newPRBodySlice, beginNestedCommitDelimiter)
		}
		for _, line := range commitLines {
			// When OwlBot generates the commit body, after commit titles it provides 'Source-Link's.
			// The source-link pointing to the googleapis/googleapis repo commit allows us to extract
			// hash and find files changed in order to identify the commit's scope.
			if strings.Contains(line, "googleapis/googleapis/") {
				hash := extractHashFromLine(line)
				scopes, err := c.getScopesFromGoogleapisCommitHash(hash)
				for _, scope := range scopes {
					if !contains(c.modules, scope) {
						c.modules = append(c.modules, scope)
					}
				}
				var scope string
				if len(scopes) == 1 {
					scope = scopes[0]
				}
				if err != nil {
					return "", "", err
				}

				newCommitTitle := updateCommitTitle(currTitle, scope)
				if newPRTitle == "" {
					newPRTitle = newCommitTitle
				} else {
					newPRBodySlice = append(newPRBodySlice, newCommitTitle)
				}

				newPRBodySlice = append(newPRBodySlice, commitLines...)
				if commitIndex != 0 {
					newPRBodySlice = append(newPRBodySlice, endNestedCommitDelimiter)
				}
			}
		}
	}
	if c.branchOverride != "" {
		c.modules = []string{}
		c.modules = append(c.modules, moduleConfigs...)
	}
	newPRBody := strings.Join(newPRBodySlice, "\n")
	return newPRTitle, newPRBody, nil
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

func (c *config) getScopesFromGoogleapisCommitHash(commitHash string) ([]string, error) {
	files, err := c.filesChanged(commitHash)
	if err != nil {
		return nil, err
	}
	// if no files changed, return empty string
	if len(files) == 0 {
		return nil, nil
	}
	scopesMap := make(map[string]bool)
	scopes := []string{}
	for _, filePath := range files {
		for _, config := range generator.MicrogenGapicConfigs {
			if config.InputDirectoryPath == filepath.Dir(filePath) {
				// trim prefix
				scope := strings.TrimPrefix(config.ImportPath, "cloud.google.com/go/")
				// trim version
				scope = filepath.Dir(scope)
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
	var breakChangeIndicator string

	titleSlice := strings.Split(title, ":")
	firstTitlePart := titleSlice[0]
	secondTitlePart := strings.TrimSpace(titleSlice[1])

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

// WritePRInfoToFile uses OwlBot env variable specified path to write updated
// PR title and body at that location
func (c *config) WritePRInfoToFile() error {
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
	if c.prTitle == "" && c.prBody == "" {
		log.Println("No updated PR info found, will not write PR title and description to file.")
		return nil
	}
	log.Println("Writing PR title and description to file.")
	if _, err := f.WriteString(fmt.Sprintf("%s\n\n%s", c.prTitle, c.prBody)); err != nil {
		return err
	}
	return nil
}

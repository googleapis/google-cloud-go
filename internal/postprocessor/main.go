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
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"github.com/google/go-github/v59/github"
)

const (
	owlBotBranchPrefix         = "owl-bot-copy"
	beginNestedCommitDelimiter = "BEGIN_NESTED_COMMIT"
	endNestedCommitDelimiter   = "END_NESTED_COMMIT"
	copyTagSubstring           = "Copy-Tag:"

	// This is the default Go version that will be generated into new go.mod
	// files. It should be updated every time we drop support for old Go
	// versions.
	defaultGoModuleVersion = "1.19"
)

var (
	// hashFromLinePattern grabs the hash from the end of a github commit URL
	hashFromLinePattern = regexp.MustCompile(`.*/(?P<hash>[a-zA-Z0-9]*).*`)

	// conventionalCommitTypes to look out for in multi-line commit blocks.
	// Pulled from: https://github.com/googleapis/release-please/blob/656b9a9ad1ec77853d16ae1f40e63c4da1e12f0f/src/strategies/go-yoshi.ts#L25-L37
	conventionalCommitTypes = map[string]bool{
		"feat":     true,
		"fix":      true,
		"perf":     true,
		"revert":   true,
		"docs":     true,
		"style":    true,
		"chore":    true,
		"refactor": true,
		"test":     true,
		"build":    true,
		"ci":       true,
	}
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

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "validate":
			log.Println("Starting config validation.")
			if err := validate(); err != nil {
				log.Fatal(err)
			}
			log.Println("Validation complete.")
			return
		}
	}
	flag.Parse()
	ctx := context.Background()

	log.Println("client-root set to", *clientRoot)
	log.Println("googleapis-dir set to", *googleapisDir)
	log.Println("branch set to", *branchOverride)
	log.Println("prFilepath is", *prFilepath)
	log.Println("directories are", *directories)

	dirSlice := []string{}
	if *directories != "" {
		dirSlice = strings.Split(*directories, ",")
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

		if err := DeepClone("https://github.com/googleapis/googleapis", *googleapisDir); err != nil {
			log.Fatal(err)
		}
	}

	p := &postProcessor{
		googleapisDir:  *googleapisDir,
		googleCloudDir: *clientRoot,
		modules:        dirSlice,
		branchOverride: *branchOverride,
		githubUsername: *githubUsername,
		prFilepath:     *prFilepath,
	}

	if err := p.loadConfig(); err != nil {
		log.Fatal(err)
	}

	if err := p.run(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("Completed successfully.")
}

type postProcessor struct {
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

	config *config
}

func (p *postProcessor) run(ctx context.Context) error {
	if runAll, err := runAll(p.googleCloudDir, p.branchOverride); err != nil {
		return err
	} else if !runAll {
		log.Println("exiting post processing early")
		return nil
	}

	manifest, err := p.Manifest()
	if err != nil {
		return err
	}
	if err := p.InitializeNewModules(manifest); err != nil {
		return err
	}
	if err := p.UpdateSnippetsMetadata(); err != nil {
		return err
	}
	prTitle, prBody, err := p.GetNewPRTitleAndBody(ctx)
	if err != nil {
		return err
	}
	if err := p.TidyAffectedMods(); err != nil {
		return err
	}
	if err := p.UpdateReleaseFiles(); err != nil {
		return err
	}
	if err := gocmd.Vet(p.googleCloudDir); err != nil {
		return err
	}
	if err := p.WritePRInfoToFile(prTitle, prBody); err != nil {
		return err
	}
	return nil
}

// InitializeNewModule detects new modules and clients and generates the required minimum files
// For modules, the minimum required files are internal/version.go, README.md, CHANGES.md, and go.mod
// For clients, the minimum required files are a version.go file
func (p *postProcessor) InitializeNewModules(manifest map[string]ManifestEntry) error {
	log.Println("checking for new modules and clients")
	for _, moduleName := range p.config.Modules {
		modulePath := filepath.Join(p.googleCloudDir, moduleName)
		importPath := filepath.Join("cloud.google.com/go", moduleName)

		pathToModVersionFile := filepath.Join(modulePath, "internal/version.go")
		// Check if <module>/internal/version.go file exists
		if _, err := os.Stat(pathToModVersionFile); errors.Is(err, fs.ErrNotExist) {
			log.Println("detected missing file: ", pathToModVersionFile)
			var serviceImportPath string
			for _, v := range p.config.GapicImportPaths() {
				if strings.Contains(v, importPath) {
					serviceImportPath = v
					break
				}
			}
			if serviceImportPath == "" {
				return fmt.Errorf("no config found for module %s. Cannot generate min required files", importPath)
			}
			// serviceImportPath here should be a valid ImportPath from a MicrogenGapicConfigs
			apiName := manifest[serviceImportPath].Description
			if err := p.generateMinReqFilesNewMod(moduleName, modulePath, importPath, apiName); err != nil {
				return err
			}
			log.Printf("Adding new module %s to list of modules to process", moduleName)
			p.modules = append(p.modules, moduleName)
			if err := p.modEditReplaceInSnippets(modulePath, importPath); err != nil {
				return err
			}
		}
		// Check if version.go files exist for each client
		err := filepath.WalkDir(modulePath, func(path string, d fs.DirEntry, err error) error {
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
			// Skip unless the presence of doc.go indicates that this is a client.
			// Some modules contain only type protos, and don't need version.go.
			pathToClientDocFile := filepath.Join(path, "doc.go")
			if _, err = os.Stat(pathToClientDocFile); errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			pathToClientVersionFile := filepath.Join(path, "version.go")
			if _, err = os.Stat(pathToClientVersionFile); errors.Is(err, fs.ErrNotExist) {
				if err := p.generateVersionFile(moduleName, path); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *postProcessor) generateMinReqFilesNewMod(moduleName, modulePath, importPath, apiName string) error {
	log.Println("generating files for new module", apiName)
	if err := generateReadmeAndChanges(modulePath, importPath, apiName); err != nil {
		return err
	}
	if err := p.generateInternalVersionFile(moduleName); err != nil {
		return err
	}
	if err := p.generateModule(modulePath, importPath); err != nil {
		return err
	}
	return nil
}

func (p *postProcessor) generateModule(modPath, importPath string) error {
	if err := os.MkdirAll(modPath, os.ModePerm); err != nil {
		return err
	}
	log.Printf("Creating %s/go.mod", modPath)
	if err := gocmd.ModInit(modPath, importPath, defaultGoModuleVersion); err != nil {
		return err
	}
	log.Print("Updating workspace")
	return gocmd.WorkUse(p.googleCloudDir)
}

func (p *postProcessor) generateVersionFile(moduleName, path string) error {
	// These directories are not modules on purpose, don't generate a version
	// file for them.
	if strings.Contains(path, "debugger/apiv2") || strings.Contains(path, "orgpolicy/apiv1") {
		return nil
	}
	log.Println("generating version.go file in", path)
	pathSegments := strings.Split(filepath.Dir(path), "/")

	rootModInternal := fmt.Sprintf("cloud.google.com/go/%s/internal", moduleName)

	f, err := os.Create(filepath.Join(path, "version.go"))
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
		Package:            pathSegments[len(pathSegments)-1],
		ModuleRootInternal: rootModInternal,
	}
	if err := t.Execute(f, versionData); err != nil {
		return err
	}
	return nil
}

func (p *postProcessor) generateInternalVersionFile(apiName string) error {
	rootModInternal := filepath.Join(apiName, "internal")
	os.MkdirAll(filepath.Join(p.googleCloudDir, rootModInternal), os.ModePerm)

	f, err := os.Create(filepath.Join(p.googleCloudDir, rootModInternal, "version.go"))
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

func (p *postProcessor) getDirs() []string {
	dirs := []string{}
	for _, module := range p.modules {
		dirs = append(dirs, filepath.Join(p.googleCloudDir, module))
	}
	return dirs
}

func (p *postProcessor) modEditReplaceInSnippets(modulePath, importPath string) error {
	// Replace it. Use a relative path to avoid issues on different systems.
	snippetsDir := filepath.Join(p.googleCloudDir, "internal", "generated", "snippets")
	rel, err := filepath.Rel(snippetsDir, modulePath)
	if err != nil {
		return err
	}
	return gocmd.EditReplace(snippetsDir, importPath, rel)
}

func (p *postProcessor) UpdateSnippetsMetadata() error {
	log.Println("updating snippets metadata")
	for _, clientRelPath := range p.config.ClientRelPaths {
		// OwlBot dest relative paths in ClientRelPaths begin with /, so the
		// first path segment is the second element.
		moduleName := strings.Split(clientRelPath, "/")[1]
		if moduleName == "" {
			return fmt.Errorf("unable to parse module name for %v", clientRelPath)
		}
		// Skip if dirs option set and this module is not included.
		if len(p.modules) > 0 && !contains(p.modules, moduleName) {
			continue
		}
		// debugger/apiv2 is not in a module so it does not have version info to read.
		if strings.Contains(clientRelPath, "debugger/apiv2") {
			continue
		}
		snpDir := filepath.Join(p.googleCloudDir, "internal", "generated", "snippets", clientRelPath)
		glob := filepath.Join(snpDir, "snippet_metadata.*.json")
		metadataFiles, err := filepath.Glob(glob)
		if err != nil {
			return err
		}
		if len(metadataFiles) == 0 {
			log.Println("skipping, file not found with glob: ", glob)
			continue
		}
		log.Println("updating ", glob)
		version, err := getModuleVersion(filepath.Join(p.googleCloudDir, moduleName))
		if err != nil {
			return err
		}
		read, err := os.ReadFile(metadataFiles[0])
		if err != nil {
			return err
		}
		if strings.Contains(string(read), "$VERSION") {
			log.Printf("setting $VERSION to %s in %s", version, metadataFiles[0])
			s := strings.Replace(string(read), "$VERSION", version, 1)
			err = os.WriteFile(metadataFiles[0], []byte(s), 0)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getModuleVersion(dir string) (string, error) {
	node, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, "internal", "version.go"), nil, parser.ParseComments)
	if err != nil {
		return "", err
	}
	version := node.Scope.Objects["Version"].Decl.(*ast.ValueSpec).Values[0].(*ast.BasicLit).Value
	version = strings.Trim(version, `"`)
	return version, nil
}

func (p *postProcessor) TidyAffectedMods() error {
	dirs := p.getDirs()
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

func (p *postProcessor) GetNewPRTitleAndBody(ctx context.Context) (string, string, error) {
	var prTitle, prBody string
	log.Println("Amending PR title and body")
	pr, err := p.getPR(ctx)
	if err != nil {
		return prTitle, prBody, err
	}
	newPRTitle, newPRBody, err := p.processCommit(*pr.Title, *pr.Body)
	if err != nil {
		return prTitle, prBody, err
	}
	return newPRTitle, newPRBody, nil
}

func contains(s []string, str string) bool {
	for _, elem := range s {
		if elem == str {
			return true
		}
	}
	return false
}

func (p *postProcessor) processCommit(title, body string) (string, string, error) {
	var newTitle string
	var newBody strings.Builder
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

	// Add scope to each commit and every nested commit therein.
	for commitIndex, commit := range commitsSlice {
		commitLines := strings.Split(strings.TrimSpace(commit), "\n")
		var currTitle string
		if commitIndex == 0 {
			currTitle = title
		} else {
			currTitle = commitLines[0]
			commitLines = commitLines[1:]
			newBody.WriteString(fmt.Sprintf("\n%v\n", beginNestedCommitDelimiter))
		}
		for _, line := range commitLines {
			// When OwlBot generates the commit body, after commit titles it provides 'Source-Link's.
			// The source-link pointing to the googleapis/googleapis repo commit allows us to extract
			// hash and find files changed in order to identify the commit's scope.
			if strings.HasPrefix(line, "Source-Link") && strings.Contains(line, "googleapis/googleapis/") {
				hash := extractHashFromLine(line)
				scopes, err := p.getScopesFromGoogleapisCommitHash(hash)
				if err != nil {
					return "", "", err
				}
				for _, scope := range scopes {
					if !contains(p.modules, scope) {
						p.modules = append(p.modules, scope)
					}
				}
				var scope string
				if len(scopes) == 1 {
					scope = scopes[0]
				}

				newCommitTitle := updateCommit(currTitle, scope)
				if newTitle == "" {
					newTitle = newCommitTitle
				} else {
					newBody.WriteString(fmt.Sprintf("%v\n", newCommitTitle))
				}

				for i, line := range commitLines {
					if !strings.Contains(line, ":") {
						// couldn't be a conventional commit line
						continue
					}
					commitType := line[:strings.Index(line, ":")]
					if strings.Contains(commitType, "(") {
						// if it has a scope, remove it - updateCommitTitle does
						// already, we want to force our own scope.
						commitType = commitType[:strings.Index(commitType, "(")]
					}

					// always trim any potential bang
					commitType = strings.TrimSuffix(commitType, "!")

					if _, ok := conventionalCommitTypes[commitType]; !ok {
						// not a known conventional commit type, ignore
						continue
					}
					commitLines[i] = updateCommit(line, scope)
				}
				newBody.WriteString(strings.Join(commitLines, "\n"))
				if commitIndex != 0 {
					newBody.WriteString(fmt.Sprintf("\n%v", endNestedCommitDelimiter))
				}
			}
		}
	}
	if p.branchOverride != "" {
		p.modules = []string{}
		p.modules = append(p.modules, p.config.Modules...)
	}
	return newTitle, newBody.String(), nil
}

func (p *postProcessor) getPR(ctx context.Context) (*github.PullRequest, error) {
	client := github.NewClient(nil)
	prs, _, err := client.PullRequests.List(ctx, p.githubUsername, "google-cloud-go", nil)
	if err != nil {
		return nil, err
	}
	var owlbotPR *github.PullRequest
	branch := p.branchOverride
	if p.branchOverride == "" {
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

func (p *postProcessor) getScopesFromGoogleapisCommitHash(commitHash string) ([]string, error) {
	files, err := filesChanged(p.googleapisDir, commitHash)
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
		// Need import path
		for inputDir, li := range p.config.GoogleapisToImportPath {
			if inputDir == filepath.Dir(filePath) {
				// trim service version
				scope := filepath.Dir(li.RelPath)
				// trim leading slash
				scope = strings.TrimPrefix(scope, "/")
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
	hash := fmt.Sprintf("${%s}", hashFromLinePattern.SubexpNames()[1])
	hashVal := hashFromLinePattern.ReplaceAllString(line, hash)
	return hashVal
}

func updateCommit(title, titlePkg string) string {
	var breakChangeIndicator string
	titleParts := strings.Split(title, ":")
	commitPrefix := titleParts[0]
	msg := strings.TrimSpace(titleParts[1])

	// If a scope is already provided, remove it.
	if i := strings.Index(commitPrefix, "("); i > 0 {
		commitPrefix = commitPrefix[:i]
	}
	if strings.HasSuffix(commitPrefix, "!") {
		breakChangeIndicator = "!"
		// trim it so we don't dupe it, but put it back in the right place
		commitPrefix = strings.TrimSuffix(commitPrefix, "!")
	}
	if titlePkg == "" {
		return fmt.Sprintf("%v%v: %v", commitPrefix, breakChangeIndicator, msg)
	}
	return fmt.Sprintf("%v(%v)%v: %v", commitPrefix, titlePkg, breakChangeIndicator, msg)
}

// WritePRInfoToFile uses OwlBot env variable specified path to write updated
// PR title and body at that location
func (p *postProcessor) WritePRInfoToFile(prTitle, prBody string) error {
	if prTitle == "" && prBody == "" {
		log.Println("No updated PR info found, will not write PR title and description to file.")
		return nil
	}
	// if file exists at location, delete
	if err := os.Remove(p.prFilepath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Println(err)
		} else {
			return err
		}
	}
	f, err := os.OpenFile(p.prFilepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	log.Println("Writing PR title and description to file.")
	if _, err := f.WriteString(fmt.Sprintf("%s\n\n%s", prTitle, prBody)); err != nil {
		return err
	}
	return nil
}

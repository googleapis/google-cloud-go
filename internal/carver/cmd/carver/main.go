// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

const (
	marjorWeight = 1e9
	minorWeight  = 1e4
	patchWeight  = 1
)

var (
	semverRegex = regexp.MustCompile(`.*v(?P<major>\d*)\.(?P<minor>\d*)\.(?P<patch>\d*)(?P<suffix>.*)`)
	//go:embed _tidyhack_tmpl.txt
	tidyHackTmpl string
	//go:embed _CHANGES.md.txt
	changesTmpl string
	//go:embed _README.md.txt
	readmeTmpl string
)

type carver struct {
	rootMod  *modInfo
	childMod *modInfo

	// flags
	repoMetadataPath string
	name             string
	dryRun           bool

	w io.WriteCloser
}

var once sync.Once

func main() {
	parent := flag.String("parent", "", "The path to the parent module. Required.")
	child := flag.String("child", "", "The relative path to the child module from the parent module. Required.")
	repoMetadataPath := flag.String("repo-metadata", "", "The full path to the repo metadata file. Required.")
	name := flag.String("name", "", "The name used to identify the API in the README. Optional")
	parentTagPrefix := flag.String("parent-tag-prefix", "", "The prefix for a git tag, should end in a '/'. Only required if parent is not the root module. Optional.")
	parentTag := flag.String("parent-tag", "", "The newest tag from the parent module, this will override the lookup. If not specified the latest tag will be used. Optional.")
	childTagVersion := flag.String("child-tag-version", "v0.1.0", "The tag version of the carved out child module. Should be in the form of vX.X.X with no prefix. Optional.")
	dryRun := flag.Bool("dry-run", false, "If true no files or tags will be created. Optional.")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nExample\n\tcarver -parent=/Users/me/google-cloud-go -child=asset repo-metadata=/Users/me/google-cloud-go/internal/.repo-metadata-full.json\n")
	}
	flag.Parse()
	rootMod, err := rootModInfo(*parent, *parentTagPrefix, *parentTag)
	if err != nil {
		log.Fatalf("unable to calculate root mod info: %v", err)
	}
	children := strings.Split(strings.TrimSpace(*child), ",")
	var tags []string
	for _, child := range children {
		child := strings.TrimSpace(child)
		childMod := childModInfo(rootMod, child, *childTagVersion)
		c := &carver{
			rootMod:          rootMod,
			childMod:         childMod,
			repoMetadataPath: *repoMetadataPath,
			name:             *name,
			dryRun:           *dryRun,
		}
		ts, err := c.Run()
		if err != nil {
			log.Println(err)
			flag.Usage()
			os.Exit(1)
		}
		tags = append(tags, ts...)
	}
	if err := gitCommit(rootMod.path, tags, *dryRun); err != nil {
		log.Fatal(err)
	}

	log.Println("Successfully carved out modules. Please run the following commands after your changes are merged:")
	for _, tag := range tags {
		fmt.Fprintf(os.Stdout, "git tag %s $COMMIT_SHA\n", tag)
	}
	for _, tag := range tags {
		fmt.Fprintf(os.Stdout, "git push origin refs/tags/%s\n", tag)
	}
	fmt.Fprintf(os.Stdout, "Once tags have propagated open a new PR tidying the new child mods go.sum entries.\n")
}

func (c *carver) Run() ([]string, error) {
	if c.rootMod.path == "" || c.childMod.path == "" || c.repoMetadataPath == "" {
		return nil, fmt.Errorf("all required flags were not provided")
	}
	if err := c.CreateChildCommonFiles(); err != nil {
		return nil, fmt.Errorf("failed to create readme: %v", err)
	}
	if err := c.CreateChildModule(); err != nil {
		return nil, fmt.Errorf("failed to create child module: %v", err)
	}
	if err := c.FixupSnippets(); err != nil {
		return nil, fmt.Errorf("failed to update snippet module: %v", err)
	}

	var tags []string
	once.Do(func() {
		tags = append(tags, c.rootMod.Tag())
	})
	tags = append(tags, c.childMod.Tag())
	return tags, nil
}

type modInfo struct {
	// path is the filepath to the module locally.
	path string
	// importPath is the import path of the module.
	importPath string
	// futureTagVersion is the tag version that this module should be tagged
	// after all operations are preformed.
	futureTagVersion string
	// tagPrefix is the prefix of the module used while tagging.
	tagPrefix string
}

// Tag returns a formatted git tag for a module.
func (mi *modInfo) Tag() string {
	if mi.tagPrefix == "" {
		return mi.futureTagVersion
	}
	return fmt.Sprintf("%s/%s", mi.tagPrefix, mi.futureTagVersion)
}

func (mi *modInfo) PkgName() string {
	ss := strings.Split(mi.importPath, "/")
	return ss[len(ss)-1]
}

func rootModInfo(rootModPath, rootTagPrefix, rootTagVersion string) (*modInfo, error) {
	log.Println("Looking up parent module import path")
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}")
	cmd.Dir = rootModPath
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	modName := string(bytes.TrimSpace(b))

	if rootTagVersion != "" {
		tag, err := bumpSemverPatch(rootTagVersion)
		if err != nil {
			return nil, err
		}
		return &modInfo{
			path:             rootModPath,
			importPath:       modName,
			futureTagVersion: tag,
		}, nil
	}

	log.Println("Looking up latest parent tag")
	cmd = exec.Command("git", "tag")
	cmd.Dir = rootModPath
	b, err = cmd.Output()
	if err != nil {
		return nil, err
	}

	var relevantTags []string
	for _, tag := range strings.Split(string(bytes.TrimSpace(b)), "\n") {
		if rootTagPrefix != "" && strings.HasPrefix(tag, rootTagPrefix) {
			relevantTags = append(relevantTags, tag)
			continue
		}
		if rootTagPrefix == "" && !strings.Contains(tag, "/") && strings.HasPrefix(tag, "v") {
			relevantTags = append(relevantTags, tag)
		}
	}
	sortTags(relevantTags)
	tag := relevantTags[0]
	log.Println("Found latest tag: ", tag)

	futureTag, err := bumpSemverPatch(tag)
	if err != nil {
		return nil, err
	}
	return &modInfo{
		path:             rootModPath,
		importPath:       modName,
		futureTagVersion: futureTag,
	}, nil
}

func childModInfo(rootMod *modInfo, childRelPath, childTagVersion string) *modInfo {
	return &modInfo{
		path:             filepath.Join(rootMod.path, childRelPath),
		importPath:       filepath.Join(rootMod.importPath, childRelPath),
		futureTagVersion: childTagVersion,
		tagPrefix:        childRelPath,
	}
}

func (c *carver) CreateChildCommonFiles() error {
	log.Printf("Reading metadata file from %q", c.repoMetadataPath)
	metaFile, err := os.Open(c.repoMetadataPath)
	if err != nil {
		return fmt.Errorf("unable to open metadata file: %v", err)
	}
	meta, err := parseMetadata(metaFile)
	if err != nil {
		return err
	}

	readmePath := filepath.Join(c.childMod.path, "README.md")
	log.Printf("Creating %q", readmePath)
	readmeFile, err := c.newWriterCloser(readmePath)
	if err != nil {
		return err
	}
	defer readmeFile.Close()
	t := template.Must(template.New("readme").Parse(readmeTmpl))
	name := c.name
	if name == "" {
		name = meta[c.childMod.importPath]
		if name == "" {
			return fmt.Errorf("unable to determine a name from API metadata, please set -name flag")
		}
	}
	readmeData := struct {
		Name       string
		ImportPath string
	}{
		Name:       name,
		ImportPath: c.childMod.importPath,
	}
	if err := t.Execute(readmeFile, readmeData); err != nil {
		return err
	}

	changesPath := filepath.Join(c.childMod.path, "CHANGES.md")
	log.Printf("Creating %q", changesPath)
	changesFile, err := c.newWriterCloser(changesPath)
	if err != nil {
		return err
	}
	defer changesFile.Close()
	t2 := template.Must(template.New("changes").Parse(changesTmpl))
	changesData := struct {
		Package string
	}{
		Package: c.childMod.PkgName(),
	}
	return t2.Execute(changesFile, changesData)
}

func (c *carver) CreateChildModule() error {
	fp := filepath.Join(c.childMod.path, "go_mod_tidy_hack.go")
	log.Printf("Creating %q", fp)
	f, err := c.newWriterCloser(fp)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("tidyhack").Parse(tidyHackTmpl))
	data := struct {
		Year    int
		RootMod string
		Package string
	}{
		Year:    time.Now().Year(),
		RootMod: c.rootMod.importPath,
		Package: c.childMod.PkgName(),
	}
	if err := t.Execute(f, data); err != nil {
		return err
	}

	log.Printf("Creating child module in %q", c.childMod.path)
	if c.dryRun {
		return nil
	}
	cmd := exec.Command("go", "mod", "init", c.childMod.importPath)
	cmd.Dir = c.childMod.path
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to init module: %s", b)
	}

	if err != nil {
		return err
	}
	cmd = exec.Command("go", "mod", "edit", "-require", fmt.Sprintf("%s@%s", c.rootMod.importPath, c.rootMod.futureTagVersion))
	cmd.Dir = c.childMod.path
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to require module: %s", b)
	}

	cmd = exec.Command("go", "mod", "edit", "-replace", fmt.Sprintf("%s@%s=%s", c.rootMod.importPath, c.rootMod.futureTagVersion, c.rootMod.path))
	cmd.Dir = c.childMod.path
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to add replace module: %s", b)
	}

	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = c.childMod.path
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to tidy child module: %s", b)
	}

	cmd = exec.Command("go", "mod", "edit", "-dropreplace", fmt.Sprintf("%s@%s", c.rootMod.importPath, c.rootMod.futureTagVersion))
	cmd.Dir = c.childMod.path
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to add replace module: %s", b)
	}

	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = c.rootMod.path
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to tidy parent module: %s", b)
	}
	return nil
}

func (c *carver) FixupSnippets() error {
	log.Println("Fixing snippets")
	snippetsModDir := filepath.Join(c.rootMod.path, "internal", "generated", "snippets")
	childRelPath := strings.TrimPrefix(c.childMod.path, c.rootMod.path)
	cmd := exec.Command("go", "mod", "edit", "-require", fmt.Sprintf("%s@%s", c.childMod.importPath, c.childMod.futureTagVersion))
	cmd.Dir = snippetsModDir
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to require module: %s", b)
	}

	cmd = exec.Command("go", "mod", "edit", "-replace", fmt.Sprintf("%s@%s=../../..%s", c.childMod.importPath, c.childMod.futureTagVersion, childRelPath))
	cmd.Dir = snippetsModDir
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to add replace module: %s", b)
	}

	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = snippetsModDir
	if b, err := cmd.Output(); err != nil {
		return fmt.Errorf("unable to tidy child module: %s", b)
	}
	return nil
}

func gitCommit(dir string, tags []string, dryRun bool) error {
	log.Println("Commiting changes")
	if !dryRun {
		cmd := exec.Command("git", "add", "-A")
		cmd.Dir = dir
		if b, err := cmd.Output(); err != nil {
			return fmt.Errorf("unable to add changes: %s", b)
		}
		var sb strings.Builder
		sb.WriteString("chore: carve out sub-modules\n\nThis commit will be tagged:\n")
		for _, tag := range tags {
			sb.WriteString(fmt.Sprintf("\t- %s\n", tag))
		}
		cmd = exec.Command("git", "commit", "-m", sb.String())
		cmd.Dir = dir
		if b, err := cmd.Output(); err != nil {
			return fmt.Errorf("unable to commit changes: %s", b)
		}
	}
	return nil
}

// newWriterCloser is wrapper for creating a file. Used for testing and
// dry-runs.
func (c *carver) newWriterCloser(fp string) (io.WriteCloser, error) {
	if c.dryRun {
		return noopCloser{w: io.Discard}, nil
	}
	if c.w != nil {
		return noopCloser{w: c.w}, nil
	}
	return os.Create(fp)
}

// sortTags does a best effort sort based on semver. It was made a function for
// testing. Only the top result will ever be used.
func sortTags(tags []string) {
	sort.Slice(tags, func(i, j int) bool {
		imatch := semverRegex.FindStringSubmatch(tags[i])
		jmatch := semverRegex.FindStringSubmatch(tags[j])
		if len(imatch) < 5 {
			return false
		}
		if len(jmatch) < 5 {
			return true
		}

		// Matches must be numbers due to regex they are parsed from.
		iM, _ := strconv.Atoi(imatch[1])
		jM, _ := strconv.Atoi(jmatch[1])
		im, _ := strconv.Atoi(imatch[2])
		jm, _ := strconv.Atoi(jmatch[2])
		ip, _ := strconv.Atoi(imatch[3])
		jp, _ := strconv.Atoi(jmatch[3])

		// weight each level of semver for comparison
		iTotal := iM*marjorWeight + im*minorWeight + ip*patchWeight
		jTotal := jM*marjorWeight + jm*minorWeight + jp*patchWeight

		// de-rank all prereleases by a major version
		if imatch[4] != "" {
			iTotal -= marjorWeight
		}
		if jmatch[4] != "" {
			jTotal -= marjorWeight
		}

		return iTotal > jTotal
	})
}

type noopCloser struct {
	w io.Writer
}

func (n noopCloser) Write(p []byte) (int, error) {
	return n.w.Write(p)
}

func (n noopCloser) Close() error { return nil }

func bumpSemverPatch(tag string) (string, error) {
	splitTag := semverRegex.FindStringSubmatch(tag)
	if len(splitTag) < 5 {
		return "", fmt.Errorf("invalid tag layout: %q", tag)
	}
	var maj, min, pat int
	var err error
	if maj, err = strconv.Atoi(splitTag[1]); err != nil {
		return "", fmt.Errorf("invalid tag layout: %q", tag)
	}
	if min, err = strconv.Atoi(splitTag[2]); err != nil {
		return "", fmt.Errorf("invalid tag layout: %q", tag)
	}
	if pat, err = strconv.Atoi(splitTag[3]); err != nil {
		return "", fmt.Errorf("invalid tag layout: %q", tag)
	}

	if strings.Contains(tag, "/") {
		splitTag := strings.Split(tag, "/")
		return fmt.Sprintf("%s/v%d.%d.%d", strings.Join(splitTag[:len(splitTag)-1], "/"), maj, min, pat+1), nil
	}
	return fmt.Sprintf("v%d.%d.%d", maj, min, pat+1), nil
}

// parseMetadata creates a mapping of potential modules to API full name.
func parseMetadata(r io.Reader) (map[string]string, error) {
	m := map[string]struct {
		Description string `json:"description"`
	}{}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	m2 := map[string]string{}
	for k, v := range m {
		k2 := k
		if i := strings.Index(k2, "/apiv"); i > 0 {
			k2 = k2[:i]
		}
		m2[k2] = v.Description
	}
	return m2, nil
}

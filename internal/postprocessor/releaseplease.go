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
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var unreleasedModuleDir map[string]bool = map[string]bool{
	"spanner/test/opentelemetry/test": true,
}

var individuallyReleasedModules map[string]bool = map[string]bool{
	".":                true,
	"ai":               true,
	"aiplatform":       true,
	"auth":             true,
	"auth/oauth2adapt": true,
	"bigquery":         true,
	"bigtable":         true,
	"datastore":        true,
	"errorreporting":   true,
	"firestore":        true,
	"logging":          true,
	"profiler":         true,
	"pubsub":           true,
	"pubsublite":       true,
	"spanner":          true,
	"storage":          true,
	"vertexai":         true,
}

var defaultReleasePleaseConfig = &releasePleaseConfig{
	ReleaseType:           "go-yoshi",
	IncludeComponentInTag: true,
	TagSeparator:          "/",
	Packages:              map[string]*releasePleasePackage{},
	Plugins:               []string{"sentence-case"},
}

type releasePleaseConfig struct {
	ReleaseType           string                           `json:"release-type"`
	IncludeComponentInTag bool                             `json:"include-component-in-tag"`
	TagSeparator          string                           `json:"tag-separator"`
	Packages              map[string]*releasePleasePackage `json:"packages"`
	Plugins               []string                         `json:"plugins"`
}

type releasePleasePackage struct {
	Component string `json:"component"`
}

// updateReleaseFiles reconciles release-please configure based of the state of
// the repo. It will auto-detect and add configure for new modules.
func (p *postProcessor) UpdateReleaseFiles() error {
	mods, err := detectModules(p.googleCloudDir)
	if err != nil {
		return err
	}
	sort.Strings(mods)

	f, err := os.OpenFile(filepath.Join(p.googleCloudDir, "release-please-config-yoshi-submodules.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := updateConfigFile(f, mods); err != nil {
		return err
	}

	fp := filepath.Join(p.googleCloudDir, ".release-please-manifest-submodules.json")
	b, err := os.ReadFile(fp)
	if err != nil {
		return err
	}
	f2, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f2.Close()
	if err := updateManifestFile(f2, b, mods); err != nil {
		return err
	}
	return nil
}

// updateConfigFile updates the release-please submodule config file.
func updateConfigFile(w io.Writer, mods []string) error {
	conf := *defaultReleasePleaseConfig
	for _, mod := range mods {
		conf.Packages[mod] = &releasePleasePackage{
			Component: mod,
		}
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	if err := e.Encode(&conf); err != nil {
		return err
	}
	return nil
}

// updateManifestFile updates the release-please submodule manifest file.
func updateManifestFile(w io.Writer, existingContents []byte, mods []string) error {
	manifest := map[string]string{}
	if err := json.Unmarshal(existingContents, &manifest); err != nil {
		return err
	}
	for _, mod := range mods {
		if _, ok := manifest[mod]; !ok {
			log.Printf("adding release please manifest entry for: %s", mod)
			manifest[mod] = "0.0.0"
		}
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	if err := e.Encode(&manifest); err != nil {
		return err
	}
	return nil
}

// detectModules returns a list of relative paths to module roots that are
// managed by release-please.
func detectModules(dir string) ([]string, error) {
	var mods []string
	fileSystem := os.DirFS(dir)
	err := fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		if !d.IsDir() && d.Name() == "go.mod" && !strings.Contains(path, "internal") && !individuallyReleasedModules[filepath.Dir(path)] && !unreleasedModuleDir[filepath.Dir(path)] {
			mods = append(mods, filepath.Dir(path))
		}
		return nil
	})
	return mods, err
}

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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	// Modules are all the modules roots the post processor should generate
	// template files for.
	Modules []string `yaml:"modules"`
	// ClientRelPaths are the relative paths to the client root directories in
	// google-cloud-go.
	ClientRelPaths []string
	// GoogleapisToImportPath is a map of a googleapis dir to the corresponding
	// gapic import path.
	GoogleapisToImportPath map[string]*libraryInfo
	// ManualClientInfo contains information on manual clients used to generate
	// the manifest file.
	ManualClientInfo []*ManifestEntry
}

type serviceConfigEntry struct {
	InputDirectory       string `yaml:"input-directory"`
	ServiceConfig        string `yaml:"service-config"`
	ImportPath           string `yaml:"import-path"`
	RelPath              string `yaml:"rel-path"`
	ReleaseLevelOverride string `yaml:"release-level-override"`
}

type postProcessorConfig struct {
	Modules        []string              `yaml:"modules"`
	ServiceConfigs []*serviceConfigEntry `yaml:"service-configs"`
	ManualClients  []*ManifestEntry      `yaml:"manual-clients"`
}

type deepCopyConfig struct {
	Source string `yaml:"source"`
	Dest   string `yaml:"dest"`
}
type owlBotConfig struct {
	DeepCopyRegex   []deepCopyConfig `yaml:"deep-copy-regex"`
	DeepRemoveRegex []string         `yaml:"deep-remove-regex"`
}

// libraryInfo contains information about a GAPIC client.
type libraryInfo struct {
	// ImportPath is the Go import path for the GAPIC library.
	ImportPath string
	// ServiceConfig is the relative directory to the service config from the
	// services directory in googleapis.
	ServiceConfig string
	// RelPath is the relative path to the client from the repo root.
	RelPath string
	// ReleaseLevel is an override for the release level of a library. It is
	// used in cases where a release level can't be determined by looking at
	// the import path and/or reading service `doc.go` files because there are
	// no associated services.
	ReleaseLevel string
}

func loadConfigs(ppcPath, obcPath string) (*postProcessorConfig, *owlBotConfig, error) {
	var ppc postProcessorConfig
	b, err := os.ReadFile(ppcPath)
	if err != nil {
		return nil, nil, err
	}
	if err := yaml.Unmarshal(b, &ppc); err != nil {
		return nil, nil, err
	}
	var obc owlBotConfig
	b2, err := os.ReadFile(obcPath)
	if err != nil {
		return nil, nil, err
	}
	if err := yaml.Unmarshal(b2, &obc); err != nil {
		return nil, nil, err
	}

	return &ppc, &obc, nil
}

func (p *postProcessor) loadConfig() error {
	ppcPath := filepath.Join(p.googleCloudDir, "internal", "postprocessor", "config.yaml")
	obcPath := filepath.Join(p.googleCloudDir, ".github", ".OwlBot.yaml")
	ppc, obc, err := loadConfigs(ppcPath, obcPath)
	if err != nil {
		return err
	}

	c := &config{
		Modules:                ppc.Modules,
		ClientRelPaths:         make([]string, 0),
		GoogleapisToImportPath: make(map[string]*libraryInfo),
		ManualClientInfo:       ppc.ManualClients,
	}
	for _, v := range ppc.ServiceConfigs {
		c.GoogleapisToImportPath[v.InputDirectory] = &libraryInfo{
			ServiceConfig: v.ServiceConfig,
			ImportPath:    v.ImportPath,
			RelPath:       v.RelPath,
			ReleaseLevel:  v.ReleaseLevelOverride,
		}
	}
	for _, v := range obc.DeepCopyRegex {
		i := strings.Index(v.Source, "/cloud.google.com/go")
		li, ok := c.GoogleapisToImportPath[v.Source[1:i]]
		if !ok {
			return fmt.Errorf("unable to find value for %q, it may be missing a service config entry", v.Source[1:i])
		}
		if li.ImportPath == "" {
			li.ImportPath = v.Source[i+1:]
		}
		if li.RelPath == "" {
			li.RelPath = strings.TrimPrefix(li.ImportPath, "cloud.google.com/go")
		}
		c.ClientRelPaths = append(c.ClientRelPaths, li.RelPath)
	}
	p.config = c
	return nil
}

func (c *config) GapicImportPaths() []string {
	var s []string
	for _, v := range c.GoogleapisToImportPath {
		s = append(s, v.ImportPath)
	}
	return s
}

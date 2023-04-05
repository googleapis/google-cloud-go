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
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/gensnippets"
	"cloud.google.com/go/internal/postprocessor/execv"
	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"gopkg.in/yaml.v3"
)

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (p *postProcessor) RegenSnippets() error {
	log.Println("regenerating snippets")
	snippetDir := filepath.Join(p.googleCloudDir, "internal", "generated", "snippets")
	confs := p.getChangedClientConfs()
	apiShortnames, err := p.parseAPIShortnames(confs)
	if err != nil {
		return err
	}
	dirs := p.getDirs()
	if err := gensnippets.GenerateSnippetsDirs(p.googleCloudDir, snippetDir, apiShortnames, dirs); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := p.replaceAllForSnippets(snippetDir); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}
	return nil
}

func (p *postProcessor) parseAPIShortnames(confs map[string]*libraryInfo) (map[string]string, error) {
	shortnames := map[string]string{}
	for inputDir, li := range p.config.GoogleapisToImportPath {
		// Issue: https://github.com/googleapis/google-cloud-go/issues/7357
		// Issue: https://github.com/googleapis/google-cloud-go/issues/7349
		// Issue: https://github.com/googleapis/google-cloud-go/issues/7352
		// Issue: https://github.com/googleapis/google-cloud-go/issues/7335
		if strings.Contains(li.ImportPath, "apigeeregistry/apiv1") ||
			strings.Contains(li.ImportPath, "dialogflow/apiv2beta1") ||
			strings.Contains(li.ImportPath, "asset/apiv1") ||
			strings.Contains(li.ImportPath, "oslogin/apiv1") {
			continue
		}
		if li.ServiceConfig == "" {
			continue
		}
		yamlPath := filepath.Join(p.googleapisDir, inputDir, li.ServiceConfig)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		config := struct {
			Name string `yaml:"name"`
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&config); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		shortname := strings.TrimSuffix(config.Name, ".googleapis.com")
		shortnames[li.ImportPath] = shortname
	}

	// Do our best for manuals.
	for _, manual := range p.config.ManualClientInfo {
		p := strings.TrimPrefix(manual.DistributionName, "cloud.google.com/go/")
		if strings.Contains(p, "/") {
			p = p[0:strings.Index(p, "/")]
		}
		shortnames[manual.DistributionName] = p
	}
	return shortnames, nil
}

// getChangedClientConfs iterates through the MicrogenGapicConfigs and returns
// a slice of the entries corresponding to modified modules and clients
func (p *postProcessor) getChangedClientConfs() map[string]*libraryInfo {
	runConfs := map[string]*libraryInfo{}
	if len(p.modules) != 0 {
		for inputDir, li := range p.config.GoogleapisToImportPath {
			for _, scope := range p.modules {
				scopePathElement := "/" + scope + "/"
				if strings.Contains(inputDir, scopePathElement) {
					runConfs[inputDir] = li
				}
			}
		}
		return runConfs
	}
	return p.config.GoogleapisToImportPath
}

func (p *postProcessor) replaceAllForSnippets(snippetDir string) error {
	return execv.ForEachMod(p.googleCloudDir, func(dir string) error {
		processMod := false
		if p.modules != nil {
			// Checking each path component in its entirety prevents mistaken addition of modules whose names
			// contain the scope as a substring. For example if the scope is "video" we do not want to regenerate
			// snippets for "videointelligence"
			dirSlice := strings.Split(dir, "/")
			for _, mod := range p.modules {
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
		return gocmd.EditReplace(snippetDir, mod, rel)
	})
}

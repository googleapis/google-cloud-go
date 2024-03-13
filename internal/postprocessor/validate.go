// Copyright 2024 Google LLC
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
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	validateCmd           *flag.FlagSet
	owlBotConfigPath      string
	processorConfigPath   string
	validateGoogleapisDir string

	lastSegmentExecptions = map[string]bool{
		"admin":       true, // firestore
		"common":      true, // oslogin
		"type":        true, // shopping et al.
		"autogen":     true, // longrunning
		"longrunning": true,
	}
	moduleAPIVersionRegex    = regexp.MustCompile(`\/api(v[1-9]+[a-z0-9]*)`)
	directoryAPIVersionRegex = regexp.MustCompile(`\/(v[1-9]+[a-z0-9]*)`)
)

const (
	defaultOwlBotConfig    = ".github/.OwlBot.yaml"
	defaultProcessorConfig = "internal/postprocessor/config.yaml"
)

func init() {
	validateCmd = flag.NewFlagSet("validate", flag.ExitOnError)
	validateCmd.StringVar(&owlBotConfigPath, "owl-bot-config", "", "Absolute path to OwlBot config. Defaults to $PWD/"+defaultOwlBotConfig)
	validateCmd.StringVar(&processorConfigPath, "processor-config", "", "Absolute path to PostProcessor config. Defaults to $PWD/"+defaultProcessorConfig)
	validateCmd.StringVar(&validateGoogleapisDir, "googleapis-dir", "", "Absolute path to googleapis directory - enables file existence check(s). Default disabled.")
}

func validate() error {
	validateCmd.Parse(os.Args[2:])
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	if owlBotConfigPath == "" {
		owlBotConfigPath = filepath.Join(dir, defaultOwlBotConfig)
	}
	if processorConfigPath == "" {
		processorConfigPath = filepath.Join(dir, defaultProcessorConfig)
	}
	log.Println("owl-bot-config set to", owlBotConfigPath)
	log.Println("processor-config set to", processorConfigPath)
	log.Println("googleapis-dir set to", validateGoogleapisDir)

	ppc, obc, err := loadConfigs(processorConfigPath, owlBotConfigPath)
	if err != nil {
		return err
	}

	if err := validatePostProcessorConfig(ppc); err != nil {
		log.Println("error validating post processor config")
		return err
	}

	if err := validateOwlBotConfig(obc, ppc); err != nil {
		log.Println("error validating OwlBot config")
		return err
	}

	return nil
}

func validatePostProcessorConfig(ppc *postProcessorConfig) error {
	// Verify no duplicate module entries - `modules` property in YAML.
	mods := make(map[string]bool, len(ppc.Modules))
	for _, m := range ppc.Modules {
		if seen := mods[m]; seen {
			return fmt.Errorf("duplicate post-processor modules entry: %s", m)
		}
		mods[m] = true
	}

	serviceConfigs := make(map[string]*serviceConfigEntry)
	for _, s := range ppc.ServiceConfigs {
		if strings.Contains(s.InputDirectory, "grafeas") {
			// Skip grafeas because it's an oddity that won't change anytime soon.
			continue
		}

		// Verify no duplicate service config entries by `import-path`.
		if _, seen := serviceConfigs[s.ImportPath]; seen {
			return fmt.Errorf("duplicate post-processor service-configs entry for import-path: %s", s.ImportPath)
		}
		if err := validateServiceConfigEntry(s); err != nil {
			return err
		}

		serviceConfigs[s.ImportPath] = s
	}

	return nil
}

func validateServiceConfigEntry(s *serviceConfigEntry) error {
	if !strings.HasPrefix(s.ImportPath, "cloud.google.com/go/") {
		return fmt.Errorf("import-path should start with 'cloud.google.com/go/': %s", s.ImportPath)
	}

	// Verify that import-path ends with "apiv" suffix.
	importMatches := moduleAPIVersionRegex.FindAllStringSubmatch(s.ImportPath, 1)
	last := s.ImportPath[strings.LastIndex(s.ImportPath, "/")+1:]
	if len(importMatches) == 0 && !lastSegmentExecptions[last] {
		return fmt.Errorf("import-path should have an api version in format 'apiv[a-b1-9]+': %s", s.ImportPath)
	}

	// Verify that input-directory ends with version suffix.
	dirMatches := directoryAPIVersionRegex.FindAllStringSubmatch(s.InputDirectory, -1)
	last = s.InputDirectory[strings.LastIndex(s.InputDirectory, "/")+1:]
	if len(dirMatches) == 0 && !lastSegmentExecptions[last] {
		return fmt.Errorf("import-path should have an api version in format 'v[a-b1-9]+': %s", s.InputDirectory)
	}

	// Verify import-path version matches api version in input-directory.
	// Skip this if there were no matches for the expected segments.
	if len(dirMatches) > 0 && len(importMatches) > 0 {
		importVersion := importMatches[0][1]
		dirVersion := dirMatches[0][1]
		if importVersion != dirVersion {
			return fmt.Errorf("mismatched input-directory (%s) and import-path (%s) versions: %s vs. %s", s.InputDirectory, s.ImportPath, dirVersion, importVersion)
		}
	}

	// Verify that the service-config file actually exists, if requested.
	if validateGoogleapisDir != "" {
		serviceConfigPath := filepath.Join(validateGoogleapisDir, s.InputDirectory, s.ServiceConfig)
		if _, err := os.Stat(serviceConfigPath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("service-config file does not exist: %s", serviceConfigPath)
		}
	}

	return nil
}

func validateOwlBotConfig(obc *owlBotConfig, ppc *postProcessorConfig) error {
	// Collect all API directories with post processor configs to ensure each
	// has an appropriate OwlBot config.
	postProcessedDirectories := make(map[string]*serviceConfigEntry, len(ppc.ServiceConfigs))
	postProcessedImportPaths := make(map[string]*serviceConfigEntry, len(ppc.ServiceConfigs))
	for _, s := range ppc.ServiceConfigs {
		postProcessedDirectories[s.InputDirectory] = s
		importPath := s.ImportPath
		if s.RelPath != "" {
			importPath = filepath.Join("cloud.google.com/go", s.RelPath)
		}
		postProcessedImportPaths[importPath] = s
	}

	sources := make(map[string]bool, len(obc.DeepCopyRegex))
	for _, dcr := range obc.DeepCopyRegex {
		// Verify no duplicate DeepCopyRegex configs
		if sources[dcr.Source] {
			return fmt.Errorf("duplicate deep-copy-regex entry: %s", dcr.Source)
		}

		// Verify that each DeepCopyRegex has a corresponding PostProcessor config
		// entry. Also detects if there is typo in the DeepCopyRegex source.
		//
		// Substring from 1 to trim the leading '/' from Source.
		apiSource := dcr.Source[1:strings.Index(dcr.Source, "/cloud.google.com")]
		if _, ok := postProcessedDirectories[apiSource]; !ok {
			return fmt.Errorf("copied directory is missing a post-processor config or vice versa: %s", dcr.Source)
		}

		sources[dcr.Source] = true
	}

	removals := make(map[string]bool, len(obc.DeepRemoveRegex))
	for _, drr := range obc.DeepRemoveRegex {
		drr = strings.TrimSuffix(drr, "/")
		// Verify no duplicate deep-remove-regex entries.
		if removals[drr] {
			return fmt.Errorf("duplicate deep-remove-regex entry: %s", drr)
		}

		// Verify deep-remove-regex is associated with a PostProcessor config entry.
		// Also detects if there is typo in the deep-remove-regex entry.
		if !strings.HasPrefix(drr, "/internal/generated/snippets") {
			fullImportPath := filepath.Join("cloud.google.com/go", drr)
			if _, ok := postProcessedImportPaths[fullImportPath]; !ok {
				return fmt.Errorf("removed importpath is missing a post-processor config or vice versa: %s", drr)
			}
		}

		removals[drr] = true
	}

	return nil
}

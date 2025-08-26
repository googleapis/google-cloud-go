// Copyright 2025 Google LLC
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

	"gopkg.in/yaml.v3"
)

// Copied from https://github.com/googleapis/google-cloud-go/blob/main/internal/postprocessor/config.go
// (and then trimmed)

type serviceConfigEntry struct {
	InputDirectory       string `yaml:"input-directory"`
	ServiceConfig        string `yaml:"service-config"`
	ImportPath           string `yaml:"import-path"`
	RelPath              string `yaml:"rel-path"`
	ReleaseLevelOverride string `yaml:"release-level-override"`
}

type postProcessorConfig struct {
	ServiceConfigs []*serviceConfigEntry `yaml:"service-configs"`
}

func loadPostProcessorConfig(path string) (*postProcessorConfig, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config postProcessorConfig
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		return nil, fmt.Errorf("unmarshaling post-processor config: %w", err)
	}
	return &config, nil
}

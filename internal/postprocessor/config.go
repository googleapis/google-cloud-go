package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Modules are all the modules roots the post processor should generate
	// template files for.
	Modules []string `yaml:"modules"`
	// GoogleapisToImportPath is a map of a googleapis dir to the corresponding
	// gapic import path.
	GoogleapisToImportPath map[string]*LibraryInfo
}

// ServiceConfig
type ServiceConfig struct {
	InputDirectory string `yaml:"input-directory"`
	ServiceConfig  string `yaml:"service-config"`
}

// LibraryInfo contains information about a GAPIC client.
type LibraryInfo struct {
	// ImportPath is the Go import path for the GAPIC library.
	ImportPath string
	// ServiceConfig is the relative directory to the service config from the
	// services directory in googleapis.
	ServiceConfig string
}

func loadConfig(root string) (*Config, error) {
	var postProcessorConfig struct {
		Modules        []string `yaml:"modules"`
		ServiceConfigs []*struct {
			InputDirectory string `yaml:"input-directory"`
			ServiceConfig  string `yaml:"service-config"`
		} `yaml:"service-configs"`
	}
	b, err := os.ReadFile(filepath.Join(root, "internal", "postprocessor", "config.yaml"))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &postProcessorConfig); err != nil {
		return nil, err
	}
	var owlBotConfig struct {
		DeepCopyRegex []struct {
			Source string `yaml:"source"`
			Dest   string `yaml:"dest"`
		} `yaml:"deep-copy-regex"`
	}
	b2, err := os.ReadFile(filepath.Join(root, ".github", ".OwlBot.yaml"))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b2, &owlBotConfig); err != nil {
		return nil, err
	}

	c := &Config{
		Modules: postProcessorConfig.Modules,
	}
	for _, v := range postProcessorConfig.ServiceConfigs {
		c.GoogleapisToImportPath[v.InputDirectory] = &LibraryInfo{
			ServiceConfig: v.ServiceConfig,
		}
	}
	for _, v := range owlBotConfig.DeepCopyRegex {
		i := strings.Index(v.Source, "/cloud.google.com/go")
		li, ok := c.GoogleapisToImportPath[v.Source[1:i]]
		if !ok {
			return nil, fmt.Errorf("unable to find value for %q, it may be missing a service config entry", v.Source[1:i])
		}
		li.ImportPath = v.Source[i+1:]
	}

	return c, nil
}

func (c *Config) GapicImportPaths() []string {
	var s []string
	for _, v := range c.GoogleapisToImportPath {
		s = append(s, v.ImportPath)
	}
	return s
}

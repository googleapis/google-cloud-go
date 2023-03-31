package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Modules []string `yaml:"modules"`
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

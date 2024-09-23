// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// config holds the configuration for a package.
type config struct {
	Package         string
	ProtoImportPath string `yaml:"protoImportPath"`

	// The types to process. Only these types and the types they depend
	// on will be output.
	// The key is the name of the proto type.
	Types map[string]*typeConfig
	// Omit the types in this list, even if they would normally be output.
	// Elements can be globs.
	OmitTypes []string `yaml:"omitTypes"`
	// Converter functions for types not in the proto package.
	// Each value should be "tofunc, fromfunc"
	Converters map[string]string
}

type typeConfig struct {
	// The name for the veneer type, if different.
	Name string
	// The prefix of the proto enum values. It will be removed.
	ProtoPrefix string `yaml:"protoPrefix"`
	// The prefix for the veneer enum values, if different from the type name.
	VeneerPrefix string `yaml:"veneerPrefix"`
	// Overrides for enum values.
	ValueNames map[string]string `yaml:"valueNames"`
	// Overrides for field types. Map key is proto field name.
	Fields map[string]fieldConfig
	// Custom conversion functions: "tofunc, fromfunc"
	ConvertToFrom string `yaml:"convertToFrom"`
	// Custom population functions, that are called after field-by-field conversion: "tofunc, fromfunc"
	PopulateToFrom string `yaml:"populateToFrom"`
	// Doc string for the type, omitting the initial type name.
	// This replaces the first line of the doc.
	Doc string
	// Remove all but the first line of the doc.
	RemoveOtherDoc bool `yaml:"removeOtherDoc"`
	// Verb to place after type name in doc. Default: "is".
	// Ignored if Doc is non-empty.
	DocVerb string `yaml:"docVerb"`
}

type fieldConfig struct {
	Name string // veneer name
	Type string // veneer type
	Doc  string // Doc string for the field. Replaces existing doc.
	// Omit from output.
	Omit bool
	// This field is not part of the proto; add it.
	Add bool
	// Generate the type, but not conversions.
	// The populate functions (see [typeConfg.PopulateToFrom]) should set the field.
	NoConvert bool `yaml:"noConvert"`
	// Custom conversion functions: "tofunc, fromfunc"
	ConvertToFrom string `yaml:"convertToFrom"`
}

func (c *config) init() {
	for protoName, tc := range c.Types {
		if tc == nil {
			tc = &typeConfig{Name: protoName}
			c.Types[protoName] = tc
		}
		if tc.Name == "" {
			tc.Name = protoName
		}
		tc.init()
	}
}

func (tc *typeConfig) init() {
	if tc.VeneerPrefix == "" {
		tc.VeneerPrefix = tc.Name
	}
}

func readConfigFile(filename string) (*config, error) {
	if filename == "" {
		return nil, errors.New("missing config file")
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var c config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	c.init()
	return &c, nil
}

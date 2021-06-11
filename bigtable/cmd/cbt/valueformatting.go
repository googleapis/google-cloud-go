package main

import (
	"io/ioutil"

	"github.com/jhump/protoreflect/desc"
	"gopkg.in/yaml.v2"
)

type ValueFormatColumn struct {
	Encoding string
	Type string
}

type ValueFormatFamily struct {
	DefaultEncoding string `yaml:"default_encoding"`
	DefaultType string `yaml:"default_type"`
	Columns map[string]ValueFormatColumn
}

type ValueFormatProtocolBufferDefinition struct {
	Definitions []string
	Imports []string
}


type ValueFormatSettings struct {
	ProtocolBuffer ValueFormatProtocolBufferDefinition `yaml:"protocol_buffer"`
	DefaultEncoding string `yaml:"default_encoding"`
	DefaultType string `yaml:"default_type"`
	Families map[string]ValueFormatFamily
}

type ValueFormatting struct {
	settings ValueFormatSettings
	flags struct {
		formatFile string
	}
	pbMessageTypes map[string]*desc.MessageDescriptor
}


func (self *ValueFormatting) parse(path string) error {
	data, err := ioutil.ReadFile(path)
	if err == nil {
		err = yaml.UnmarshalStrict([]byte(data), &self.settings)
	}
	return err
}


func (self *ValueFormatting) setup() error {
	if self.flags.formatFile != "" {
		self.parse(self.flags.formatFile)
	}
	return nil
}


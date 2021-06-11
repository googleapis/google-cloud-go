package main

import (
	"io/ioutil"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
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

func (self *ValueFormatting) setupPBMessages() error {
	if len(self.settings.ProtocolBuffer.Definitions) > 0 {
		parser := protoparse.Parser{
			ImportPaths: self.settings.ProtocolBuffer.Imports,
		}
		fds, err := parser.ParseFiles(
			self.settings.ProtocolBuffer.Definitions...)
		if err != nil {
			return err
		}
		self.pbMessageTypes = make(map[string]*desc.MessageDescriptor)
		for _, fd := range(fds) {
			prefix := fd.GetPackage()
			for _, md := range fd.GetMessageTypes() {
				key := md.GetName()
				self.pbMessageTypes[key] = md
				self.pbMessageTypes[strings.ToLower(key)] = md
				if prefix != "" {
					key = prefix + "." + key
					self.pbMessageTypes[key] = md
					self.pbMessageTypes[strings.ToLower(key)] = md
				}
			}		
		}
	}
	return nil
}

func (self *ValueFormatting) setup() error {
	if self.flags.formatFile != "" {
		err := self.parse(self.flags.formatFile)
		if err == nil {
			err = self.setupPBMessages()
		}
		return err
	}
	return nil
}


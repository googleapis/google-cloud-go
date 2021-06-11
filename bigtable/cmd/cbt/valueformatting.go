package main

import (
	"encoding/binary"
	"bytes"
	"fmt"
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

func binaryFormatterHelper(
	in []byte,
	byteOrder binary.ByteOrder,
	elemsize int,
	v interface{}) (string, error) {

	if (len(in) % elemsize) != 0 {
		return "", fmt.Errorf(
			"data size, %d, isn't a multiple of element size, %d",
			len(in),
			elemsize,
		)
	}
	var s string
	err := binary.Read(bytes.NewReader(in), byteOrder, v)
	if err == nil {
		s = fmt.Sprint(v)[1:]
		if len(in) == elemsize {
			s = s[1:len(s) - 1]
		}
	}
	return s, err
}

type valueFormatter func ([]byte) (string, error)

type binaryValueFormatter func([]byte, binary.ByteOrder) (string, error) 

var binaryValueFormatters = map[string]binaryValueFormatter{
	"int8": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int8, len(in))
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"int16": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int16, len(in)/2)
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"int32": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int32, len(in)/4)
		return binaryFormatterHelper(in, byteOrder, 4, &v)
	},
	"int64": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int64, len(in)/8)
		return binaryFormatterHelper(in, byteOrder, 8, &v)
	},
	"uint8": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint8, len(in))
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"uint16": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint16, len(in)/2)
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"uint32": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint32, len(in)/4)
		return binaryFormatterHelper(in, byteOrder, 4, &v)
	},
	"uint64": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint64, len(in)/8)
		return binaryFormatterHelper(in, byteOrder, 8, &v)
	},
	"float32": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]float32, len(in)/4)
		return binaryFormatterHelper(in, byteOrder, 4, &v)
	},
	"float64": func (in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]float64, len(in)/8)
		return binaryFormatterHelper(in, byteOrder, 8, &v)
	},
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
		for _, fd := range fds {
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


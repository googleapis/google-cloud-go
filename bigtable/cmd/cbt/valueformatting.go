/*
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"gopkg.in/yaml.v2"
)

type ValueFormatColumn struct {
	Encoding string
	Type     string
}

type ValueFormatFamily struct {
	DefaultEncoding string `yaml:"default_encoding"`
	DefaultType     string `yaml:"default_type"`
	Columns         map[string]ValueFormatColumn
}

func NewValueFormatFamily() ValueFormatFamily { // for tests :)
	family := ValueFormatFamily{}
	family.Columns = make(map[string]ValueFormatColumn)
	return family
}

type ValueFormatSettings struct {
	ProtocolBufferDefinitions []string `yaml:"protocol_buffer_definitions"`
	ProtocolBufferPaths       []string `yaml:"protocol_buffer_paths"`
	DefaultEncoding           string   `yaml:"default_encoding"`
	DefaultType               string   `yaml:"default_type"`
	Columns                   map[string]ValueFormatColumn
	Families                  map[string]ValueFormatFamily
}

type valueFormatter func([]byte) (string, error)

type ValueFormatting struct {
	settings       ValueFormatSettings
	pbMessageTypes map[string]*desc.MessageDescriptor
	formatters     map[[2]string]valueFormatter
}

func NewValueFormatting() ValueFormatting {
	formatting := ValueFormatting{}
	formatting.settings.Columns = make(map[string]ValueFormatColumn)
	formatting.settings.Families = make(map[string]ValueFormatFamily)
	formatting.pbMessageTypes = make(map[string]*desc.MessageDescriptor)
	formatting.formatters = make(map[[2]string]valueFormatter)
	return formatting
}

var valueFormatting = NewValueFormatting()

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
			s = s[1 : len(s)-1]
		}
	}
	return s, err
}

type binaryValueFormatter func([]byte, binary.ByteOrder) (string, error)

var binaryValueFormatters = map[string]binaryValueFormatter{
	"int8": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int8, len(in))
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"int16": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int16, len(in)/2)
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"int32": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int32, len(in)/4)
		return binaryFormatterHelper(in, byteOrder, 4, &v)
	},
	"int64": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]int64, len(in)/8)
		return binaryFormatterHelper(in, byteOrder, 8, &v)
	},
	"uint8": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint8, len(in))
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"uint16": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint16, len(in)/2)
		return binaryFormatterHelper(in, byteOrder, 2, &v)
	},
	"uint32": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint32, len(in)/4)
		return binaryFormatterHelper(in, byteOrder, 4, &v)
	},
	"uint64": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]uint64, len(in)/8)
		return binaryFormatterHelper(in, byteOrder, 8, &v)
	},
	"float32": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]float32, len(in)/4)
		return binaryFormatterHelper(in, byteOrder, 4, &v)
	},
	"float64": func(in []byte, byteOrder binary.ByteOrder) (string, error) {
		v := make([]float64, len(in)/8)
		return binaryFormatterHelper(in, byteOrder, 8, &v)
	},
}

func (self *ValueFormatting) binaryFormatter(encoding, type_ string) valueFormatter {
	var byteOrder binary.ByteOrder
	typeFormatter := binaryValueFormatters[type_]
	if encoding == "BigEndian" {
		byteOrder = binary.BigEndian
	} else {
		byteOrder = binary.LittleEndian
	}
	return func(in []byte) (string, error) {
		return typeFormatter(in, byteOrder)
	}
}

func (self *ValueFormatting) pbFormatter(type_ string) valueFormatter {
	md := self.pbMessageTypes[strings.ToLower(type_)]
	return func(in []byte) (string, error) {
		message := dynamic.NewMessage(md)
		err := message.Unmarshal(in)
		if err == nil {
			data, err := message.MarshalTextIndent()
			if err == nil {
				return string(data), nil
			}
		}
		return "", err
	}
}

var validValueFormattingEncodings = map[string]string{
	"bigendian":       "BigEndian",
	"b":               "BigEndian",
	"binary":          "BigEndian",
	"littleendian":    "LittleEndian",
	"L":               "LittleEndian",
	"hex":             "Hex",
	"h":               "Hex",
	"protocolbuffer":  "ProtocolBuffer",
	"protocol-buffer": "ProtocolBuffer",
	"protocol_buffer": "ProtocolBuffer",
	"proto":           "ProtocolBuffer",
	"p":               "ProtocolBuffer",
	"":                "",
}

func (self *ValueFormatting) validateEncoding(encoding string) (string, error) {
	validEncoding, got := validValueFormattingEncodings[strings.ToLower(encoding)]
	if !got {
		return "", fmt.Errorf("Invalid encoding: %s", encoding)
	}
	return validEncoding, nil
}

func (self *ValueFormatting) validateType(
	cname, validEncoding, encoding, type_ string,
) (string, error) {
	var got bool
	switch validEncoding {
	case "LittleEndian", "BigEndian":
		if type_ == "" {
			return type_, fmt.Errorf(
				"No type specified for encoding: %s",
				encoding)
		}
		_, got = binaryValueFormatters[strings.ToLower(type_)]
		if !got {
			return type_, fmt.Errorf("Invalid type: %s for encoding: %s",
				type_, encoding)
		}
		type_ = strings.ToLower(type_)
	case "ProtocolBuffer":
		if type_ == "" {
			type_ = cname
		}
		_, got = self.pbMessageTypes[strings.ToLower(type_)]
		if !got {
			return type_, fmt.Errorf("Invalid type: %s for encoding: %s",
				type_, encoding)
		}
	}
	return type_, nil
}

func (self *ValueFormatting) validateFormat(
	cname, encoding, type_ string,
) (string, string, error) {
	validEncoding, err := self.validateEncoding(encoding)
	if err == nil {
		type_, err = self.validateType(cname, validEncoding, encoding, type_)
	}
	return validEncoding, type_, err
}

func (self *ValueFormatting) override(old, new string) string {
	if new != "" {
		return new
	} else {
		return old
	}
}

func (self *ValueFormatting) validateColumns() error {
	encoding := self.settings.DefaultEncoding
	type_ := self.settings.DefaultType

	var errs []string
	for cname, col := range self.settings.Columns {
		_, _, err := self.validateFormat(
			cname,
			self.override(encoding, col.Encoding),
			self.override(type_, col.Type))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", cname, err))
		}
	}
	for fname, fam := range self.settings.Families {
		fencoding := self.override(encoding, fam.DefaultEncoding)
		ftype_ := self.override(type_, fam.DefaultType)
		for cname, col := range fam.Columns {
			_, _, err := self.validateFormat(
				cname,
				self.override(fencoding, col.Encoding),
				self.override(ftype_, col.Type))
			if err != nil {
				errs = append(errs, fmt.Sprintf(
					"%s:%s: %s", fname, cname, err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf(
			"Bad encoding and types:\n%s",
			strings.Join(errs, "\n"))
	}
	return nil
}

func (self *ValueFormatting) parse(path string) error {
	data, err := ioutil.ReadFile(path)
	if err == nil {
		err = yaml.UnmarshalStrict([]byte(data), &self.settings)
	}
	return err
}

func (self *ValueFormatting) setupPBMessages() error {
	if len(self.settings.ProtocolBufferDefinitions) > 0 {
		parser := protoparse.Parser{
			ImportPaths: self.settings.ProtocolBufferPaths,
		}
		fds, err := parser.ParseFiles(
			self.settings.ProtocolBufferDefinitions...)
		if err != nil {
			return err
		}
		for _, fd := range fds {
			prefix := fd.GetPackage()
			for _, md := range fd.GetMessageTypes() {
				key := md.GetName()
				self.pbMessageTypes[strings.ToLower(key)] = md
				if prefix != "" {
					key = prefix + "." + key
					self.pbMessageTypes[strings.ToLower(key)] = md
				}
			}
		}
	}
	return nil
}

func (self *ValueFormatting) setup(options map[string]string) error {
	var err error = nil
	if options["format-file"] != "" {
		err = self.parse(options["format-file"])
	}
	if err == nil {
		err = self.setupPBMessages()
	}
	if err == nil {
		err = self.validateColumns()
	}
	return err
}

func (self *ValueFormatting) colEncodingType(family, column string) (string, string) {
	encoding := self.settings.DefaultEncoding
	type_ := self.settings.DefaultType

	fam, got := self.settings.Families[family]
	if got {
		fencoding := self.override(encoding, fam.DefaultEncoding)
		ftype := self.override(type_, fam.DefaultType)
		col, got := fam.Columns[column]
		if got {
			return self.override(fencoding, col.Encoding),
				self.override(ftype, col.Type)
		} else {
			return fencoding, ftype
		}
	} else {
		col, got := self.settings.Columns[column]
		if got {
			return self.override(encoding, col.Encoding),
				self.override(type_, col.Type)
		} else {
			return encoding, type_
		}
	}
}

func (self *ValueFormatting) badFormatter(err error) valueFormatter {
	return func(in []byte) (string, error) {
		return "", err
	}
}

func (self *ValueFormatting) hexFormatter(in []byte) (string, error) {
	return fmt.Sprintf("% x", in), nil
}

func (self *ValueFormatting) defaultFormatter(in []byte) (string, error) {
	return fmt.Sprintf("%q", in), nil
}

func (self *ValueFormatting) format(
	prefix, family, column string, value []byte,
) (string, error) {
	key := [2]string{family, column}
	formatter, got := self.formatters[key]
	if !got {
		encoding, type_ := self.colEncodingType(family, column)
		encoding, type_, err := self.validateFormat(column, encoding, type_)
		if err != nil {
			formatter = self.badFormatter(err)
		} else {
			switch encoding {
			case "BigEndian", "LittleEndian":
				formatter = self.binaryFormatter(encoding, type_)
			case "Hex":
				formatter = self.hexFormatter
			case "ProtocolBuffer":
				formatter = self.pbFormatter(type_)
			case "":
				formatter = self.defaultFormatter
			}
		}
		self.formatters[key] = formatter
	}
	formatted, err := formatter(value)
	if err == nil {
		formatted = prefix +
			strings.TrimSuffix(
				strings.ReplaceAll(formatted, "\n", "\n"+prefix),
				prefix) + "\n"
	}
	return formatted, err
}

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

type valueFormatColumn struct {
	Encoding string
	Type     string
}

type valueFormatFamily struct {
	DefaultEncoding string `yaml:"default_encoding"`
	DefaultType     string `yaml:"default_type"`
	Columns         map[string]valueFormatColumn
}

func newValueFormatFamily() valueFormatFamily { // for tests :)
	family := valueFormatFamily{}
	family.Columns = make(map[string]valueFormatColumn)
	return family
}

type valueFormatSettings struct {
	ProtocolBufferDefinitions []string `yaml:"protocol_buffer_definitions"`
	ProtocolBufferPaths       []string `yaml:"protocol_buffer_paths"`
	DefaultEncoding           string   `yaml:"default_encoding"`
	DefaultType               string   `yaml:"default_type"`
	Columns                   map[string]valueFormatColumn
	Families                  map[string]valueFormatFamily
}

type valueFormatter func([]byte) (string, error)

type valueFormatting struct {
	settings       valueFormatSettings
	pbMessageTypes map[string]*desc.MessageDescriptor
	formatters     map[[2]string]valueFormatter
}

func newValueFormatting() valueFormatting {
	formatting := valueFormatting{}
	formatting.settings.Columns = make(map[string]valueFormatColumn)
	formatting.settings.Families = make(map[string]valueFormatFamily)
	formatting.pbMessageTypes = make(map[string]*desc.MessageDescriptor)
	formatting.formatters = make(map[[2]string]valueFormatter)
	return formatting
}

var globalValueFormatting = newValueFormatting()

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

func (formatting *valueFormatting) binaryFormatter(
	encoding, ctype string,
) valueFormatter {
	var byteOrder binary.ByteOrder
	// We don't check the get below because it's checked in
	// validateType, which is called by validateFormat, which is
	// called by format before calling this. :)
	typeFormatter := binaryValueFormatters[ctype]
	if encoding == "BigEndian" {
		byteOrder = binary.BigEndian
	} else {
		byteOrder = binary.LittleEndian
	}
	return func(in []byte) (string, error) {
		return typeFormatter(in, byteOrder)
	}
}

func (formatting *valueFormatting) pbFormatter(ctype string) valueFormatter {
	// We don't check the get below because it's checked in
	// validateType, which is called by validateFormat, which is
	// called by format before calling this. :)
	md := formatting.pbMessageTypes[strings.ToLower(ctype)]
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

func (formatting *valueFormatting) validateEncoding(encoding string) (string, error) {
	validEncoding, got := validValueFormattingEncodings[strings.ToLower(encoding)]
	if !got {
		return "", fmt.Errorf("Invalid encoding: %s", encoding)
	}
	return validEncoding, nil
}

func (formatting *valueFormatting) validateType(
	cname, validEncoding, encoding, ctype string,
) (string, error) {
	var got bool
	switch validEncoding {
	case "LittleEndian", "BigEndian":
		if ctype == "" {
			return ctype, fmt.Errorf(
				"No type specified for encoding: %s",
				encoding)
		}
		_, got = binaryValueFormatters[strings.ToLower(ctype)]
		if !got {
			return ctype, fmt.Errorf("Invalid type: %s for encoding: %s",
				ctype, encoding)
		}
		ctype = strings.ToLower(ctype)
	case "ProtocolBuffer":
		if ctype == "" {
			ctype = cname
		}
		_, got = formatting.pbMessageTypes[strings.ToLower(ctype)]
		if !got {
			return ctype, fmt.Errorf("Invalid type: %s for encoding: %s",
				ctype, encoding)
		}
	}
	return ctype, nil
}

func (formatting *valueFormatting) validateFormat(
	cname, encoding, ctype string,
) (string, string, error) {
	validEncoding, err := formatting.validateEncoding(encoding)
	if err == nil {
		ctype, err =
			formatting.validateType(cname, validEncoding, encoding, ctype)
	}
	return validEncoding, ctype, err
}

func (formatting *valueFormatting) override(old, new string) string {
	if new != "" {
		return new
	}
	return old
}

func (formatting *valueFormatting) validateColumns() error {
	defaultEncoding := formatting.settings.DefaultEncoding
	defaultType := formatting.settings.DefaultType

	var errs []string
	for cname, col := range formatting.settings.Columns {
		_, _, err := formatting.validateFormat(
			cname,
			formatting.override(defaultEncoding, col.Encoding),
			formatting.override(defaultType, col.Type))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", cname, err))
		}
	}
	for fname, fam := range formatting.settings.Families {
		familyEncoding :=
			formatting.override(defaultEncoding, fam.DefaultEncoding)
		familyType := formatting.override(defaultType, fam.DefaultType)
		for cname, col := range fam.Columns {
			_, _, err := formatting.validateFormat(
				cname,
				formatting.override(familyEncoding, col.Encoding),
				formatting.override(familyType, col.Type))
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

func (formatting *valueFormatting) parse(path string) error {
	data, err := ioutil.ReadFile(path)
	if err == nil {
		err = yaml.UnmarshalStrict([]byte(data), &formatting.settings)
	}
	return err
}

func (formatting *valueFormatting) setupPBMessages() error {
	if len(formatting.settings.ProtocolBufferDefinitions) > 0 {
		parser := protoparse.Parser{
			ImportPaths: formatting.settings.ProtocolBufferPaths,
		}
		fds, err := parser.ParseFiles(
			formatting.settings.ProtocolBufferDefinitions...)
		if err != nil {
			return err
		}
		for _, fd := range fds {
			prefix := fd.GetPackage()
			for _, md := range fd.GetMessageTypes() {
				key := md.GetName()
				formatting.pbMessageTypes[strings.ToLower(key)] = md
				if prefix != "" {
					key = prefix + "." + key
					formatting.pbMessageTypes[strings.ToLower(key)] = md
				}
			}
		}
	}
	return nil
}

func (formatting *valueFormatting) setup(options map[string]string) error {
	var err error = nil
	if options["format-file"] != "" {
		err = formatting.parse(options["format-file"])
	}
	if err == nil {
		err = formatting.setupPBMessages()
	}
	if err == nil {
		err = formatting.validateColumns()
	}
	return err
}

func (formatting *valueFormatting) colEncodingType(
	family, column string,
) (string, string) {
	defaultEncoding := formatting.settings.DefaultEncoding
	defaultType := formatting.settings.DefaultType

	fam, got := formatting.settings.Families[family]
	if got {
		familyEncoding :=
			formatting.override(defaultEncoding, fam.DefaultEncoding)
		familyType := formatting.override(defaultType, fam.DefaultType)
		col, got := fam.Columns[column]
		if got {
			return formatting.override(familyEncoding, col.Encoding),
				formatting.override(familyType, col.Type)
		}
		return familyEncoding, familyType
	}
	col, got := formatting.settings.Columns[column]
	if got {
		return formatting.override(defaultEncoding, col.Encoding),
			formatting.override(defaultType, col.Type)
	}
	return defaultEncoding, defaultType
}

func (formatting *valueFormatting) badFormatter(err error) valueFormatter {
	return func(in []byte) (string, error) {
		return "", err
	}
}

func (formatting *valueFormatting) hexFormatter(in []byte) (string, error) {
	return fmt.Sprintf("% x", in), nil
}

func (formatting *valueFormatting) defaultFormatter(in []byte) (string, error) {
	return fmt.Sprintf("%q", in), nil
}

func (formatting *valueFormatting) format(
	prefix, family, column string, value []byte,
) (string, error) {
	key := [2]string{family, column}
	formatter, got := formatting.formatters[key]
	if !got {
		encoding, ctype := formatting.colEncodingType(family, column)
		encoding, ctype, err :=
			formatting.validateFormat(column, encoding, ctype)
		if err != nil {
			formatter = formatting.badFormatter(err)
		} else {
			switch encoding {
			case "BigEndian", "LittleEndian":
				formatter = formatting.binaryFormatter(encoding, ctype)
			case "Hex":
				formatter = formatting.hexFormatter
			case "ProtocolBuffer":
				formatter = formatting.pbFormatter(ctype)
			case "":
				formatter = formatting.defaultFormatter
			}
		}
		formatting.formatters[key] = formatter
	}
	formatted, err := formatter(value)
	if err == nil {
		formatted = prefix +
			strings.TrimSuffix(
				strings.Replace(formatted, "\n", "\n"+prefix, 999999),
				prefix) + "\n"
	}
	return formatted, err
}

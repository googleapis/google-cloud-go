package main

import (
	"encoding/binary"
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
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

func NewValueFormatFamily() ValueFormatFamily {  // for tests :)
	family := ValueFormatFamily{}
	family.Columns = make(map[string]ValueFormatColumn)
	return family
}

type ValueFormatProtocolBufferDefinition struct {
	Definitions []string
	Imports []string
}

type ValueFormatSettings struct {
	ProtocolBuffer ValueFormatProtocolBufferDefinition `yaml:"protocol_buffer"`
	DefaultEncoding string `yaml:"default_encoding"`
	DefaultType string `yaml:"default_type"`
	Columns map[string]ValueFormatColumn
	Families map[string]ValueFormatFamily
}

type ValueFormatting struct {
	settings ValueFormatSettings
	flags struct {
		formatFile string
	}
	pbMessageTypes map[string]*desc.MessageDescriptor
}

func NewValueFormatting() ValueFormatting {
	formatting := ValueFormatting{}
	formatting.settings.Columns = make(map[string]ValueFormatColumn)
	formatting.settings.Families = make(map[string]ValueFormatFamily)
	formatting.pbMessageTypes = make(map[string]*desc.MessageDescriptor)
	return formatting
}

var valueFormatting = NewValueFormatting()


var validValueFormattingEncodings = map[string]string{
	"bigendian": "BigEndian",
	"b": "BigEndian",
	"binary": "BigEndian",
	"littleendian": "LittleEndian",
	"L": "LittleEndian",
	"hex": "Hex",
	"h": "Hex",
	"protocolbuffer": "ProtocolBuffer",
	"protocol-buffer": "ProtocolBuffer",
	"protocol_buffer": "ProtocolBuffer",
	"proto": "ProtocolBuffer",
	"p": "ProtocolBuffer",
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

func (self *ValueFormatting) badFormatter(err error) valueFormatter {
	return func (in []byte) (string, error) {
		return "", err
	}
}

func (self *ValueFormatting) badFormatterf(format string, args ...interface{}) valueFormatter {
	return self.badFormatter(fmt.Errorf(format, args...))
}

func (self *ValueFormatting) binaryFormatter(
      valid_encoding string,
      type_ string,
      encoding string,
) valueFormatter {
	type_formatter, valid := binaryValueFormatters[strings.ToLower(type_)]
	if ! valid {
		if type_ == "" {
			return self.badFormatterf(
				"A data type must be provided for the %s encoding",
				encoding)
		} else {
			return self.badFormatterf("Invalid binary type: %s", type_)
		}
	}
	var byteOrder binary.ByteOrder
	if valid_encoding == "BigEndian" {
		byteOrder = binary.BigEndian
	} else {
		byteOrder = binary.LittleEndian
	} 

	return func (in []byte) (string, error) {
		return type_formatter(in, byteOrder)
	}
}

func (self *ValueFormatting) pbFormatter(type_ string) valueFormatter {

	md, got := self.pbMessageTypes[type_]
	if ! got {
		md, got = self.pbMessageTypes[strings.ToLower(type_)]
		if ! got {
			return self.badFormatterf(
				"No Protocol-Buffer message time for: %s", type_)
		}
	}

	return func (in []byte) (string, error) {
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

func (self *ValueFormatting) validate_format(encoding string, type_ string) error {
	valid_encoding, got := validValueFormattingEncodings[strings.ToLower(encoding)]
	if ! got {
		if encoding == "" {
			return fmt.Errorf("No Encoding specified")
		}
		return fmt.Errorf("Invalid encoding: %s", encoding)
	}
	if ((valid_encoding == "LittleEndian" || valid_encoding == "BigEndian")) {
		if type_ == "" {
			return fmt.Errorf(
				"No type specified for encoding: %s",
				encoding)
		}
		_, got = binaryValueFormatters[strings.ToLower(type_)]
		if ! got {
			return fmt.Errorf("Invalid type: %s for encoding: %s",
				type_, encoding)
		}
	} else if valid_encoding == "ProtocolBuffer" {
		if type_ == "" {
			return fmt.Errorf(
				"No type specified for encoding: %s",
				encoding)
		}
		_, got = self.pbMessageTypes[type_]
		if ! got {
			return fmt.Errorf("Invalid type: %s for encoding: %s",
				type_, encoding)
		}
	}
	return nil
}

func (self *ValueFormatting) override(old string, new string) string {
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
		err := self.validate_format(
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
			err := self.validate_format(
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
	var err error = nil
	if self.flags.formatFile != "" {
		err = self.parse(self.flags.formatFile)
		if err == nil {
			err = self.setupPBMessages()
		}
	}
	if err == nil {
		err = self.validateColumns()
	}
	return err
}	


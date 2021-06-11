package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jhump/protoreflect/dynamic"
	"cloud.google.com/go/internal/testutil"
)

func assertEqual(t *testing.T, label string, got, want interface{}) {
	if ! testutil.Equal(got, want) {
		t.Errorf("%s didn't match %s", label, got)
	}
}

func assertNoError(t *testing.T, err error) bool {
	if err != nil {
		t.Error(err)
		return true
	}
	return false
}

func TestParseValueFormatSettings(t *testing.T) {
	want := ValueFormatSettings{
		DefaultEncoding: "HEX",
		ProtocolBuffer: ValueFormatProtocolBufferDefinition{
			Definitions: []string{"MyProto.proto", "MyOtherProto.proto"},
			Imports: []string{"mycode/stuff", "/home/user/dev/othercode/"},
		},
		Families: map[string]ValueFormatFamily{
			"family1": ValueFormatFamily{
				DefaultEncoding: "BigEndian:INT64",
				Columns: map[string]ValueFormatColumn{
					"address": ValueFormatColumn{
						Encoding: "PROTO",
						Type: "tutorial.Person",
					},
				},
			},
			
			
			"family2": ValueFormatFamily{
				Columns: map[string]ValueFormatColumn{
					"col1": ValueFormatColumn{
						Encoding: "B",
						Type: "INT32",
					},
					"col2": ValueFormatColumn{
						Encoding: "L",
						Type: "INT16",
					},
					"address": ValueFormatColumn{
						Encoding: "PROTO",
						Type: "tutorial.Person",
					},
				},
			},
			"family3": ValueFormatFamily{
				Columns: map[string]ValueFormatColumn{
					"proto_col": ValueFormatColumn{ 
						Encoding: "PROTO",
						Type: "MyProtoMessageType",
					},
				},
			},
			
		},
	}

	formatting := ValueFormatting{}
	
	err := formatting.parse(filepath.Join("testdata", t.Name() + ".yml"))
	if err != nil {
		t.Errorf("Parse error: %s", err)
	}

	assertEqual(t, "format", formatting.settings, want)
}

func TestSetupPBMessages(t *testing.T) {

	formatting := ValueFormatting{}
	
	formatting.settings.ProtocolBuffer.Imports = append(
		formatting.settings.ProtocolBuffer.Imports,
		"testdata")
	formatting.settings.ProtocolBuffer.Imports = append(
		formatting.settings.ProtocolBuffer.Imports,
		filepath.Join("testdata", "protoincludes"))
	formatting.settings.ProtocolBuffer.Definitions = append(
		formatting.settings.ProtocolBuffer.Definitions,
		"addressbook.proto")
	formatting.settings.ProtocolBuffer.Definitions = append(
		formatting.settings.ProtocolBuffer.Definitions,
		"club.proto")
	err := formatting.setupPBMessages()
	if err != nil {
		t.Errorf("Proto parse error: %s", err)
		return
	}

	var keys []string
	for k := range(formatting.pbMessageTypes) {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assertEqual(
		t,
		"keys",
		keys,
		[]string{
			"AddressBook",
			"Equipment",
			"Person",
			"addressbook",
			"equipment",
			"person",
			"tutorial.AddressBook",
			"tutorial.Person",
			"tutorial.addressbook",
			"tutorial.person",
			},
	)

	// Make sure teh message descriptors are usable.
	message := dynamic.NewMessage(formatting.pbMessageTypes["tutorial.person"])
	in, err := ioutil.ReadFile(filepath.Join("testdata", "person.bin"))
	if err != nil {
		t.Errorf("reading data file")
		return
	}
	err = message.Unmarshal(in)
	if err != nil {
		t.Errorf("unmarshalling")
		return
	}
	assertEqual(
		t,
		"message",
		fmt.Sprint(message),
		`name:"Jim" id:42 email:"jim@example.com"` +
		` phones:<number:"555-1212" type:HOME>`)
}

var TestBinaryFormaterTestData = []byte{
	0, 1, 2, 3, 4, 5, 6, 7, 255, 255, 255, 255, 255, 255, 255, 156}

func checkBinaryValueFormater(
	t *testing.T, type_ string, nbytes int, expect string, order binary.ByteOrder,
) {
	s, err := binaryValueFormatters[type_](TestBinaryFormaterTestData[:nbytes], order)
	if assertNoError(t, err) { return }
	assertEqual(t, type_, s, expect)
}

func TestBinaryValueFormaterINT8(t *testing.T) {
	checkBinaryValueFormater(
		t, "int8", 16, "[0 1 2 3 4 5 6 7 -1 -1 -1 -1 -1 -1 -1 -100]", binary.BigEndian)
}

func TestBinaryValueFormaterINT16(t *testing.T) {
	// Main test that tests special handling of arrays vs scalers, etc.
	
	checkBinaryValueFormater(
		t, "int16", 16, "[1 515 1029 1543 -1 -1 -1 -100]", binary.BigEndian)
	checkBinaryValueFormater(t, "int16", 0, "[]", binary.BigEndian)
	checkBinaryValueFormater(t, "int16", 2, "1", binary.BigEndian)
	checkBinaryValueFormater(
		t, "int16", 16, "[256 770 1284 1798 -1 -1 -1 -25345]", binary.LittleEndian)
}

func TestBinaryValueFormaterINT32(t *testing.T) {
	checkBinaryValueFormater(
		t, "int32", 16, "[66051 67438087 -1 -100]", binary.BigEndian)
}

func TestBinaryValueFormaterINT64(t *testing.T) {
	checkBinaryValueFormater(
		t, "int64", 16, "[283686952306183 -100]", binary.BigEndian)
}

func TestBinaryValueFormaterUINT8(t *testing.T) {
	checkBinaryValueFormater(
		t, "uint8", 16, "[0 1 2 3 4 5 6 7 255 255 255 255 255 255 255 156]",
		binary.BigEndian)
}

func TestBinaryValueFormaterUINT16(t *testing.T) {
	checkBinaryValueFormater(
		t, "uint16", 16, "[1 515 1029 1543 65535 65535 65535 65436]",
		binary.BigEndian)
}

func TestBinaryValueFormaterUINT32(t *testing.T) {
	checkBinaryValueFormater(
		t, "uint32", 16, "[66051 67438087 4294967295 4294967196]", binary.BigEndian)
}

func TestBinaryValueFormaterUINT64(t *testing.T) {
	checkBinaryValueFormater(
		t, "uint64", 16, "[283686952306183 18446744073709551516]", binary.BigEndian)
}

func TestBinaryValueFormaterFLOAT32(t *testing.T) {
	checkBinaryValueFormater(
		t, "float32", 16, "[9.2557e-41 1.5636842e-36 NaN NaN]", binary.BigEndian)
}

func TestBinaryValueFormaterFLOAT64(t *testing.T) {
	checkBinaryValueFormater(
		t, "float64", 16, "[1.40159977307889e-309 NaN]", binary.BigEndian)
}

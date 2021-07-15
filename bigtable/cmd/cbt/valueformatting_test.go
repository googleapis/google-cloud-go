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
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"

	"cloud.google.com/go/bigtable"
)

func TestParseValueFormatSettings(t *testing.T) {
	want := valueFormatSettings{
		DefaultEncoding:           "HEX",
		ProtocolBufferDefinitions: []string{"MyProto.proto", "MyOtherProto.proto"},
		ProtocolBufferPaths:       []string{"mycode/stuff", "/home/user/dev/othercode/"},
		Columns: map[string]valueFormatColumn{
			"col3": {
				Encoding: "P",
				Type:     "person",
			},
			"col4": {
				Encoding: "P",
				Type:     "hobby",
			},
		},
		Families: map[string]valueFormatFamily{
			"family1": {
				DefaultEncoding: "BigEndian",
				DefaultType:     "INT64",
				Columns: map[string]valueFormatColumn{
					"address": {
						Encoding: "PROTO",
						Type:     "tutorial.Person",
					},
				},
			},

			"family2": {
				Columns: map[string]valueFormatColumn{
					"col1": {
						Encoding: "B",
						Type:     "INT32",
					},
					"col2": {
						Encoding: "L",
						Type:     "INT16",
					},
					"address": {
						Encoding: "PROTO",
						Type:     "tutorial.Person",
					},
				},
			},
			"family3": {
				Columns: map[string]valueFormatColumn{
					"proto_col": {
						Encoding: "PROTO",
						Type:     "MyProtoMessageType",
					},
				},
			},
		},
	}

	formatting := newValueFormatting()

	err := formatting.parse(filepath.Join("testdata", t.Name()+".yml"))
	if err != nil {
		t.Errorf("Parse error: %s", err)
	}

	assertEqual(t, formatting.settings, want)
}

func TestSetupPBMessages(t *testing.T) {

	formatting := newValueFormatting()

	formatting.settings.ProtocolBufferPaths = append(
		formatting.settings.ProtocolBufferPaths,
		"testdata")
	formatting.settings.ProtocolBufferPaths = append(
		formatting.settings.ProtocolBufferPaths,
		filepath.Join("testdata", "protoincludes"))
	formatting.settings.ProtocolBufferDefinitions = append(
		formatting.settings.ProtocolBufferDefinitions,
		"addressbook.proto")
	formatting.settings.ProtocolBufferDefinitions = append(
		formatting.settings.ProtocolBufferDefinitions,
		"club.proto")
	err := formatting.setupPBMessages()
	if err != nil {
		t.Errorf("Proto parse error: %s", err)
		return
	}

	var keys []string
	for k := range formatting.pbMessageTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assertEqual(
		t,
		keys,
		[]string{
			"addressbook",
			"equipment",
			"person",
			"tutorial.addressbook",
			"tutorial.person",
		},
	)

	// Make sure the message descriptors are usable.
	message := dynamic.NewMessage(formatting.pbMessageTypes["tutorial.person"])
	in, err := ioutil.ReadFile(filepath.Join("testdata", "person.bin"))
	assertNoError(t, err)
	err = message.Unmarshal(in)
	assertNoError(t, err)
	assertEqual(
		t,
		fmt.Sprint(message),
		`name:"Jim" id:42 email:"jim@example.com"`+
			` phones:<number:"555-1212" type:HOME>`)
}

var TestBinaryFormaterTestData = []byte{
	0, 1, 2, 3, 4, 5, 6, 7, 255, 255, 255, 255, 255, 255, 255, 156}

func checkBinaryValueFormater(
	t *testing.T, ctype string, nbytes int, expect string, order binary.ByteOrder,
) {
	s, err :=
		binaryValueFormatters[ctype](TestBinaryFormaterTestData[:nbytes], order)
	assertNoError(t, err)
	assertEqual(t, s, expect)
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

func TestValueFormattingBinaryFormatter(t *testing.T) {
	formatting := newValueFormatting()
	var formatter = formatting.binaryFormatter("BigEndian", "int32")
	s, err := formatter(TestBinaryFormaterTestData)
	assertNoError(t, err)
	assertEqual(t, s, "[66051 67438087 -1 -100]")
	formatter = formatting.binaryFormatter("LittleEndian", "int32")
	s, err = formatter(TestBinaryFormaterTestData)
	assertNoError(t, err)
	assertEqual(t, s, "[50462976 117835012 -1 -1660944385]")
}

func testValueFormattingPBFormatter(t *testing.T) {
	formatting := newValueFormatting()
	formatting.settings.ProtocolBufferDefinitions = append(
		formatting.settings.ProtocolBufferDefinitions,
		filepath.Join("testdata", "addressbook.proto"))
	err := formatting.setupPBMessages()
	assertNoError(t, err)

	formatter := formatting.pbFormatter("person")
	in, err := ioutil.ReadFile(filepath.Join("testdata", "person.bin"))
	assertNoError(t, err)

	text, err := formatter(in)
	assertNoError(t, err)

	assertEqual(t, text,
		`name:"Jim" id:42 email:"jim@example.com"`+
			` phones:<number:"555-1212" type:HOME>`)

	formatter = formatting.pbFormatter("not a thing")
	text, err = formatter(in)

	assertEqual(t, fmt.Sprint(err),
		"No Protocol-Buffer message time for: not a thing")
}

func TestValueFormattingValidateColumns(t *testing.T) {
	formatting := newValueFormatting()

	// Typeless encoding:
	formatting.settings.Columns["c1"] = valueFormatColumn{Encoding: "HEX"}
	err := formatting.validateColumns()
	assertEqual(t, err, nil)

	// Inherit encoding:
	formatting.settings.Columns["c1"] = valueFormatColumn{}
	formatting.settings.DefaultEncoding = "H"
	err = formatting.validateColumns()
	assertEqual(t, err, nil)

	// Inherited encoding wants a type:
	formatting.settings.DefaultEncoding = "B"
	err = formatting.validateColumns()
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\nc1: No type specified for encoding: B")

	// provide a type:
	formatting.settings.Columns["c1"] = valueFormatColumn{Type: "INT"}
	err = formatting.validateColumns()
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\nc1: Invalid type: INT for encoding: B")

	// Fix the type:
	formatting.settings.Columns["c1"] = valueFormatColumn{Type: "INT64"}
	err = formatting.validateColumns()
	assertEqual(t, err, nil)

	// Now, do a bunch of this again in a family
	family := newValueFormatFamily()
	formatting.settings.Families["f"] = family
	formatting.settings.Families["f"].Columns["c2"] = valueFormatColumn{}
	err = formatting.validateColumns()
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\nf:c2: No type specified for encoding: B")
	formatting.settings.Families["f"].Columns["c2"] =
		valueFormatColumn{Type: "int64"}
	err = formatting.validateColumns()
	assertEqual(t, err, nil)

	// Change the family encoding.  The type won't work anymore.
	family.DefaultEncoding = "p"
	formatting.settings.Families["f"] = family
	err = formatting.validateColumns()
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\nf:c2: Invalid type: int64 for encoding: p")

	// clear the type_ to make sure we get that message:
	formatting.settings.Families["f"].Columns["c2"] = valueFormatColumn{}
	err = formatting.validateColumns()
	// we're bad here because no type was specified, so we fall
	// back to the column name, which doesn't have a
	// protocol-buffer message type.
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\nf:c2: Invalid type: c2 for encoding: p")

	// Look! Multiple errors!
	formatting.settings.Columns["c1"] = valueFormatColumn{}
	err = formatting.validateColumns()
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\n"+
			"c1: No type specified for encoding: B\n"+
			"f:c2: Invalid type: c2 for encoding: p")

	// Fix the protocol-buffer problem:
	formatting.pbMessageTypes["address"] = &desc.MessageDescriptor{}
	formatting.settings.Families["f"].Columns["c2"] =
		valueFormatColumn{Type: "address"}
	err = formatting.validateColumns()
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\n"+
			"c1: No type specified for encoding: B")
}

func TestValueFormattingSetup(t *testing.T) {
	formatting := newValueFormatting()
	err := formatting.setup(map[string]string{
		"format-file": filepath.Join("testdata", t.Name()+".yml")})
	assertEqual(t, fmt.Sprint(err),
		"Bad encoding and types:\ncol1: No type specified for encoding: B")
}

func TestValueFormattingFormat(t *testing.T) {
	formatting := newValueFormatting()
	formatting.settings.ProtocolBufferDefinitions =
		append(formatting.settings.ProtocolBufferDefinitions,
			filepath.Join("testdata", "addressbook.proto"))
	family := newValueFormatFamily()
	family.DefaultEncoding = "Binary"
	formatting.settings.Families["binaries"] = family
	formatting.settings.Families["binaries"].Columns["cb"] =
		valueFormatColumn{Type: "int16"}

	formatting.settings.Columns["hexy"] =
		valueFormatColumn{Encoding: "hex"}
	formatting.settings.Columns["address"] =
		valueFormatColumn{Encoding: "p", Type: "tutorial.Person"}
	formatting.settings.Columns["person"] = valueFormatColumn{Encoding: "p"}
	err := formatting.setup(map[string]string{})

	s, err := formatting.format("", "f1", "c1", []byte("Hello world!"))
	assertEqual(t, s, "\"Hello world!\"\n")

	s, err = formatting.format("  ", "f1", "hexy", []byte("Hello world!"))
	assertNoError(t, err)
	assertEqual(t, s, "  48 65 6c 6c 6f 20 77 6f 72 6c 64 21\n")

	s, err = formatting.format("    ", "binaries", "cb", []byte("Hello world!"))
	assertNoError(t, err)
	assertEqual(t, s, "    [18533 27756 28448 30575 29292 25633]\n")

	in, err := ioutil.ReadFile(filepath.Join("testdata", "person.bin"))
	assertNoError(t, err)
	pbExpect :=
		"      name: \"Jim\"\n" +
			"      id: 42\n" +
			"      email: \"jim@example.com\"\n" +
			"      phones: <\n" +
			"        number: \"555-1212\"\n" +
			"        type: HOME\n" +
			"      >\n"

	for _, col := range []string{"address", "person"} {
		s, err = formatting.format("      ", "f1", col, in)
		assertNoError(t, err)
		assertEqual(t, s, pbExpect)
	}
}

func TestPrintRow(t *testing.T) {
	row := bigtable.Row{
		"f1": {
			bigtable.ReadItem{
				Row:    "r1",
				Column: "c1",
				Value:  []byte("Hello!"),
			},
			bigtable.ReadItem{
				Row:    "r1",
				Column: "c2",
				Value:  []byte{1, 2},
			},
		},
		"f2": {
			bigtable.ReadItem{
				Row:    "r1",
				Column: "person",
				Value: []byte("\n\x03Jim\x10*\x1a\x0fjim@example.com\"" +
					"\x0c\n\x08555-1212\x10\x01"),
			},
		},
	}
	out := captureStdout(func() { printRow(row) })
	expect :=
		"----------------------------------------\n" +
			"r1\n" +
			"  c1\n" +
			"    \"Hello!\"\n" +
			"  c2\n" +
			"    \"\\x01\\x02\"\n" +
			"  person\n" +
			"    \"\\n\\x03Jim\\x10*\\x1a\\x0fjim@example.com\\\"\\f\\n\\b555-1212\\x10\\x01\"\n" +
			""

	timestampsRE := regexp.MustCompile("[ ]+@ [^ \t\n]+")

	stripTimestamps := func(s string) string {
		return string(timestampsRE.ReplaceAll([]byte(s), []byte("")))
	}

	assertEqual(t, stripTimestamps(out), expect)

	oldValueFormatting := globalValueFormatting
	defer func() { globalValueFormatting = oldValueFormatting }()

	globalValueFormatting = newValueFormatting()
	globalValueFormatting.settings.ProtocolBufferDefinitions =
		[]string{filepath.Join("testdata", "addressbook.proto")}
	globalValueFormatting.settings.Columns["c2"] =
		valueFormatColumn{Encoding: "Binary", Type: "int16"}
	globalValueFormatting.settings.Columns["person"] =
		valueFormatColumn{Encoding: "ProtocolBuffer"}
	globalValueFormatting.setup(map[string]string{})

	expectf := ("----------------------------------------\n" +
		"r1\n" +
		"  c1\n" +
		"    \"Hello!\"\n" +
		"  c2\n" +
		"    258\n" +
		"  person\n" +
		"    name: \"Jim\"\n" +
		"    id: 42\n" +
		"    email: \"jim@example.com\"\n" +
		"    phones: <\n" +
		"      number: \"555-1212\"\n" +
		"      type: HOME\n" +
		"    >\n" +
		"")

	out = captureStdout(func() { printRow(row) })
	assertEqual(t, stripTimestamps(out), expectf)
}

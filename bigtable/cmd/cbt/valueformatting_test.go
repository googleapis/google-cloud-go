package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"cloud.google.com/go/internal/testutil"
)

func assertEqual(t *testing.T, label string, got, want interface{}) {
	if ! testutil.Equal(got, want) {
		_, fpath, lno, ok := runtime.Caller(1)
		if ok {
			_, fname := filepath.Split(fpath)
			t.Errorf("%s:%d:%s didn't match: %s", fname, lno, label, got)
		} else {
			t.Errorf("%s didn't match: %s", label, got)
		}
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
		Columns: map[string]ValueFormatColumn{
			"col3": ValueFormatColumn{
				Encoding: "P",
				Type: "person",
			},
			"col4": ValueFormatColumn{
				Encoding: "P",
				Type: "hobby",
			},
		},
		Families: map[string]ValueFormatFamily{
			"family1": ValueFormatFamily{
				DefaultEncoding: "BigEndian",
				DefaultType: "INT64",
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
	if assertNoError(t, err) { return }
	err = message.Unmarshal(in)
	if assertNoError(t, err) { return }
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

func TestValueFormattingBinaryFormatter(t *testing.T) {
	formatting := ValueFormatting{}
	var formatter = formatting.binaryFormatter("BigEndian", "int32")
	s, err := formatter(TestBinaryFormaterTestData)
	assertNoError(t, err)
	assertEqual(t, "int32", s, "[66051 67438087 -1 -100]")
	formatter = formatting.binaryFormatter("LittleEndian", "int32")
	s, err = formatter(TestBinaryFormaterTestData)
	assertNoError(t, err)
	assertEqual(t, "int32", s, "[50462976 117835012 -1 -1660944385]")
}

func testValueFormattingPBFormatter(t *testing.T) {
	formatting := ValueFormatting{}
	formatting.settings.ProtocolBuffer.Definitions = append(
		formatting.settings.ProtocolBuffer.Definitions,
		filepath.Join("testdata", "addressbook.proto"))	
	err := formatting.setupPBMessages()
	if assertNoError(t, err) { return }

	formatter := formatting.pbFormatter("person")
	in, err := ioutil.ReadFile(filepath.Join("testdata", "person.bin"))
	if assertNoError(t, err) { return }

	text, err := formatter(in)
	if assertNoError(t, err) { return }

	assertEqual(t, "formatted person", text, 
		`name:"Jim" id:42 email:"jim@example.com"` +
		` phones:<number:"555-1212" type:HOME>`)

	formatter = formatting.pbFormatter("not a thing")
	text, err = formatter(in)

	assertEqual(t, "bad pb type error", fmt.Sprint(err),
		"No Protocol-Buffer message time for: not a thing")
}

func TestValueFormattingValidateColumns(t *testing.T) {
	formatting := NewValueFormatting()

	// Typeless encoding:
	formatting.settings.Columns["c1"] = ValueFormatColumn{Encoding: "HEX"}
	err := formatting.validateColumns()
	assertEqual(t, "c1 good", err, nil)

	// Inherit encoding:
	formatting.settings.Columns["c1"] = ValueFormatColumn{}
	formatting.settings.DefaultEncoding = "H"
	err = formatting.validateColumns()
	assertEqual(t, "c1 good", err, nil)

	// Inherited encoding wants a type:
	formatting.settings.DefaultEncoding = "B"
	err = formatting.validateColumns()
	assertEqual(t, "c1 bad", fmt.Sprint(err),
		"Bad encoding and types:\nc1: No type specified for encoding: B")

	// provide a type:
	formatting.settings.Columns["c1"] = ValueFormatColumn{Type: "INT"}
	err = formatting.validateColumns()
	assertEqual(t, "c1 bad", fmt.Sprint(err),
		"Bad encoding and types:\nc1: Invalid type: INT for encoding: B")
	
	// Fix the type:
	formatting.settings.Columns["c1"] = ValueFormatColumn{Type: "INT64"}
	err = formatting.validateColumns()
	assertEqual(t, "c1 good", err, nil)

	// Now, do a bunch of this again in a family
	family := NewValueFormatFamily()
	formatting.settings.Families["f"] = family
	formatting.settings.Families["f"].Columns["c2"] = ValueFormatColumn{}
	err = formatting.validateColumns()
	assertEqual(t, "c2 bad", fmt.Sprint(err),
		"Bad encoding and types:\nf:c2: No type specified for encoding: B")
	formatting.settings.Families["f"].Columns["c2"] =
		ValueFormatColumn{Type: "int64"}
	err = formatting.validateColumns()
	assertEqual(t, "c1 good", err, nil)

	// Change the family encoding.  The type won't work anymore.
	family.DefaultEncoding = "p"
	formatting.settings.Families["f"] = family
	err = formatting.validateColumns()
	assertEqual(t, "c1 bad", fmt.Sprint(err),
		"Bad encoding and types:\nf:c2: Invalid type: int64 for encoding: p")

	// clear the type_ to make sure we get that message:
	formatting.settings.Families["f"].Columns["c2"] = ValueFormatColumn{}
	err = formatting.validateColumns()
	// we're bad here because no type was specified, so we fall
	// back to the column name, which doesn't have a
	// protocol-buffer message type.
	assertEqual(t, "c2 bad", fmt.Sprint(err),
		"Bad encoding and types:\nf:c2: Invalid type: c2 for encoding: p")

	// Look! Multiple errors!
	formatting.settings.Columns["c1"] = ValueFormatColumn{}
	err = formatting.validateColumns()
	assertEqual(t, "all bad", fmt.Sprint(err),
		"Bad encoding and types:\n" +
		"c1: No type specified for encoding: B\n" +
		"f:c2: Invalid type: c2 for encoding: p")

	// Fix the protocol-buffer problem:
	formatting.pbMessageTypes["address"] = &desc.MessageDescriptor{}
	formatting.settings.Families["f"].Columns["c2"] =
		ValueFormatColumn{Type: "address"}
	err = formatting.validateColumns()
	assertEqual(t, "all bad", fmt.Sprint(err),
		"Bad encoding and types:\n" +
		"c1: No type specified for encoding: B")
}

func TestValueFormattingSetup(t *testing.T) {
	formatting := NewValueFormatting()
	formatting.flags.formatFile = filepath.Join("testdata", t.Name() + ".yml")
	err := formatting.setup()
	assertEqual(t, "setup w bad settings", fmt.Sprint(err),
		"Bad encoding and types:\ncol1: No type specified for encoding: B")
}

func TestValueFormattingFormat(t *testing.T) {
	formatting := NewValueFormatting()
	formatting.settings.ProtocolBuffer.Definitions =
		append(formatting.settings.ProtocolBuffer.Definitions,
			filepath.Join("testdata", "addressbook.proto"))
	family := NewValueFormatFamily()
	family.DefaultEncoding="Binary"
	formatting.settings.Families["binaries"] = family
	formatting.settings.Families["binaries"].Columns["cb"] =
		ValueFormatColumn{Type: "int16"}

	formatting.settings.Columns["hexy"] =
		ValueFormatColumn{Encoding: "hex"}
	formatting.settings.Columns["address"] =
		ValueFormatColumn{Encoding: "p", Type: "tutorial.Person"}
	formatting.settings.Columns["person"] =	ValueFormatColumn{Encoding: "p"}
	err := formatting.setup()

	s, err := formatting.format("", "f1", "c1", []byte("Hello world!"))
	assertEqual(t, "q", s, `"Hello world!"`)

	s, err = formatting.format("  ", "f1", "hexy", []byte("Hello world!"))
	assertNoError(t, err)
	assertEqual(t, "q", s, "  48 65 6c 6c 6f 20 77 6f 72 6c 64 21")

	s, err = formatting.format("    ", "binaries", "cb", []byte("Hello world!"))
	assertNoError(t, err)
	assertEqual(t, "q", s, "    [18533 27756 28448 30575 29292 25633]")

	in, err := ioutil.ReadFile(filepath.Join("testdata", "person.bin"))
	if assertNoError(t, err) { return }
	pbExpect := 
		"      name: \"Jim\"\n" +
		"      id: 42\n" +
		"      email: \"jim@example.com\"\n" +
		"      phones: <\n" +
		"        number: \"555-1212\"\n" +
		"        type: HOME\n" +
		"      >"

	for _, col := range []string{"address", "person"} {
		s, err = formatting.format("      ", "f1", col, in)
		assertNoError(t, err)
		assertEqual(t, "q", s, pbExpect)
	}
}
	

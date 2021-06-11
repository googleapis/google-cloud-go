package main

import (
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

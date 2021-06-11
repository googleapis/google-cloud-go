package main

import (
	"path/filepath"
	"testing"

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

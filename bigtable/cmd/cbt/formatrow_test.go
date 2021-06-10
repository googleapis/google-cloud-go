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

func TestParseRowFormatFile(t *testing.T) {
	want := RowFormat{
		DefaultEncoding: "HEX",
		ProtocolBuffer: RowFormatProtocolBufferDefinition{
			Definitions: []string{"MyProto.proto", "MyOtherProto.proto"},
			Imports: []string{"mycode/stuff", "/home/user/dev/othercode/"},
		},
		Families: map[string]RowFormatFamily{
			"family1": RowFormatFamily{
				DefaultEncoding: "BigEndian:INT64",
				Columns: map[string]RowFormatColumn{
					"address": RowFormatColumn{
						Encoding: "PROTO",
						Type: "tutorial.Person",
					},
				},
			},
			
			
			"family2": RowFormatFamily{
				Columns: map[string]RowFormatColumn{
					"col1": RowFormatColumn{
						Encoding: "B",
						Type: "INT32",
					},
					"col2": RowFormatColumn{
						Encoding: "L",
						Type: "INT16",
					},
					"address": RowFormatColumn{
						Encoding: "PROTO",
						Type: "tutorial.Person",
					},
				},
			},
			"family3": RowFormatFamily{
				Columns: map[string]RowFormatColumn{
					"proto_col": RowFormatColumn{ 
						Encoding: "PROTO",
						Type: "MyProtoMessageType",
					},
				},
			},
			
		},
	}
	
	formats, err := parseRowFormatFile(filepath.Join("testdata", t.Name() + ".yml"))
	if err != nil {
		t.Errorf("Parse error: %s", err)
	}

	assertEqual(t, "format", formats, want)
}

package main

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
)

var sample = `

#
# Example format file
#

default_encoding: HEX

protocol_buffer:
  definitions:
    - MyProto.proto
    - MyOtherProto.proto
  imports:
    - mycode/stuff
    - /home/user/dev/othercode/

families:
  family1:
    default_encoding: BigEndian:INT64
    columns:
      address:
        encoding: PROTO
        type: tutorial.Person

  family2:
    columns:
      col1:
        encoding: B
        type: INT32
      col2:
        encoding: L
        type: INT16
      address:
        encoding: PROTO
        type: tutorial.Person

  family3:
    columns:
      proto_col: 
        encoding: PROTO
        type: MyProtoMessageType
`

func assertEqual(t *testing.T, label string, got, want interface{}) {
	if ! testutil.Equal(got, want) {
		t.Errorf("%s didn't match %s", label, got)
	}
}

func TestParseRowFormatText(t *testing.T) {
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
	
	formats, err := parseRowFormatText(sample)
	if err != nil {
		t.Errorf("Parse error: %s", err)
	}

	assertEqual(t, "format", formats, want)
}

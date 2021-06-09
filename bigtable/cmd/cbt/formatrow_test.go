package main

import (
	"testing"
	"cloud.google.com/go/internal/testutil"
)

var sample = `

#
# Example format file
#

# Everything in "family1" is binary
family1:=BINARY

# Two columns in family2 are binary
family2:col1,col2=BINARY

# Protocol buffer config

# The paths used to search for dependencies that are referenced in import
# statements in proto source files. If no import paths are provided then
# "." (current directory) is assumed to be the only import path.
proto_import_paths=mycode/stuff,/home/user/dev/othercode/

# The proto files to compile. Any referenced message types should be found here.
proto_source_files=MyProto.proto, MyOtherProto.proto

# Format one column as a specified message
family3:proto_col=MyProtoMessageType

` 

func TestParseFormats(t *testing.T) {
	
	want := map[string]string{
		"family1:": "BINARY",
		"family2:col1,col2": "BINARY",
		"proto_import_paths": "mycode/stuff,/home/user/dev/othercode/",
		"proto_source_files": "MyProto.proto, MyOtherProto.proto",
		"family3:proto_col": "MyProtoMessageType",
		}

	formats, err := ParseFormats(sample)
	if err != nil {
		t.Errorf("ParseFormats failed")
	}

	if ! testutil.Equal(formats, want) {
		t.Errorf("Didn't match %s", formats)
	}
}

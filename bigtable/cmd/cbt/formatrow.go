package main

import (
	//"flag"

	"gopkg.in/yaml.v2"
)

// type arrayFlags []string

// func (i *arrayFlags) Set(value string) error {
// 	*i = append(*i, value)
// 	return nil
// }

// func (i *arrayFlags) String() string {
// 	return "my string representation"
// }

// var rowFormatFlags struct {
// 	format string
// 	proto arrayFlags
// 	protoImport arrayFlags
// 	protoMessage arrayFlags
// 	formatFile string
// } 

// func setupRowFormatFlags() {
// 	flag.StringVar(&rowFormatFlags.format, "format", "", "Default format")
// 	flag.Var(
// 		&rowFormatFlags.proto, "proto",
// 		"Protocol buffer definition file. (May be given more than once.)")
// 	flag.Var(
// 		&rowFormatFlags.protoImport, "import",
// 		"Protocol buffer import path for dependencies.\n" +
// 		"(May be given more than once.)")
// 	flag.Var(
// 		&rowFormatFlags.protoMessage, "proto-message",
// 		"Protocol Buffer message type to use for a given column or family\n" +
// 		"of the form: 'fam:col=type\n" +
// 		"(May be given more than once.)")
// 	flag.Var(
// 		&rowFormatFlags.protoMessage, "m",
// 		"Protocol Buffer message type to use for a given column or family\n" +
// 		"of the form: 'fam:col=type\n" +
// 		"(May be given more than once.)")
// 	flag.StringVar(
// 		&rowFormatFlags.formatFile, "format-file", "",
// 		"File containing format definitions")
// }


type RowFormatColumn struct {
	Encoding string
	Type string
}

type RowFormatFamily struct {
	DefaultEncoding string `yaml:"default_encoding"`
	DefaultType string `yaml:"default_type"`
	Columns map[string]RowFormatColumn
}

type RowFormatProtocolBufferDefinition struct {
	Definitions []string
	Imports []string
}


type RowFormat struct {
	ProtocolBuffer RowFormatProtocolBufferDefinition `yaml:"protocol_buffer"`
	DefaultEncoding string `yaml:"default_encoding"`
	Families map[string]RowFormatFamily
}

func parseRowFormatText(format_data string) (RowFormat, error) {
	format := RowFormat{}
	err := yaml.UnmarshalStrict([]byte(format_data), &format)
	return format, err
}

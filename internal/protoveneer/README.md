# The protoveneer tool

Protoveneer is an experimental tool that generates idiomatic Go types that
correspond to protocol buffer messages and enums -- a veneer on top of the proto
layer.

## Usage

Call protoveneer with a config file and a directory containing *.pb.go files:

    protoveneer config.yaml ../ai/generativelanguage/apiv1beta/generativelanguagepb

That will write Go source code to the current directory, or the one specified by -outdir.

To add a license to the generated code, pass the -license flag with a filename.

See testdata/basic/config.yaml for a sample config file.

The generated code requires the "support" package. Copy this package to your
project and provide its import path as the value of `supportImportPath` in the
config file.



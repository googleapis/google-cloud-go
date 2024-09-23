# go-sentencepiece

<p align="center">
  <img alt="Logo" src="doc/toklogo2.png" />
</p>

----

[![Go Reference](https://pkg.go.dev/badge/github.com/eliben/go-sentencepiece.svg)](https://pkg.go.dev/github.com/eliben/go-sentencepiece)

This is a pure Go implementation of encoding and decoding text with
the [SentencePiece tokenizer](https://github.com/google/sentencepiece).

"Encoding" is the operation used to split text into tokens, using
a trained tokenizer model. "Decoding" is the reverse process - converting
a list of tokens into the original text.

SentencePiece is a general family of tokenizers that is configured
by a protobuf configuration file. This repository currently focuses
on implementing just the functionality required to reproduce the
tokenization of [Gemma models](https://ai.google.dev/gemma) (the same
tokenizer is used for Google's proprietary Gemini family of models).
Specifically, it only implements BPE tokenization since this is what
Gemma uses.

## Current status

This package should be ready to use for encoding text into tokens
using the Gemma tokenizer; it's been reasonably optimized and extensively
tested vs. the [SentencePiece Python bindings](https://pypi.org/project/sentencepiece/)
(see `system_test.go` in this repository).

If you find any problems or discrepancies, please open an issue.

## Tokenizer configuration

The configuration file for the tokenizer is a protobuf (structured
data, serialized in the [protocol buffer format](https://protobuf.dev/))
that describes a trained tokenizer model; it includes
the complete learned vocabulary used for tokenization, as well as
other configuration information.

It is not part of this repository. Please fetch it from the
[official Gemma implementation repository](https://github.com/google/gemma_pytorch/tree/main/tokenizer).
`NewProcessor*` constructors will expect to read this file.

## Developing

A protobuf is used to configure the tokenizer. The structure of the
protobuf is described by the `internal/model/sentencepiece_model.proto` file,
which is vendored from https://github.com/google/sentencepiece

To re-generate the `*.pb.go` file from it:

```
$ cd internal/model
$ ./gen.sh
```

The configuration protobuf itself is obtained as described in the
[Tokenizer configuration](#tokenizer-configuration) section. All
tests require the `MODELPATH` env var to point to a local
copy of the tokenizer configuration file.

## Online demo

To see an in-browser demo of this tokenizer in action, visit
https://eliben.github.io/go-sentencepiece/

The Go code is compiled to WebAssembly and loaded from a small
JS program to allow interactive encoding of text.

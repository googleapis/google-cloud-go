#!/bin/bash

set -o pipefail
set -eux

protoc \
  --go_out=. \
  --go_opt="Msentencepiece_model.proto=;model" sentencepiece_model.proto

goimports -w .


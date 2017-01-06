#!/bin/bash

# Fail on any error.
set -e

cd git/gocloud
./internal/kokoro/build.sh

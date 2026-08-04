#!/bin/bash

# Copyright 2026 Google LLC.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
go run "$SCRIPT_DIR/../../../../internal/transport/cert/cmd/test_signer.go" "$SCRIPT_DIR/../../../../internal/transport/cert/testdata/rsa2048bit.pem"

//go:build !linux

package udsthin

import "errors"

var errNoEmbedded = errors.New("no embedded gcs-core-server for this GOOS/GOARCH")

// HowStarted records which strategy produced the sidecar binary (for the report).
var HowStarted string

// embeddedServer: the extract/memfd strategies are Linux-only in the POC.
func embeddedServer() (string, error) { return "", errNoEmbedded }

//go:build linux && amd64

package udsthin

import _ "embed"

// The sidecar for this platform. Built by the pipeline (musl, static) and
// copied here before `go build`; an empty file means "not embedded".
//
//go:embed bin/gcs-core-server-linux-amd64
var serverBin []byte

//go:build !(linux && amd64)

package udsthin

// No sidecar binary is embedded for this GOOS/GOARCH in the POC; the client
// falls back to Options.ServerBinary / $GCS_CORE_SERVER / an already-running
// sidecar on the socket.
var serverBin []byte

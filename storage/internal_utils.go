package storage

import (
	"strings"
)

// ensureEndpoint fixes schemeless endpoints by prepending "https://"
// and ensuring it has the "/storage/v1/" suffix.
func ensureEndpoint(ep string) string {
	if !strings.Contains(ep, "://") {
		ep = "https://" + ep
	}
	if !strings.Contains(ep, "/storage/v1") {
		if !strings.HasSuffix(ep, "/") {
			ep += "/"
		}
		ep += "storage/v1/"
	}
	if !strings.HasSuffix(ep, "/") {
		ep += "/"
	}
	return ep
}

package storage

import (
	"strings"
)

// ensureEndpoint fixes schemeless endpoints by prepending "https://"
// and ensuring it has the "/storage/v1/" suffix.
func ensureEndpoint(ep string) string {
	// If the resolved endpoint doesn't have a scheme, it is most likely a
	// host-only string from WithEndpoint. Prepend https:// and ensure it has
	// the storage path.
	if !strings.Contains(ep, "://") {
		ep = "https://" + ep
		if !strings.Contains(ep, "/storage/v1") {
			if !strings.HasSuffix(ep, "/") {
				ep += "/"
			}
			ep += "storage/v1/"
		}
	}
	return ep
}

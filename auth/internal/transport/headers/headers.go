package headers

import (
	"net/http"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

// SetAuthHeader uses the provided token to set the Authorization and trust
// boundary headers on a request. If the token.Type is empty, the type is
// assumed to be Bearer.
func SetAuthHeader(token *auth.Token, req *http.Request) {
	typ := token.Type
	if typ == "" {
		typ = internal.TokenTypeBearer
	}
	req.Header.Set("Authorization", typ+" "+token.Value)

	headerVal, setHeader := token.TrustBoundaryData.TrustBoundaryHeader()
	if setHeader {
		req.Header.Set("x-allowed-locations", headerVal)
	}
}

// SetAuthMetadata uses the provided token to set the Authorization and trust
// boundary metadata. If the token.Type is empty, the type is assumed to be
// Bearer.
func SetAuthMetadata(token *auth.Token, m map[string]string) {
	typ := token.Type
	if typ == "" {
		typ = internal.TokenTypeBearer
	}
	m["authorization"] = typ + " " + token.Value

	headerVal, setHeader := token.TrustBoundaryData.TrustBoundaryHeader()
	if setHeader {
		m["x-allowed-locations"] = headerVal
	}
}

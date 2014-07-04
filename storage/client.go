package storage

import (
	"net/http"
)

type client struct {
	transport http.RoundTripper
}

// +build go1.5

package transport

import "net/http"

// makeReqCancel returns a closure that cancels the given http.Request
// when called.
func makeReqCancel(req *http.Request) func(http.RoundTripper) {
	c := make(chan struct{})
	req.Cancel = c
	return func(http.RoundTripper) {
		close(c)
	}
}

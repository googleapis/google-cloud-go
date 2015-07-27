// +build !go1.5

package transport

import "net/http"

// makeReqCancel returns a closure that cancels the given http.Request
// when called.
func makeReqCancel(req *http.Request) func(http.RoundTripper) {
	// Go 1.4 and prior do not have a reliable way of cancelling a request.
	// Transport.CancelRequest will only work if the request is already in-flight.
	return func(r http.RoundTripper) {
		if t, ok := r.(*http.Transport); ok {
			t.CancelRequest(req)
		}
	}
}

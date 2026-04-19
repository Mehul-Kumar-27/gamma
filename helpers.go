package gamma

import "net/http"

// RoundTripperFunc adapts an ordinary function into an [http.RoundTripper],
// mirroring the [http.HandlerFunc] pattern on the server side. It is exported
// so that callers writing their own middlewares can return a [RoundTripper]
// without defining a new struct type.
//
//	logging := func(next http.RoundTripper) http.RoundTripper {
//	    return gamma.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
//	        log.Printf("→ %s %s", req.Method, req.URL)
//	        return next.RoundTrip(req)
//	    })
//	}
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls f(req) and satisfies [http.RoundTripper].
func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

package gamma

import "net/http"

// Middleware wraps an [http.RoundTripper] and returns a new one with added
// behaviour. It is the fundamental building block of gamma — every feature
// (retry, timeout, circuit breaker, rate limit, observability hooks) is
// expressed as a Middleware so the pieces compose cleanly.
//
// Because a Middleware is just a function, users can freely write their own
// and drop them into the chain alongside the built-ins.
//
//	logging := func(next http.RoundTripper) http.RoundTripper {
//	    return gamma.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
//	        log.Printf("→ %s %s", req.Method, req.URL)
//	        return next.RoundTrip(req)
//	    })
//	}
//
//	client := gamma.NewGamma(gamma.Use(logging))
type Middleware func(http.RoundTripper) http.RoundTripper

// Chain composes middlewares into a single Middleware. The first argument is
// the outermost wrapper: it runs first on the way in and last on the way out.
//
// Given Chain(a, b, c), a request flows a → b → c → base transport, and the
// response flows back c → b → a. This matches the conventional "onion" model
// used by most HTTP middleware libraries.
//
//	chain := gamma.Chain(logging, rateLimit, retry)
//	rt    := chain(http.DefaultTransport)
//	client := &http.Client{Transport: rt}
func Chain(middlewares ...Middleware) Middleware {
	return func(base http.RoundTripper) http.RoundTripper {
		for i := len(middlewares) - 1; i >= 0; i-- {
			base = middlewares[i](base)
		}
		return base
	}
}

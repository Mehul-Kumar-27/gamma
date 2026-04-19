package gamma

import (
	"net/http"
	"time"
)

// config is the internal state assembled from [Option] values before a
// transport or client is constructed. It is not part of the public API —
// callers shape it indirectly through the Option constructors below.
type config struct {
	// base is the underlying transport that executes the actual HTTP call.
	// Middlewares are wrapped around it in the order they were added.
	base http.RoundTripper

	// middlewares is the ordered list of middlewares to install. The first
	// entry becomes the outermost wrapper (see [Chain] for semantics).
	middlewares []Middleware

	// clientTimeout is applied to the returned *http.Client via its Timeout
	// field when using [NewGamma]. It has no effect when using [NewTransport]
	// because a bare [http.RoundTripper] has no Timeout knob.
	clientTimeout time.Duration
}

// Option customises the transport or client produced by [NewTransport] and
// [NewGamma]. Options follow the functional-options pattern: each one is a
// small function that mutates the internal [config].
type Option func(*config)

// defaultConfig returns a [config] with production-ready defaults:
// [http.DefaultTransport] as the base, no middlewares, no client timeout.
func defaultConfig() *config {
	return &config{
		base:          http.DefaultTransport,
		clientTimeout: 0,
		middlewares:   nil,
	}
}

// Use appends a [Middleware] to the chain. Order matters: the first Use()
// call becomes the outermost wrapper, so it runs first on the request and
// last on the response.
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Timeout(30 * time.Second)), // outermost
//	    gamma.Use(gamma.Retry()),
//	    gamma.Use(gamma.CircuitBreaker(5, 30*time.Second)),
//	)
func Use(m Middleware) Option {
	return func(c *config) {
		c.middlewares = append(c.middlewares, m)
	}
}

// WithBase overrides the underlying [http.RoundTripper]. Defaults to
// [http.DefaultTransport]. Use this when you need to customise the base
// transport (for example, to set proxy settings, TLS config, or connection
// pool limits) while still layering gamma middlewares on top.
//
//	base := &http.Transport{MaxIdleConnsPerHost: 100}
//	client := gamma.NewGamma(
//	    gamma.WithBase(base),
//	    gamma.Use(gamma.Retry()),
//	)
func WithBase(rt http.RoundTripper) Option {
	return func(c *config) {
		c.base = rt
	}
}

// WithClientTimeout sets the [http.Client.Timeout] on the client returned by
// [NewGamma]. This is the hard ceiling the standard library enforces on the
// entire request (including redirects, connect, and read). It has no effect
// when using [NewTransport].
//
// Prefer [Timeout] middleware when you want the deadline to participate in
// the middleware pipeline (for example, applying per-attempt timeouts or
// composing with retry).
//
//	client := gamma.NewGamma(
//	    gamma.WithClientTimeout(10 * time.Second),
//	    gamma.Use(gamma.Retry()),
//	)
func WithClientTimeout(d time.Duration) Option {
	return func(c *config) {
		c.clientTimeout = d
	}
}

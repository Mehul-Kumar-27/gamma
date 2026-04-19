package gamma

import (
	"net/http"
)

// NewGamma builds a standard [*http.Client] with the gamma middleware chain
// installed as its transport. This is the recommended entry point for most
// callers — you get a fully configured client in one call and keep using the
// familiar [http.Client] API.
//
// Middlewares are applied in the order given: the first [Use] becomes the
// outermost wrapper (see [Chain] for the exact semantics). [WithBase] and
// [WithClientTimeout] may appear anywhere in the option list.
//
//	client := gamma.NewGamma(
//	    gamma.WithClientTimeout(30 * time.Second),
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryMaxAttempts(3),
//	        gamma.RetryOn(429, 503, 504),
//	    )),
//	)
//	resp, err := client.Get("https://api.example.com/data")
func NewGamma(opts ...Option) *http.Client {
	cfg := defaultConfig()
	for _, apply := range opts {
		apply(cfg)
	}
	transport := NewTransport(opts...)

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.clientTimeout,
	}
}

// NewTransport builds a composable [http.RoundTripper] from the given options.
// Use it when you need to attach the gamma middleware chain to an
// [*http.Client] you manage yourself — for example, because the client is
// shared with other code, or because you want to pin its Timeout, Jar, or
// redirect policy independently of gamma.
//
// The [WithClientTimeout] option is ignored here (a bare RoundTripper has no
// Timeout field); set [http.Client.Timeout] on your client directly instead.
//
//	rt := gamma.NewTransport(
//	    gamma.Use(gamma.Retry()),
//	    gamma.Use(gamma.Timeout(5 * time.Second)),
//	)
//	client := &http.Client{Transport: rt, Timeout: 30 * time.Second}
func NewTransport(opts ...Option) http.RoundTripper {
	cfg := defaultConfig()

	for _, apply := range opts {
		apply(cfg)
	}

	cfg.base = Chain(cfg.middlewares...)(cfg.base)
	return cfg.base
}

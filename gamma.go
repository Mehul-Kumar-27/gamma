package gamma

import (
	"net/http"
)

// Doer abstracts any type that can execute an HTTP request.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewGamma creates an *http.Client with retry-aware transport and
// exponential backoff. All behaviour is configurable via Option functions.
func NewGamma(opts ...Option) *http.Client {
	defaultOpts := defaultOptions()
	for _, apply := range opts {
		apply(defaultOpts)
	}

	totalTimeout := TotalTimeout(defaultOpts)

	transport := &GammaTransport{
		Base: defaultOpts.transport,
		opts: defaultOpts,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   totalTimeout,
	}
}

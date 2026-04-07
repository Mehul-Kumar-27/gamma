package gamma

import (
	"net/http"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}


/*
Fetcher is a wrapper around http.Client that provides a more convenient interface for fetching data.
*/
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

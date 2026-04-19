package gamma

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RetryConfig holds all tuneable knobs for the retry middleware.
// Every field has a sensible default (see [defaultRetryConfig]), so callers
// only need to override what they care about via [RetryOption] functions.
//
//	cfg := &gamma.RetryConfig{
//	    MaxAttempts:      4,
//	    RetryStatusCodes: []int{429, 502, 503},
//	    Backoff:          gamma.ExponentialBackoff(500*time.Millisecond, 2.0),
//	    Policy:           gamma.DefaultRetryPolicy,
//	}
type RetryConfig struct {
	MaxAttempts       int
	Policy            RetryPolicy
	RetryStatusCodes  []int
	Backoff           BackoffStrategy
	PerAttemptTimeout time.Duration
}

// RetryOption is a functional option that mutates a [RetryConfig].
// Pass one or more RetryOption values to [Retry] to customize behaviour.
//
//	gamma.Retry(
//	    gamma.RetryMaxAttempts(5),
//	    gamma.RetryOn(429, 503),
//	)
type RetryOption func(*RetryConfig)

// RetryMaxAttempts sets the total number of attempts (initial + retries).
// For example, RetryMaxAttempts(3) means one initial request plus two retries.
// The default is 2.
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryMaxAttempts(5),
//	    )),
//	)
func RetryMaxAttempts(n int) RetryOption {
	return func(c *RetryConfig) {
		c.MaxAttempts = n
	}
}

// RetryOn replaces the default set of retryable HTTP status codes.
// Only responses whose status code appears in codes will be retried
// (in addition to network-level errors, which are always retried by the
// default policy).
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryOn(429, 502, 503, 504),
//	    )),
//	)
func RetryOn(codes ...int) RetryOption {
	return func(c *RetryConfig) {
		c.RetryStatusCodes = codes
	}
}

// RetryWithBackoff overrides the delay strategy used between retry attempts.
// The default is [ExponentialBackoff] with a 1 s base and a factor of 2.0.
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryWithBackoff(gamma.ExponentialBackoff(200*time.Millisecond, 3.0)),
//	    )),
//	)
func RetryWithBackoff(b BackoffStrategy) RetryOption {
	return func(c *RetryConfig) {
		c.Backoff = b
	}
}

// RetryWithPolicy overrides the function that decides whether a failed request
// should be retried. The default is [DefaultRetryPolicy], which retries on
// network errors and any status code listed in RetryStatusCodes.
//
//	idempotentOnly := func(resp *http.Response, err error, codes []int) bool {
//	    if resp != nil && resp.Request.Method == http.MethodPost {
//	        return false
//	    }
//	    return gamma.DefaultRetryPolicy(resp, err, codes)
//	}
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryWithPolicy(idempotentOnly),
//	    )),
//	)
func RetryWithPolicy(p RetryPolicy) RetryOption {
	return func(c *RetryConfig) {
		c.Policy = p
	}
}

// RetryPerAttemptTimeout sets a per-attempt deadline. Each individual round-trip
// is cancelled if it exceeds this duration, and the next retry fires.
// A zero value (the default) means no per-attempt timeout — only the overall
// request context governs cancellation.
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryPerAttemptTimeout(2*time.Second),
//	    )),
//	)
func RetryPerAttemptTimeout(d time.Duration) RetryOption {
	return func(c *RetryConfig) {
		c.PerAttemptTimeout = d
	}
}

// defaultRetryConfig returns a [RetryConfig] populated with production-ready
// defaults: 2 attempts, exponential backoff (1 s base, factor 2),
// retry on 429 / 503 / 504, and no per-attempt timeout.
func defaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:       2,
		Policy:            DefaultRetryPolicy,
		RetryStatusCodes:  []int{429, 503, 504},
		Backoff:           ExponentialBackoff(time.Second, 2.0),
		PerAttemptTimeout: 0,
	}
}

// Retry returns a middleware that has a baked in default configuration for retry.
// This is the simplest way to get started with retry.
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry()),
//	)
//	resp, err := client.Get("https://api.example.com/data")
//
// It also supports custom configuration via RetryOption functions.
//
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Retry(
//	        gamma.RetryMaxAttempts(3),
//	        gamma.RetryOn(429, 503, 504),
//	        gamma.RetryWithBackoff(gamma.ExponentialBackoff(time.Second, 2.0)),
//	    )),
//	)
//	resp, err := client.Get("https://api.example.com/data")
func Retry(opts ...RetryOption) Middleware {
	cfg := defaultRetryConfig()

	for _, apply := range opts {
		apply(cfg)
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return &retryTransport{next: next, cfg: cfg}
	}
}

// retryTransport is an [http.RoundTripper] decorator that replays a request
// according to the policy and backoff strategy in its [RetryConfig].
// It buffers the request body so it can be re-sent on each attempt.
type retryTransport struct {
	next http.RoundTripper
	cfg  *RetryConfig
}

// RoundTrip satisfies [http.RoundTripper]. It delegates to do, which contains
// the retry loop.
func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cfg := rt.cfg
	if overrides, ok := getOverrides(req.Context()); ok {
		cfg = mergeOverrides(rt.cfg, overrides)
	}
	return rt.do(req, cfg)
}

// do executes the retry loop. It reads and buffers the request body once,
// then clones the request for each attempt so that the original is never
// consumed. Between attempts it sleeps for the duration returned by the
// configured [BackoffStrategy], honouring context cancellation at every step.
func (rt *retryTransport) do(req *http.Request, cfg *RetryConfig) (*http.Response, error) {
	var bodyBytes []byte

	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("gamma: failed to read request body: %w", err)
		}
		req.Body.Close()
	}

	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, fmt.Errorf("gamma: context cancelled: %w", err)
		}

		var ctx context.Context
		var cancel context.CancelFunc

		if cfg.PerAttemptTimeout > 0 {
			ctx, cancel = context.WithTimeout(req.Context(), cfg.PerAttemptTimeout)
		} else {
			ctx = req.Context()
		}

		fork := req.Clone(ctx)
		if bodyBytes != nil {
			fork.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := rt.next.RoundTrip(fork)

		if cancel != nil {
			cancel()
		}

		if !cfg.Policy(resp, err, cfg.RetryStatusCodes) {
			return resp, err
		}

		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("gamma: retryable status %d", resp.StatusCode)
			lastResp = resp
			drainAndClose(resp)
		}

		if attempt < cfg.MaxAttempts-1 {
			delay := cfg.Backoff.Delay(attempt, lastResp)
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				return nil, fmt.Errorf("gamma: context cancelled: %w", req.Context().Err())
			}
		}

	}

	return nil, fmt.Errorf("gamma: all %d attempts failed: %w", cfg.MaxAttempts, lastErr)
}

// drainAndClose fully reads and closes the response body so the underlying
// TCP connection can be reused by the transport's connection pool.
func drainAndClose(resp *http.Response) {
	if resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func mergeOverrides(cfg *RetryConfig, overrides *Overrides) *RetryConfig {
	merged := *cfg
	if overrides.MaxAttempts != nil {
		merged.MaxAttempts = *overrides.MaxAttempts
	}
	if overrides.RetryStatusCodes != nil {
		merged.RetryStatusCodes = overrides.RetryStatusCodes
	}
	if overrides.Backoff != nil {
		merged.Backoff = overrides.Backoff
	}
	if overrides.PerAttemptTimeout != nil {
		merged.PerAttemptTimeout = *overrides.PerAttemptTimeout
	}
	return &merged
}

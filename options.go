package gamma

import (
	"context"
	"net/http"
	"time"
)

const (
	DEFAULT_RETRIES            = 2
	DEFAULT_RETRY_DELAY        = 1 * time.Second
	DEFAULT_BACKOFF_MULTIPLIER = 2
)

var DEFAULT_RETRY_STATUS_CODES = []int{
	http.StatusTooManyRequests,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
}

// Options holds all configuration for a Gamma client.
type Options struct {
	Retries           int
	RetryDelay        time.Duration
	BackoffMultiplier float64
	parentCtx         context.Context
	transport         http.RoundTripper
	Timeout           time.Duration
	RetryStatusCodes  []int
	RetryPolicy       RetryPolicy
}

// Option is a functional option for configuring a Gamma client.
type Option func(*Options)

func WithRetries(retries int) Option {
	return func(o *Options) {
		o.Retries = retries
	}
}

func WithRetryDelay(retryDelay time.Duration) Option {
	return func(o *Options) {
		o.RetryDelay = retryDelay
	}
}

func WithBackoffMultiplier(backoffMultiplier float64) Option {
	return func(o *Options) {
		o.BackoffMultiplier = backoffMultiplier
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *Options) {
		o.parentCtx = ctx
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		o.Timeout = timeout
	}
}

func WithTransport(transport http.RoundTripper) Option {
	return func(o *Options) {
		o.transport = transport
	}
}

func WithRetryStatusCodes(statusCodes ...int) Option {
	return func(o *Options) {
		o.RetryStatusCodes = statusCodes
	}
}

func WithRetryPolicy(retryPolicy RetryPolicy) Option {
	return func(o *Options) {
		o.RetryPolicy = retryPolicy
	}
}

func defaultOptions() *Options {
	return &Options{
		Retries:           DEFAULT_RETRIES,
		RetryDelay:        DEFAULT_RETRY_DELAY,
		BackoffMultiplier: DEFAULT_BACKOFF_MULTIPLIER,
		parentCtx:         context.Background(),
		transport:         http.DefaultTransport,
		Timeout:           0,
		RetryStatusCodes:  DEFAULT_RETRY_STATUS_CODES,
		RetryPolicy:       DefaultRetryPolicy,
	}
}

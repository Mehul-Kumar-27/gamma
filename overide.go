package gamma

import (
	"context"
	"net/http"
	"time"
)

// overrideKey is the private context key under which per-request [Overrides]
// are stored. Defining it as an unexported struct type avoids collisions with
// any other package that may also use [context.WithValue] on the request.
type overrideKey struct{}

// Overrides holds per-request configuration that takes precedence over the
// defaults baked into the middleware chain. Fields use pointers where the
// zero value is a meaningful setting (for example, 0 retries), so that an
// "unset" override can be distinguished from "explicitly set to zero".
//
// Callers don't usually construct an Overrides directly — they use
// [WithOverrides] together with the Override* option constructors.
type Overrides struct {
	// MaxAttempts, when non-nil, replaces [RetryConfig.MaxAttempts] for this
	// request only.
	MaxAttempts *int

	// RetryStatusCodes, when non-nil, replaces [RetryConfig.RetryStatusCodes]
	// for this request only.
	RetryStatusCodes []int

	// Backoff, when non-nil, replaces [RetryConfig.Backoff] for this request
	// only.
	Backoff BackoffStrategy

	// PerAttemptTimeout, when non-nil, replaces
	// [RetryConfig.PerAttemptTimeout] for this request only.
	PerAttemptTimeout *time.Duration
}

// OverrideOption mutates an [Overrides] value. Pass one or more to
// [WithOverrides] to attach per-request settings to an [http.Request].
type OverrideOption func(*Overrides)

// OverrideRetries sets the maximum number of attempts for this request,
// overriding whatever was configured on the retry middleware.
//
//	// this specific request should not be retried
//	req = gamma.WithOverrides(req, gamma.OverrideRetries(1))
func OverrideRetries(n int) OverrideOption {
	return func(o *Overrides) { o.MaxAttempts = &n }
}

// OverrideBackoff sets the [BackoffStrategy] for this request, overriding
// the one configured on the retry middleware.
//
//	req = gamma.WithOverrides(req,
//	    gamma.OverrideBackoff(gamma.ConstantBackoff(5 * time.Second)),
//	)
func OverrideBackoff(b BackoffStrategy) OverrideOption {
	return func(o *Overrides) { o.Backoff = b }
}

// OverridePerAttemptTimeout sets the per-attempt deadline for this request,
// overriding the value configured on the retry middleware.
//
//	// this endpoint's backend is known to be slow
//	req = gamma.WithOverrides(req,
//	    gamma.OverridePerAttemptTimeout(15 * time.Second),
//	)
func OverridePerAttemptTimeout(d time.Duration) OverrideOption {
	return func(o *Overrides) { o.PerAttemptTimeout = &d }
}

// WithOverrides attaches per-request configuration to the request via its
// context. Middlewares that support overrides (for example, the retry
// middleware) read these values through [getOverrides] and merge them on
// top of their own defaults.
//
//	req, _ := http.NewRequest("POST", "https://api.example.com/pay", body)
//	req = gamma.WithOverrides(req,
//	    gamma.OverrideRetries(1),
//	    gamma.OverridePerAttemptTimeout(15*time.Second),
//	)
//	resp, err := client.Do(req)
func WithOverrides(req *http.Request, opts ...OverrideOption) *http.Request {
	o := &Overrides{}
	for _, apply := range opts {
		apply(o)
	}
	ctx := context.WithValue(req.Context(), overrideKey{}, o)
	return req.WithContext(ctx)
}

// getOverrides retrieves the [Overrides] previously attached via
// [WithOverrides]. The second return value is false when no overrides
// are present. This is unexported because it is only useful to middleware
// implementations inside the gamma package.
func getOverrides(ctx context.Context) (*Overrides, bool) {
	o, ok := ctx.Value(overrideKey{}).(*Overrides)
	return o, ok
}

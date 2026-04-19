package gamma

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns a middleware that enforces a deadline on the request
// context. Its meaning depends on where you place it in the chain:
//
//   - Outside (before) [Retry] — acts as an overall timeout across every
//     attempt plus the backoff waits between them. Once d elapses the
//     context is cancelled and the retry loop gives up.
//
//   - Inside (after) [Retry] — acts as a per-attempt timeout, because the
//     retry middleware re-enters the rest of the chain on every iteration
//     and each iteration gets a fresh Timeout context.
//
// If you only need a per-attempt deadline, prefer [RetryPerAttemptTimeout]
// — it lives inside the retry config and is both simpler and more
// discoverable. Reach for this middleware when you want an overall cap
// or want the deadline expressed as a composable pipeline stage.
//
//	// overall: 30s hard cap across all retries + backoff
//	client := gamma.NewGamma(
//	    gamma.Use(gamma.Timeout(30 * time.Second)),
//	    gamma.Use(gamma.Retry()),
//	)
//
//	// per-attempt: each individual attempt capped at 5s
//	client = gamma.NewGamma(
//	    gamma.Use(gamma.Retry()),
//	    gamma.Use(gamma.Timeout(5 * time.Second)),
//	)
func Timeout(d time.Duration) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			ctx, cancel := context.WithTimeout(req.Context(), d)
			defer cancel()
			return next.RoundTrip(req.WithContext(ctx))
		})
	}
}

package gamma

import (
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// BackoffStrategy determines the delay between retry attempts.
// Implementations receive the current attempt number (zero-indexed)
// and the HTTP response (which may be nil for network-level errors).
//
//	strategy := gamma.ExponentialBackoff(time.Second, 2.0)
//	delay := strategy.Delay(3, resp) // 1s * 2^3 = 8s
type BackoffStrategy interface {
	Delay(attempt int, resp *http.Response) time.Duration
}

// BackoffFunc is an adapter that lets ordinary functions satisfy [BackoffStrategy].
//
//	custom := gamma.BackoffFunc(func(attempt int, resp *http.Response) time.Duration {
//	    return time.Duration(attempt+1) * 500 * time.Millisecond
//	})
type BackoffFunc func(attempt int, resp *http.Response) time.Duration

// Delay calls f(attempt, resp).
func (f BackoffFunc) Delay(attempt int, resp *http.Response) time.Duration {
	return f(attempt, resp)
}

// ExponentialBackoff returns a strategy that waits base * factor^attempt.
// If the response contains a Retry-After header, that value takes precedence.
//
//	// 1s, 2s, 4s, 8s, …
//	backoff := gamma.ExponentialBackoff(time.Second, 2.0)
//
//	// 500ms, 1.5s, 4.5s, 13.5s, …
//	backoff = gamma.ExponentialBackoff(500*time.Millisecond, 3.0)
func ExponentialBackoff(base time.Duration, factor float64) BackoffStrategy {
	return BackoffFunc(func(attempt int, resp *http.Response) time.Duration {
		if d := parseRetryAfter(resp); d > 0 {
			return d
		}
		return time.Duration(float64(base) * math.Pow(factor, float64(attempt)))
	})
}

// ExponentialJitterBackoff is like [ExponentialBackoff] but adds random jitter
// to avoid thundering-herd problems. The delay is uniformly distributed in
// [half, full] where full = base * factor^attempt.
// If the response contains a Retry-After header, that value takes precedence.
//
//	// jittered delays centred around 1s, 2s, 4s, …
//	backoff := gamma.ExponentialJitterBackoff(time.Second, 2.0)
func ExponentialJitterBackoff(base time.Duration, factor float64) BackoffStrategy {
	return BackoffFunc(func(attempt int, resp *http.Response) time.Duration {
		if d := parseRetryAfter(resp); d > 0 {
			return d
		}

		full := float64(base) * math.Pow(factor, float64(attempt))
		half := full / 2
		return time.Duration(half + rand.Float64()*half)
	})
}

// ConstantBackoff returns a strategy that always waits the same duration,
// regardless of the attempt number.
// If the response contains a Retry-After header, that value takes precedence.
//
//	// always wait 200ms between retries
//	backoff := gamma.ConstantBackoff(200 * time.Millisecond)
func ConstantBackoff(d time.Duration) BackoffStrategy {
	return BackoffFunc(func(_ int, resp *http.Response) time.Duration {
		if d := parseRetryAfter(resp); d > 0 {
			return d
		}
		return d
	})
}

// parseRetryAfter extracts the Retry-After header from the response and
// returns the delay as a time.Duration. Returns 0 if the header is absent
// or unparseable.
func parseRetryAfter(resp *http.Response) time.Duration {
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return 0
}

// AdaptiveRules maps error categories to dedicated [BackoffStrategy] implementations.
// Use with [AdaptiveBackoff] to apply different retry policies depending on the
// type of failure.
type AdaptiveRules struct {
	// OnRateLimit is used when the server responds with 429 Too Many Requests.
	OnRateLimit BackoffStrategy
	// OnServerError is used for 5xx responses.
	OnServerError BackoffStrategy
	// OnConnReset is used when the response is nil (network-level failure).
	OnConnReset BackoffStrategy
	// Default is the fallback for any other retryable error.
	Default BackoffStrategy
}

func defaultAdaptiveRules() AdaptiveRules {
	return AdaptiveRules{
		OnRateLimit:   ExponentialBackoff(2*time.Second, 3.0),
		OnServerError: ExponentialJitterBackoff(time.Second, 2.0),
		OnConnReset:   ConstantBackoff(100 * time.Millisecond),
		Default:       ExponentialBackoff(time.Second, 2.0),
	}
}

// AdaptiveOption configures an [AdaptiveRules] set via [AdaptiveBackoff].
type AdaptiveOption func(*AdaptiveRules)

// AdaptiveOnRateLimit overrides the backoff used for 429 responses.
//
//	backoff := gamma.AdaptiveBackoff(
//	    gamma.AdaptiveOnRateLimit(gamma.ConstantBackoff(5 * time.Second)),
//	)
func AdaptiveOnRateLimit(b BackoffStrategy) AdaptiveOption {
	return func(rules *AdaptiveRules) {
		rules.OnRateLimit = b
	}
}

// AdaptiveDefault overrides the fallback backoff used when no specific rule matches.
func AdaptiveDefault(b BackoffStrategy) AdaptiveOption {
	return func(rules *AdaptiveRules) {
		rules.Default = b
	}
}

// AdaptiveBackoff returns a composite strategy that selects a backoff policy
// based on the type of failure. By default it uses:
//   - 429 rate-limit:  exponential backoff (2s base, 3x factor)
//   - 5xx server error: exponential backoff with jitter (1s base, 2x factor)
//   - nil response (network error): constant 100ms
//   - everything else: exponential backoff (1s base, 2x factor)
//
// Pass [AdaptiveOption] values to override individual rules.
//
//	// use defaults
//	backoff := gamma.AdaptiveBackoff()
//
//	// override rate-limit strategy
//	backoff = gamma.AdaptiveBackoff(
//	    gamma.AdaptiveOnRateLimit(gamma.ExponentialJitterBackoff(3*time.Second, 2.0)),
//	)
func AdaptiveBackoff(opts ...AdaptiveOption) BackoffStrategy {
	rules := defaultAdaptiveRules()
	for _, apply := range opts {
		apply(&rules)
	}

	return BackoffFunc(func(attempt int, resp *http.Response) time.Duration {

		if resp != nil {
			switch {
			case resp.StatusCode == 429 && rules.OnRateLimit != nil:
				return rules.OnRateLimit.Delay(attempt, resp)
			case resp.StatusCode >= 500 && rules.OnServerError != nil:
				return rules.OnServerError.Delay(attempt, resp)
			}
		} else if rules.OnConnReset != nil {
			// nil response means network-level error
			return rules.OnConnReset.Delay(attempt, resp)
		}

		return rules.Default.Delay(attempt, resp)
	})
}

package gamma

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// Backoff returns the delay before the next retry. If the response carries a
// Retry-After header, that value is used instead. Otherwise the delay is
// baseDelay * factor^attempt, optionally with jitter to avoid thundering-herd
// problems (see https://en.wikipedia.org/wiki/Thundering_herd_problem).
func Backoff(baseDelay time.Duration, factor float64, attempt int, jittered bool, resp *http.Response) time.Duration {
	if resp != nil {
		retryAfter := parseRetryAfter(resp)
		if retryAfter > 0 {
			return retryAfter
		}
	}

	backoff := baseDelay * time.Duration(math.Pow(factor, float64(attempt)))
	if jittered {
		halfJitter := backoff / 2
		backoff = halfJitter + time.Duration(rand.Intn(int(backoff)))
	}
	return backoff
}

// TotalTimeout sums the backoff delays across all retry attempts to derive
// an overall client timeout.
func TotalTimeout(o *Options) time.Duration {
	var totalDuration time.Duration

	for attempt := 0; attempt < o.Retries; attempt++ {
		totalDuration += Backoff(o.RetryDelay, o.BackoffMultiplier, attempt, false, nil)
	}

	return totalDuration
}

func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}

	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0
	}

	retryAfter, err := strconv.Atoi(header)
	if err != nil {
		return 0
	}
	return time.Duration(retryAfter) * time.Second
}

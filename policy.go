package gamma

import (
	"net/http"
)

// RetryPolicy decides whether a failed request should be retried based on the
// response, error, and the set of retryable status codes.
type RetryPolicy func(resp *http.Response, err error, retryStatusCodes []int) (shouldRetry bool)

// DefaultRetryPolicy retries on any network error or when the response status
// code matches one of the configured retryable codes.
func DefaultRetryPolicy(resp *http.Response, err error, retryStatusCodes []int) (shouldRetry bool) {
	if err != nil {
		return true
	}

	for _, statusCode := range retryStatusCodes {
		if resp.StatusCode == statusCode {
			return true
		}
	}

	return false
}

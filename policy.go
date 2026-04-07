package gamma

import (
	"net/http"
)

type RetryPolicy func(resp *http.Response, err error, retryStatusCodes []int) (shouldRetry bool)

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

package gamma

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GammaTransport struct {
	Base http.RoundTripper
	opts *Options
}

/*
GammaTransport is a wrapper around the base transport that implements the retry policy.
*/
func (g *GammaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	ctx := g.opts.parentCtx

	timeOut := g.opts.Timeout

	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt < g.opts.Retries; attempt++ {
		if err := ctx.Err(); err != nil {
			lastErr = fmt.Errorf("context cancelled: %w", err)
			break
		}

		requestFork := req.Clone(ctx)
		if bodyBytes != nil {
			requestFork.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		attemptCtx, attemptCancel := context.WithTimeout(ctx, timeOut)
		requestFork.WithContext(attemptCtx)

		resp, err := g.Base.RoundTrip(requestFork)
		attemptCancel()

		shouldRetry := g.opts.RetryPolicy(resp, err, g.opts.RetryStatusCodes)
		if !shouldRetry {
			return resp, err
		}

		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			lastResp = resp
			drainAndClose(resp)
		}

		if attempt >= g.opts.Retries-1 {
			break
		}

		backoff := Backoff(g.opts.RetryDelay, g.opts.BackoffMultiplier, attempt, false, lastResp)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			lastErr = fmt.Errorf("context cancelled: %w", ctx.Err())
			break
		}

	}

	return nil, lastErr
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

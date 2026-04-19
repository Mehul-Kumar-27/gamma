# Gamma

**Gamma is a composable resilience layer for Go HTTP clients.** Retry, timeouts, custom backoff, per-request overrides — each one a middleware you plug into a standard `*http.Client`. No custom client type, no custom request type, no surprises for callers.

```
client.Do(req)
  └─► gamma middleware chain
        ├─► Timeout              (optional, outermost)
        ├─► Retry                (with backoff + per-attempt timeout)
        └─► http.DefaultTransport (actual HTTP call)
```

## Install

```bash
go get github.com/Mehul-Kumar-27/gamma
```

Requires Go 1.22+.

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/Mehul-Kumar-27/gamma"
)

func main() {
    client := gamma.NewGamma(
        gamma.Use(gamma.Retry(
            gamma.RetryMaxAttempts(3),
            gamma.RetryPerAttemptTimeout(5*time.Second),
        )),
    )

    resp, err := client.Get("https://httpbin.org/status/200")
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    fmt.Println("Status:", resp.StatusCode)
}
```

That's it. `NewGamma` returns a standard `*http.Client` — every existing tool that accepts one keeps working.

## Why Middleware?

The old monolithic retry client had one big knob bag. Gamma replaces it with small, composable pieces:

- **You pick what you need.** Just want retry? One `Use()` call. Want retry + timeout + rate limiting later? Three `Use()` calls.
- **You control the order.** Timeout outside retry = overall cap. Timeout inside retry = per-attempt cap. Same middleware, different meaning based on position.
- **You can bring your own.** Any `func(http.RoundTripper) http.RoundTripper` is a valid middleware. Logging, tracing, auth headers, feature flags — drop them into the chain.

## Features

### Retry

Automatic retry with configurable policy, backoff, and per-attempt timeout.

```go
client := gamma.NewGamma(
    gamma.Use(gamma.Retry(
        gamma.RetryMaxAttempts(5),
        gamma.RetryOn(429, 502, 503, 504),
        gamma.RetryWithBackoff(gamma.ExponentialJitterBackoff(time.Second, 2.0)),
        gamma.RetryPerAttemptTimeout(3 * time.Second),
    )),
)
```

| Option | Default | What it does |
| --- | --- | --- |
| `RetryMaxAttempts(n)` | `2` | Total attempts (initial + retries) |
| `RetryOn(codes...)` | `429, 503, 504` | Status codes that trigger a retry |
| `RetryWithBackoff(b)` | `ExponentialBackoff(1s, 2.0)` | Delay strategy between attempts |
| `RetryPerAttemptTimeout(d)` | `0` (disabled) | Hard deadline for each individual attempt |
| `RetryWithPolicy(fn)` | `DefaultRetryPolicy` | Custom decision function for "should retry?" |

**About `RetryPerAttemptTimeout`:** if a single attempt hangs (zombie TCP connection, server GC pause, dead replica), it would otherwise burn through your entire retry budget. This timeout cancels the stuck attempt so the retry loop actually gets to retry. Envoy, AWS SDK, and gRPC all have the same field — see the docs page linked below for the rationale.

### Backoff Strategies

```go
// Plain exponential — 1s, 2s, 4s, 8s…
gamma.ExponentialBackoff(time.Second, 2.0)

// Exponential with jitter — avoids thundering herd
gamma.ExponentialJitterBackoff(time.Second, 2.0)

// Fixed wait between every attempt
gamma.ConstantBackoff(200 * time.Millisecond)

// Different strategies for different failure types
gamma.AdaptiveBackoff(
    gamma.AdaptiveOnRateLimit(gamma.ExponentialBackoff(2*time.Second, 3.0)),
    gamma.AdaptiveDefault(gamma.ExponentialJitterBackoff(time.Second, 2.0)),
)
```

All strategies honour the `Retry-After` header when the server sends one.

Implement your own with a plain function:

```go
custom := gamma.BackoffFunc(func(attempt int, resp *http.Response) time.Duration {
    return time.Duration(attempt+1) * 500 * time.Millisecond
})
```

### Timeout

Standalone timeout middleware. Its meaning depends on where you place it in the chain:

```go
// Overall cap — 30s across all retries + backoff combined
gamma.NewGamma(
    gamma.Use(gamma.Timeout(30 * time.Second)),
    gamma.Use(gamma.Retry(gamma.RetryMaxAttempts(3))),
)

// Per-attempt — same as RetryPerAttemptTimeout, via pure composition
gamma.NewGamma(
    gamma.Use(gamma.Retry(gamma.RetryMaxAttempts(3))),
    gamma.Use(gamma.Timeout(5 * time.Second)),
)

// Both — defense in depth
gamma.NewGamma(
    gamma.Use(gamma.Timeout(30 * time.Second)),  // overall
    gamma.Use(gamma.Retry(
        gamma.RetryMaxAttempts(3),
        gamma.RetryPerAttemptTimeout(5 * time.Second),  // per-attempt
    )),
)
```

### Per-Request Overrides

Different endpoints need different behaviour. Attach overrides to a specific request via its context — they take precedence over the middleware defaults for that request only.

```go
req, _ := http.NewRequest("POST", "https://api.example.com/pay", body)
req = gamma.WithOverrides(req,
    gamma.OverrideRetries(1),                           // don't retry payments
    gamma.OverridePerAttemptTimeout(15 * time.Second),  // payment gateway is slow
)
resp, err := client.Do(req)
```

Available overrides: `OverrideRetries`, `OverrideBackoff`, `OverridePerAttemptTimeout`.

### Custom Middleware

Any `func(http.RoundTripper) http.RoundTripper` is a valid middleware.

```go
logging := func(next http.RoundTripper) http.RoundTripper {
    return gamma.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
        start := time.Now()
        resp, err := next.RoundTrip(req)
        log.Printf("%s %s → %v in %s", req.Method, req.URL, statusOf(resp, err), time.Since(start))
        return resp, err
    })
}

client := gamma.NewGamma(
    gamma.Use(logging),
    gamma.Use(gamma.Retry()),
)
```

### Transport vs Client

Two constructors depending on how much control you need:

```go
// Returns a ready-to-use *http.Client
client := gamma.NewGamma(gamma.Use(gamma.Retry()))

// Returns a bare http.RoundTripper — plug it into your own client
rt := gamma.NewTransport(gamma.Use(gamma.Retry()))
client := &http.Client{
    Transport: rt,
    Timeout:   30 * time.Second,
    Jar:       myJar,
}
```

## Middleware Ordering

Middlewares are applied in the order they're added. The first `Use()` becomes the **outermost** wrapper — it runs first on the way in and last on the way out.

```go
gamma.NewGamma(
    gamma.Use(gamma.Timeout(30 * time.Second)),   // runs first
    gamma.Use(logging),
    gamma.Use(gamma.Retry()),                     // closest to the network
)

// request flow:  Timeout → logging → Retry → http.DefaultTransport
// response flow: http.DefaultTransport → Retry → logging → Timeout
```

The placement of `Timeout` relative to `Retry` is the most common gotcha — see the Timeout section above.

## File Layout

```
gamma.go        NewGamma, NewTransport
middleware.go   Middleware type, Chain
options.go      Option, Use, WithBase, WithClientTimeout
helpers.go      RoundTripperFunc
retry.go        Retry middleware + options
backoff.go      BackoffStrategy, Exponential, Jitter, Constant, Adaptive
timeout.go      Standalone Timeout middleware
overide.go      Per-request Overrides via context
policy.go       RetryPolicy, DefaultRetryPolicy
```

## Documentation

For a deeper walkthrough with design rationale, a complete API reference, and runnable recipes, see [docs/index.html](docs/index.html).

## License

MIT

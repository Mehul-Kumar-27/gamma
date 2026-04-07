# Gamma

A lightweight Go HTTP client with built-in retries, exponential backoff, and configurable retry policies. Gamma wraps the standard `*http.Client` at the transport layer, so retries happen transparently — no changes to your existing HTTP code.

## Install

```bash
go get github.com/Mehul-Kumar-27/gamma
```

## Quick Start

```go
package main

import (
    "fmt"
    "io"
    "log"
    "net/http"

    "github.com/Mehul-Kumar-27/gamma"
)

func main() {
    client := gamma.NewGamma()

    resp, err := client.Get("https://httpbin.org/get")
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    fmt.Println(string(body))
}
```

## How It Works

`NewGamma()` returns a standard `*http.Client` whose transport is replaced with `GammaTransport` — a retry-aware `http.RoundTripper`. Every request automatically goes through the retry loop with exponential backoff, so you use the client exactly like you would a plain `http.Client`.

```
client.Do(req)
  └─► GammaTransport.RoundTrip(req)     ← retry loop lives here
        └─► http.DefaultTransport.RoundTrip(req)   ← actual HTTP call
```

## Configuration

Gamma uses the functional options pattern. All options are optional — sensible defaults are provided.

```go
client := gamma.NewGamma(
    gamma.WithRetries(5),
    gamma.WithRetryDelay(2 * time.Second),
    gamma.WithBackoffMultiplier(3.0),
    gamma.WithTimeout(10 * time.Second),
    gamma.WithContext(ctx),
    gamma.WithRetryStatusCodes(http.StatusTooManyRequests, http.StatusServiceUnavailable),
    gamma.WithRetryPolicy(gamma.DefaultRetryPolicy),
    gamma.WithTransport(&http.Transport{
        MaxIdleConns:    100,
        IdleConnTimeout: 90 * time.Second,
    }),
)
```

### Options Reference

| Option | Default | Description |
|---|---|---|
| `WithRetries(n)` | `2` | Maximum number of attempts |
| `WithRetryDelay(d)` | `1s` | Base delay between retries |
| `WithBackoffMultiplier(f)` | `2.0` | Multiplier for exponential backoff |
| `WithTimeout(d)` | `0` (no per-attempt timeout) | Timeout for each individual attempt |
| `WithContext(ctx)` | `context.Background()` | Parent context for all requests |
| `WithTransport(rt)` | `http.DefaultTransport` | Base transport (wrapped by `GammaTransport`) |
| `WithRetryStatusCodes(codes...)` | `429, 503, 504` | HTTP status codes that trigger a retry |
| `WithRetryPolicy(fn)` | `DefaultRetryPolicy` | Custom function to decide whether to retry |

## Retry Behavior

### Default Retry Policy

The built-in policy retries when:
- The request returned a **network error** (timeout, connection refused, etc.)
- The response status code matches one of the configured retry status codes (`429`, `503`, `504` by default)

### Exponential Backoff

Delay between retries grows exponentially:

```
attempt 0: baseDelay * factor^0  →  1s
attempt 1: baseDelay * factor^1  →  2s
attempt 2: baseDelay * factor^2  →  4s
...
```

If the server responds with a `Retry-After` header (in seconds), that value takes priority over the calculated backoff.

### Custom Retry Policy

```go
client := gamma.NewGamma(
    gamma.WithRetryPolicy(func(resp *http.Response, err error, codes []int) bool {
        if err != nil {
            return true
        }
        return resp.StatusCode >= 500
    }),
)
```

## Architecture

```
gamma.go        Entry point — NewGamma() constructor
options.go      Functional options and defaults
transport.go    GammaTransport — retry-aware http.RoundTripper
backoff.go      Exponential backoff and Retry-After parsing
policy.go       RetryPolicy type and default implementation
```

## License

MIT

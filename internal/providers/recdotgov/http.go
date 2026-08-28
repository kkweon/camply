package recdotgov

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/kkweon/camply/internal/logger"
)

// recdotgovHeaders are sent to the www.recreation.gov booking/search API; ridbHeaders
// authenticate the ridb.recreation.gov data API. Both ride on top of setBrowserHeaders.
var (
	recdotgovHeaders = map[string]string{"Referer": "https://www.recreation.gov/"}
	ridbHeaders      = map[string]string{"apikey": ridbApiKey}
)

// limiter throttles all Recreation.gov requests to 3/sec (burst 3), mirroring
// Python's class-level @ratelimit.limits(calls=3, period=1). It is process-global
// on purpose: the cap applies across every Provider instance and endpoint.
var limiter = rate.NewLimiter(3, 3)

// Retry policy (the "Moderate" profile). These are vars, not consts, so tests can
// shrink them. We retry only transient failures (network errors, 429, 5xx); 403
// (WAF block) and 404 are terminal because retrying them never helps — the browser
// headers below are what defeat a 403.
var (
	maxRetries     = 5
	retryBaseDelay = 1 * time.Second
	retryMaxDelay  = 8 * time.Second
	retryMaxAge    = 30 * time.Second
)

// chromeUserAgents is a small pool of realistic desktop Chrome UA strings. Python
// rotates UAs via fake-useragent; we pick one at random per request to look less
// like a bot to the WAF.
var chromeUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
}

// setBrowserHeaders mimics Python's STANDARD_HEADERS plus a rotating Chrome UA.
// We intentionally omit Accept-Encoding: setting it manually would disable Go's
// transparent gzip decompression and force us to handle brotli/deflate ourselves.
// Callers add request-specific headers (Referer for recreation.gov, apikey for RIDB).
func setBrowserHeaders(req *http.Request, extra map[string]string) {
	req.Header.Set("User-Agent", chromeUserAgents[rand.Intn(len(chromeUserAgents))])
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,la;q=0.8")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
}

// getJSON performs a rate-limited, retrying GET and decodes the JSON body into out.
// It owns the full request lifecycle (build → headers → throttle → retry → decode →
// close), so every call site is a single line and no response body leaks.
func (p *Provider) getJSON(ctx context.Context, urlStr string, extra map[string]string, out any) error {
	deadline := time.Now().Add(retryMaxAge)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepBackoff(ctx, attempt, deadline); err != nil {
				// Out of retry budget (or ctx cancelled); surface the real failure.
				if lastErr != nil {
					return lastErr
				}
				return err
			}
		}

		if err := limiter.Wait(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return err
		}
		setBrowserHeaders(req, extra)

		resp, err := p.client.Do(req)
		if err != nil {
			// Network/transport error: transient, retry.
			lastErr = err
			logger.Debug("recreation.gov request failed (%s), will retry: %v", urlStr, err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			err := json.NewDecoder(resp.Body).Decode(out)
			_ = resp.Body.Close()
			return err
		}

		// Drain + close so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if isRetryableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("recreation.gov API returned status: %d", resp.StatusCode)
			logger.Debug("recreation.gov returned %d (%s), will retry", resp.StatusCode, urlStr)
			continue
		}

		// Terminal status (403 WAF, 404, other 4xx): retrying won't help.
		return fmt.Errorf("recreation.gov API returned status: %d", resp.StatusCode)
	}

	return lastErr
}

// isRetryableStatus reports whether a non-200 status is worth retrying. Only 429
// (rate limited) and 5xx (server-side) are transient; 403/404/other 4xx are not.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// sleepBackoff waits before retry attempt n using full-jitter exponential backoff,
// capped per-sleep at retryMaxDelay and bounded overall by deadline and ctx.
func sleepBackoff(ctx context.Context, attempt int, deadline time.Time) error {
	// attempt is 1-based for the first retry.
	ceiling := retryBaseDelay << (attempt - 1)
	if ceiling > retryMaxDelay {
		ceiling = retryMaxDelay
	}
	delay := time.Duration(rand.Int63n(int64(ceiling) + 1)) // full jitter: [0, ceiling]

	if time.Now().Add(delay).After(deadline) {
		return fmt.Errorf("retry budget (%s) exhausted", retryMaxAge)
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

package onboarding

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WaitForHealthz polls baseURL+"/v1/healthz" until it returns 2xx or
// the context is cancelled. interval controls how often the probe
// retries between attempts. Callers are responsible for setting an
// appropriate context deadline (e.g. 10s during onboarding).
func WaitForHealthz(ctx context.Context, baseURL string, interval time.Duration) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("baseURL is empty")
	}
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	target := baseURL + "/v1/healthz"

	client := &http.Client{Timeout: interval + (500 * time.Millisecond)}
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return fmt.Errorf("build healthz request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("healthz status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("wait for %s: %w (last: %v)", target, ctx.Err(), lastErr)
			}
			return fmt.Errorf("wait for %s: %w", target, ctx.Err())
		case <-time.After(interval):
		}
	}
}

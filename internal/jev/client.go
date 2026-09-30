// Package jev is a client for System One decision servers that speak the
// TypeSafe Jev /v1/systemone wire protocol: hosted Jev, or local compatible
// servers such as Kev. It answers typed questions with probabilities and
// never generates text.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/secrets"
)

// ErrNotConfigured is returned by Ask when no base URL is set.
var ErrNotConfigured = errors.New("jev: base_url is not configured")

const maxResponseBytes = 1 << 20

var retryDelays = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}

type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type Answer struct {
	Noul       *float64 `json:"noul,omitempty"`
	Choice     string   `json:"choice,omitempty"`
	Score      *float64 `json:"score,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Model   string            `json:"model,omitempty"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// HTTPError is a non-2xx reply. Message is redacted and truncated.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("jev: http %d: %s", e.Status, e.Message) }

type Options struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
}

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func New(opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	key := strings.TrimSpace(opts.APIKey)
	if key != "" {
		secrets.RegisterForced("JEV_API_KEY", key)
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		apiKey:  key,
		model:   strings.TrimSpace(opts.Model),
		http:    httpClient,
		sleep:   sleep,
	}
}

func (c *Client) Configured() bool { return c != nil && c.baseURL != "" }

// IsLoopback reports whether the server runs on this machine, which is the
// condition for sending user text to it.
func (c *Client) IsLoopback() bool {
	if !c.Configured() {
		return false
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Ask sends one state with its questions. Overload replies (429, 503, 529)
// are retried with backoff; other failures return at once.
func (c *Client) Ask(ctx context.Context, state string, questions map[string]Question) (Response, error) {
	if !c.Configured() {
		return Response{}, ErrNotConfigured
	}
	payload := map[string]any{"state": state, "questions": questions}
	if c.model != "" {
		payload["model"] = c.model
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("jev: encode request: %w", err)
	}
	for attempt := 0; ; attempt++ {
		resp, status, err := c.post(ctx, body)
		if err == nil {
			return resp, nil
		}
		retryable := status == http.StatusTooManyRequests || status == 529 || status == http.StatusServiceUnavailable
		if !retryable || attempt >= len(retryDelays) {
			return Response{}, err
		}
		if sleepErr := c.sleep(ctx, retryDelays[attempt]); sleepErr != nil {
			return Response{}, sleepErr
		}
	}
}

func (c *Client) post(ctx context.Context, body []byte) (Response, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Response{}, 0, fmt.Errorf("jev: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Response{}, 0, fmt.Errorf("jev: %s", c.redact(err.Error()))
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return Response{}, res.StatusCode, fmt.Errorf("jev: read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return Response{}, res.StatusCode, &HTTPError{Status: res.StatusCode, Message: secrets.RedactPreview(c.redact(string(raw)), 200)}
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, res.StatusCode, fmt.Errorf("jev: decode response: %w", err)
	}
	return out, res.StatusCode, nil
}

func (c *Client) redact(text string) string {
	if c.apiKey != "" {
		text = strings.ReplaceAll(text, c.apiKey, "[REDACTED]")
	}
	return secrets.RedactText(text)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

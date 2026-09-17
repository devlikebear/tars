// Package jev is a thin client for TypeSafe's System One API (model "Jev"):
// typed choice/score/noul questions over a state string, answered with
// calibrated probabilities instead of text. It has no TARS dependencies beyond
// internal/secrets (for error redaction), so both app packages and pkg/* may
// use it.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/secrets"
)

const (
	DefaultBaseURL = "https://api.typesafe.ai"
	DefaultModel   = "jev-latest"
	defaultTimeout = 15 * time.Second
	maxRetries     = 3
	maxErrorBody   = 200
)

var ErrNoAPIKey = errors.New("jev: api key is not configured")

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria"`
}

func Choice(instructions string, criteria map[string]string) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: criteria}
}

func Noul(instructions, whenTrue, whenFalse string) Question {
	return Question{Type: "noul", Instructions: instructions, Criteria: map[string]string{"true": whenTrue, "false": whenFalse}}
}

func Score(instructions string, levels []string) Question {
	return Question{Type: "score", Instructions: instructions, Criteria: levels}
}

type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Error is a non-2xx reply. Message is redacted and truncated so a key can
// never leak through an error string.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("jev: http %d: %s", e.Status, e.Message) }

type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	sleep      func(time.Duration)
}

func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL:    base,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		model:      model,
		httpClient: &http.Client{Timeout: timeout},
		sleep:      time.Sleep,
	}
}

func (c *Client) Configured() bool { return c != nil && c.apiKey != "" }

type request struct {
	State     string              `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// Ask evaluates every question against state in one round trip. 429 and 529
// are retried with exponential backoff (0.5s, 1s, 2s); every other non-2xx
// returns *Error immediately.
func (c *Client) Ask(ctx context.Context, state string, questions map[string]Question) (Response, error) {
	if !c.Configured() {
		return Response{}, ErrNoAPIKey
	}
	if questions == nil {
		questions = map[string]Question{}
	}
	body, err := json.Marshal(request{State: state, Model: c.model, Questions: questions})
	if err != nil {
		return Response{}, fmt.Errorf("jev: marshal: %w", err)
	}
	backoff := 500 * time.Millisecond
	for attempt := 0; ; attempt++ {
		resp, err := c.do(ctx, body)
		var jerr *Error
		retryable := errors.As(err, &jerr) && (jerr.Status == http.StatusTooManyRequests || jerr.Status == 529)
		if err == nil || !retryable || attempt >= maxRetries || ctx.Err() != nil {
			return resp, err
		}
		c.sleep(backoff)
		backoff *= 2
	}
}

func (c *Client) do(ctx context.Context, body []byte) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("jev: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("jev: request: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Response{}, fmt.Errorf("jev: read body: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := secrets.RedactText(strings.TrimSpace(string(raw)))
		if len(msg) > maxErrorBody {
			msg = msg[:maxErrorBody]
		}
		return Response{}, &Error{Status: res.StatusCode, Message: msg}
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("jev: decode: %w", err)
	}
	return out, nil
}

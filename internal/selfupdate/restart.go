package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrServerNotRunning means nothing answered /v1/healthz.
	ErrServerNotRunning = errors.New("no tars server is running")
	// ErrRestartUnauthorized means the server refused the restart: it needs
	// the admin token.
	ErrRestartUnauthorized = errors.New("the server refused the restart; pass --admin-api-token or set TARS_ADMIN_API_TOKEN")
)

// Server is the running server to restart.
type Server struct {
	// URL is its base URL, e.g. http://127.0.0.1:43180.
	URL string
	// Token is sent as the bearer token; /v1/admin/restart needs the admin
	// token unless the server runs with auth off.
	Token string
	// HTTPClient defaults to a client with a five-second timeout per call.
	HTTPClient *http.Client
	// PollEvery is how often the restarted server is probed (default 500ms).
	PollEvery time.Duration
}

func (s Server) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (s Server) url(p string) string {
	return strings.TrimRight(strings.TrimSpace(s.URL), "/") + p
}

// Version reads the running server's version from /v1/healthz.
func (s Server) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url("/v1/healthz"), nil)
	if err != nil {
		return "", err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("%w at %s: %v", ErrServerNotRunning, s.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var health struct {
		Component string `json:"component"`
		Version   string `json:"version"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&health) != nil || health.Component != "tars" {
		return "", fmt.Errorf("%w at %s: /v1/healthz answered %s", ErrServerNotRunning, s.URL, resp.Status)
	}
	return health.Version, nil
}

// Restart asks the server to re-execute itself and waits, until ctx ends,
// for /v1/healthz to report want.
func (s Server) Restart(ctx context.Context, want string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url("/v1/admin/restart"), nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(s.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("restart the server: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrRestartUnauthorized
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("restart the server: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	every := s.PollEvery
	if every <= 0 {
		every = 500 * time.Millisecond
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			if last != "" {
				return fmt.Errorf("the server came back as %s, not %s; was it started from another tars?", last, want)
			}
			return fmt.Errorf("the server did not come back: %w", ctx.Err())
		case <-ticker.C:
		}
		v, err := s.Version(ctx)
		if err != nil {
			continue
		}
		last = v
		if strings.TrimPrefix(v, "v") == strings.TrimPrefix(want, "v") {
			return nil
		}
	}
}

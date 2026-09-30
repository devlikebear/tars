// Package activity talks to the running TARS server on the tray's behalf:
// which chat turns are running, which wait for a tool approval, which
// unattended runs wait in the ops queue, answering either kind, and opening
// a new session in a folder.
package activity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/devlikebear/tars/desktop/internal/server"
)

// Running is a session with a chat turn in progress.
type Running struct {
	SessionID string    `json:"session_id"`
	Session   string    `json:"session_title"`
	StartedAt time.Time `json:"started_at"`
}

// Approval is a tool call a turn is waiting on someone to allow or deny.
type Approval struct {
	RequestID string    `json:"request_id"`
	SessionID string    `json:"session_id"`
	Session   string    `json:"session_title"`
	ToolName  string    `json:"tool_name"`
	Preview   string    `json:"preview"`
	Reason    string    `json:"reason"`
	AskedAt   time.Time `json:"asked_at"`
	Decisions []string  `json:"decisions"`
}

// QueuedApproval is a tool call of an unattended run (cron, Telegram, a
// subagent) waiting in the server's ops queue (#970, #1033). It is answered
// with Review, never with Answer: the chat permission endpoint does not
// know it.
type QueuedApproval struct {
	ApprovalID  string    `json:"approval_id"`
	SessionID   string    `json:"session_id"`
	Session     string    `json:"session_title"`
	Source      string    `json:"source"`
	RunLabel    string    `json:"run_label"`
	ToolName    string    `json:"tool_name"`
	Preview     string    `json:"preview"`
	Reason      string    `json:"reason"`
	RequestedAt time.Time `json:"requested_at"`
}

// Snapshot is GET /v1/chat/activity.
type Snapshot struct {
	Running []Running  `json:"running"`
	Pending []Approval `json:"pending_approvals"`
	// Queued is empty from a server before #1033.
	Queued []QueuedApproval `json:"queued_approvals"`
}

// Session is one row of GET /v1/sessions.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Hidden    bool      `json:"hidden"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Client calls the server with the configured tokens.
type Client struct {
	cfg  server.Config
	http *http.Client
}

func NewClient(cfg server.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{cfg: cfg, http: httpClient}
}

// APIError is a non-2xx answer.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("tars server: %d %s", e.Status, e.Message)
	}
	return fmt.Sprintf("tars server: %d", e.Status)
}

// IsConflict reports a 409: the question was already answered or closed.
func IsConflict(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict
}

// IsUnauthorized reports a 401 or 403: the server wants a token the shell
// was not given.
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden)
}

// token picks the bearer token: the admin token on admin routes, the user
// token elsewhere. An admin token also passes user routes, so it stands in
// when it is the only one configured.
func (c *Client) token(admin bool) string {
	if admin && c.cfg.AdminToken != "" {
		return c.cfg.AdminToken
	}
	if c.cfg.Token != "" {
		return c.cfg.Token
	}
	return c.cfg.AdminToken
}

func (c *Client) do(ctx context.Context, method, path string, admin bool, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := c.token(admin); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&payload)
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(payload.Error)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Activity reads which turns run and which wait for approval.
func (c *Client) Activity(ctx context.Context) (Snapshot, error) {
	var snap Snapshot
	err := c.do(ctx, http.MethodGet, "/v1/chat/activity", false, nil, &snap)
	return snap, err
}

// Answer decides one approval: allow_once, allow_session, or deny.
func (c *Client) Answer(ctx context.Context, a Approval, decision string) error {
	if a.RequestID == "" || a.SessionID == "" {
		return errors.New("approval without a request or session id")
	}
	switch decision {
	case "allow_once", "allow_session", "deny":
	default:
		return fmt.Errorf("unknown decision %q", decision)
	}
	body := map[string]string{"session_id": a.SessionID, "decision": decision}
	return c.do(ctx, http.MethodPost, "/v1/chat/permissions/"+url.PathEscape(a.RequestID), false, body, nil)
}

// Review approves or rejects an unattended run's queued tool call through
// POST /v1/ops/approvals/{id}/approve|reject. The ops routes are not admin
// routes on the server, so this sends the same token as Activity.
func (c *Client) Review(ctx context.Context, q QueuedApproval, approve bool) error {
	if q.ApprovalID == "" {
		return errors.New("queued approval without an id")
	}
	action := "reject"
	if approve {
		action = "approve"
	}
	return c.do(ctx, http.MethodPost, "/v1/ops/approvals/"+url.PathEscape(q.ApprovalID)+"/"+action, false, nil, nil)
}

// RecentSessions lists up to limit visible, unarchived chat sessions, most
// recently updated first. Listing every session is an admin call.
func (c *Client) RecentSessions(ctx context.Context, limit int) ([]Session, error) {
	var sessions []Session
	if err := c.do(ctx, http.MethodGet, "/v1/admin/sessions?archived=exclude", true, nil, &sessions); err != nil {
		return nil, err
	}
	visible := sessions[:0]
	for _, s := range sessions {
		if s.Hidden || (s.Kind != "" && s.Kind != "main") {
			continue
		}
		visible = append(visible, s)
	}
	sortByUpdated(visible)
	if limit > 0 && len(visible) > limit {
		visible = visible[:limit]
	}
	return visible, nil
}

func sortByUpdated(sessions []Session) {
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && sessions[j].UpdatedAt.After(sessions[j-1].UpdatedAt); j-- {
			sessions[j], sessions[j-1] = sessions[j-1], sessions[j]
		}
	}
}

// NewSessionIn creates a session whose working folder is dir and returns
// its ID. Both steps are admin calls, as they are from the console.
func (c *Client) NewSessionIn(ctx context.Context, title, dir string) (string, error) {
	var created Session
	if err := c.do(ctx, http.MethodPost, "/v1/admin/sessions", true, map[string]string{"title": title}, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("tars server: created session has no id")
	}
	body := map[string]any{"work_dirs": []string{dir}, "current_dir": dir}
	if err := c.do(ctx, http.MethodPut, "/v1/admin/sessions/"+url.PathEscape(created.ID)+"/workdirs", true, body, nil); err != nil {
		return created.ID, err
	}
	return created.ID, nil
}

// Event is one notification from GET /v1/events/stream.
type Event struct {
	Type      string `json:"type"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
	OpenPath  string `json:"open_path"`
}

// Stream reads the event stream until ctx ends or the connection drops,
// calling onEvent for each notification. Keepalives are skipped.
func (c *Client) Stream(ctx context.Context, onEvent func(Event)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL+"/v1/events/stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if token := c.token(false); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// The stream is long-lived: no client timeout, only ctx.
	resp, err := (&http.Client{Transport: c.http.Transport}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &APIError{Status: resp.StatusCode}
	}
	return ReadEvents(resp.Body, onEvent)
}

// ReadEvents parses a server-sent event stream: each "data:" line is one
// JSON event. It returns when r ends.
func ReadEvents(r io.Reader, onEvent func(Event)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}
		var evt Event
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &evt) != nil {
			continue
		}
		if evt.Type == "keepalive" {
			continue
		}
		onEvent(evt)
	}
	return scanner.Err()
}

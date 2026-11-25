//go:build !windows

// The preview stand-in is a #!/bin/sh script, like easy_preview_test.go's.

package tarsserver

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/previewrelay"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func servingStatus() remoteaccess.Status {
	return remoteaccess.Status{
		Installed: true, LoggedIn: true, TailnetURL: "desk.tail1234.ts.net",
		ServeActive: true, OwnedByTARS: true, ServePort: 443,
		Relay: &remoteaccess.RelayStatus{Active: true, OwnedByTARS: true, Port: 8443},
	}
}

type ticketResp struct {
	URL       string              `json:"url"`
	ExpiresAt string              `json:"expires_at"`
	Preview   easyproject.Preview `json:"preview"`
	Code      string              `json:"code"`
}

func ticketAs(t *testing.T, h http.Handler, id, role, device string) (int, ticketResp) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/easy/projects/"+id+"/preview/ticket", nil)
	ctx := serverauth.WithRoleContext(req.Context(), role)
	if device != "" {
		ctx = serverauth.WithDeviceIDContext(ctx, device)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(ctx))
	var out ticketResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// devServer stands in for the project's Vite server: a page, the cookies
// it was sent, and an echoing upgraded socket.
func devServer(t *testing.T) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var lastCookie atomic.Value
	lastCookie.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastCookie.Store(r.Header.Get("Cookie"))
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			_ = buf.Flush()
			_, _ = io.Copy(conn, buf)
			return
		}
		_, _ = io.WriteString(w, "vibe app "+r.Host)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastCookie
}

// TestEasyPhonePreviewEndToEnd: a phone asks for a ticket, the preview
// starts, the ticket opens the relay, and the app and its socket come
// through; revoking the phone ends it.
func TestEasyPhonePreviewEndToEnd(t *testing.T) {
	previewScript(t, "exec sleep 60")
	dev, lastCookie := devServer(t)
	devPort, _ := strconv.Atoi(dev.URL[strings.LastIndex(dev.URL, ":")+1:])

	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	workspace := f.store.WorkspaceDir()
	phone := newEasyPhonePreview(workspace, func(context.Context) (remoteaccess.Status, error) { return servingStatus(), nil })
	d.phonePreview = phone
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	// The stand-in only sleeps; the dev server above is what answers on
	// the preview's port.
	phone.previews.Load().FreePort = func() (int, error) { return devPort, nil }
	relay := httptest.NewServer(phone.relay)
	t.Cleanup(relay.Close)
	phone.listening.Store(true)

	project := easyproject.Project{ID: "0123456789abcdef", Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady}
	putEasyProject(t, workspace, project)
	store := consoleauth.NewStore(workspace)
	device, _, err := store.CreateDevice("iPhone", serverauth.RoleFocus)
	if err != nil {
		t.Fatal(err)
	}

	code, tk := ticketAs(t, h, project.ID, serverauth.RoleFocus, device.ID)
	if code != http.StatusOK {
		t.Fatalf("ticket = %d %+v", code, tk)
	}
	if tk.Preview.URL != "" {
		t.Fatalf("a phone got the loopback address: %+v", tk.Preview)
	}
	if tk.Preview.State != easyproject.PreviewStarting && tk.Preview.State != easyproject.PreviewRunning {
		t.Fatalf("preview = %+v", tk.Preview)
	}
	u, err := url.Parse(tk.URL)
	if err != nil || u.Scheme != "https" || u.Host != "desk.tail1234.ts.net:8443" || u.Path != previewrelay.EnterPath || u.Query().Get("t") == "" {
		t.Fatalf("ticket url = %q", tk.URL)
	}
	expires, err := time.Parse(time.RFC3339, tk.ExpiresAt)
	if err != nil || time.Until(expires) > previewrelay.DefaultTicketTTL || time.Until(expires) < 50*time.Second {
		t.Fatalf("expires_at = %q", tk.ExpiresAt)
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(relay.URL + previewrelay.EnterPath + "?" + u.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == previewrelay.CookieName {
			cookie = c
		}
	}
	if resp.StatusCode != http.StatusOK || cookie == nil {
		t.Fatalf("enter = %d, cookie %v", resp.StatusCode, cookie)
	}

	get := func() (int, string) {
		req, _ := http.NewRequest(http.MethodGet, relay.URL+"/", nil)
		req.Header.Set("Cookie", previewrelay.CookieName+"="+cookie.Value+"; tars_session=console")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(body)
	}
	var body string
	waitFor(t, "the app through the relay", func() bool {
		code, b := get()
		body = b
		return code == http.StatusOK
	})
	if want := "vibe app 127.0.0.1:" + strconv.Itoa(devPort); body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if c := lastCookie.Load().(string); c != "" {
		t.Fatalf("dev server got cookies %q", c)
	}

	// The HMR socket.
	conn, err := net.Dial("tcp", strings.TrimPrefix(relay.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /?token=x HTTP/1.1\r\nHost: desk.tail1234.ts.net:8443\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: "+wsKey()+"\r\nCookie: "+previewrelay.CookieName+"="+cookie.Value+"\r\n\r\n")
	br := bufio.NewReader(conn)
	up, err := http.ReadResponse(br, nil)
	if err != nil || up.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade = %v %v", up, err)
	}
	_, _ = io.WriteString(conn, "hmr")
	echo := make([]byte, 3)
	if _, err := io.ReadFull(br, echo); err != nil || string(echo) != "hmr" {
		t.Fatalf("echo = %q %v", echo, err)
	}

	// The ticket was used once.
	resp, err = client.Get(relay.URL + previewrelay.EnterPath + "?" + u.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second trade = %d", resp.StatusCode)
	}

	// Revoking the phone ends its relay session at the next request.
	if err := store.RevokeDevice(device.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(); code != http.StatusUnauthorized {
		t.Fatalf("after revoke = %d", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d.Close(ctx)
}

func TestEasyPhonePreviewTicketRefusals(t *testing.T) {
	previewScript(t, "exec sleep 60")
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	var mu sync.Mutex
	status, detectErr := servingStatus(), error(nil)
	calls := 0
	phone := newEasyPhonePreview(f.store.WorkspaceDir(), func(context.Context) (remoteaccess.Status, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return status, detectErr
	})
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	phone.now = func() time.Time { return now }
	d.phonePreview = phone
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	ready := easyproject.Project{ID: "0123456789abcdef", Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady}
	putEasyProject(t, f.store.WorkspaceDir(), ready)
	putEasyProject(t, f.store.WorkspaceDir(), easyproject.Project{ID: "fedcba9876543210", Name: "y", Status: easyproject.StatusFailed})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		d.Close(ctx)
	})

	if code, tk := ticketAs(t, h, "0000000000000000", serverauth.RoleFocus, ""); code != http.StatusNotFound || tk.URL != "" {
		t.Fatalf("unknown project = %d %+v", code, tk)
	}
	if code, tk := ticketAs(t, h, "fedcba9876543210", serverauth.RoleFocus, ""); code != http.StatusConflict || tk.Code != "not_ready" {
		t.Fatalf("not ready = %d %+v", code, tk)
	}
	if code, tk := ticketAs(t, h, ready.ID, serverauth.RoleFocus, ""); code != http.StatusServiceUnavailable || tk.Code != relayReasonDown || tk.URL != "" {
		t.Fatalf("relay down = %d %+v", code, tk)
	}
	if calls != 0 {
		t.Fatalf("tailscale read while the relay is down")
	}
	phone.listening.Store(true)

	set := func(s remoteaccess.Status, err error) {
		mu.Lock()
		status, detectErr = s, err
		mu.Unlock()
		now = now.Add(relayStatusTTL) // past any cached answer
	}
	notServed := servingStatus()
	notServed.Relay = &remoteaccess.RelayStatus{Port: 8443}
	for _, tc := range []struct {
		status remoteaccess.Status
		err    error
		code   string
	}{
		{remoteaccess.Status{}, errors.New("boom"), relayReasonTailscale},
		{remoteaccess.Status{Installed: true}, nil, relayReasonTailscale},
		{remoteaccess.Status{Installed: true, LoggedIn: true, TailnetURL: "desk.ts.net"}, nil, relayReasonRemoteOff},
		{notServed, nil, relayReasonNotServed},
	} {
		set(tc.status, tc.err)
		if code, tk := ticketAs(t, h, ready.ID, serverauth.RoleFocus, ""); code != http.StatusServiceUnavailable || tk.Code != tc.code || tk.URL != "" {
			t.Errorf("want %s: %d %+v", tc.code, code, tk)
		}
	}
	// A "no" is kept for 5 seconds, then read again.
	before := calls
	ticketAs(t, h, ready.ID, serverauth.RoleFocus, "")
	if calls != before {
		t.Fatalf("a fresh refusal was not cached")
	}
	mu.Lock()
	status, detectErr = servingStatus(), nil
	mu.Unlock()
	now = now.Add(relayStatusFailureTTL)
	code, tk := ticketAs(t, h, ready.ID, serverauth.RoleUser, "")
	if code != http.StatusOK || tk.URL == "" {
		t.Fatalf("after tailscale came back = %d %+v", code, tk)
	}
	if tk.Preview.State == easyproject.PreviewRunning && tk.Preview.URL == "" {
		t.Fatalf("the desk lost the preview address: %+v", tk.Preview)
	}
	// A yes is kept for 30 seconds.
	before = calls
	now = now.Add(relayStatusTTL - time.Second)
	ticketAs(t, h, ready.ID, serverauth.RoleUser, "")
	if calls != before {
		t.Fatalf("a fresh answer was not cached")
	}
}

func TestEasyPhonePreviewFailedPreview(t *testing.T) {
	t.Setenv(easyproject.PreviewPathEnv, filepath.Join(t.TempDir(), "missing.sh"))
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	phone := newEasyPhonePreview(f.store.WorkspaceDir(), func(context.Context) (remoteaccess.Status, error) { return servingStatus(), nil })
	phone.listening.Store(true)
	d.phonePreview = phone
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	ready := easyproject.Project{ID: "0123456789abcdef", Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady}
	putEasyProject(t, f.store.WorkspaceDir(), ready)
	code, tk := ticketAs(t, h, ready.ID, serverauth.RoleFocus, "")
	if code != http.StatusConflict || tk.Code != "preview_failed" || tk.URL != "" || tk.Preview.State != easyproject.PreviewFailed {
		t.Fatalf("failed preview = %d %+v", code, tk)
	}
}

func TestEasyPhonePreviewListen(t *testing.T) {
	phone := newEasyPhonePreview(t.TempDir(), nil)
	if state, _ := phone.previewOf("x"); state != previewrelay.StateStopped {
		t.Fatalf("no previews = %v", state)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	phone.listen(addr, zerolog.Nop())
	if !phone.listening.Load() {
		t.Fatal("not listening")
	}
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("relay without a session = %d", resp.StatusCode)
	}
	// A second relay on the same port stays down.
	other := newEasyPhonePreview(t.TempDir(), nil)
	other.listen(addr, zerolog.Nop())
	if other.listening.Load() {
		t.Fatal("second relay claims to listen")
	}
	other.close()
	phone.close()
	phone.close()
	if phone.listening.Load() {
		t.Fatal("still listening after close")
	}
}

func TestEasyPhonePreviewDeviceCheck(t *testing.T) {
	workspace := t.TempDir()
	store := consoleauth.NewStore(workspace)
	phone, _, err := store.CreateDevice("iPhone", serverauth.RoleFocus)
	if err != nil {
		t.Fatal(err)
	}
	p := newEasyPhonePreview(workspace, nil)
	tk, err := p.relay.IssueTicket("p", "", phone.ID)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(p.relay)
	defer srv.Close()
	resp, err := http.Get(srv.URL + previewrelay.EnterPath + "?t=" + tk.Value)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("active device = %d", resp.StatusCode)
	}
	tk, _ = p.relay.IssueTicket("p", "", "no-such-device")
	resp, err = http.Get(srv.URL + previewrelay.EnterPath + "?t=" + tk.Value)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown device = %d", resp.StatusCode)
	}
}

// wsKey is a Sec-WebSocket-Key: 16 bytes, base64.
func wsKey() string { return base64.StdEncoding.EncodeToString([]byte("relay test nonce")) }

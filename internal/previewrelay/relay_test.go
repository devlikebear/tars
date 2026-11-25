package previewrelay

import (
	"bufio"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// upstream stands in for a project's Vite development server: it records
// what reached it, sets a few cookies, and echoes bytes on an upgraded
// connection the way the HMR socket would carry frames.
type upstream struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []*http.Request
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.requests = append(u.requests, r.Clone(r.Context()))
		u.mu.Unlock()
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, buf, err := hj.Hijack()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			_ = buf.Flush()
			_, _ = io.Copy(conn, buf)
			return
		}
		w.Header().Add("Set-Cookie", "tars_session=evil; Path=/")
		w.Header().Add("Set-Cookie", "tars_preview=evil; Path=/")
		w.Header().Add("Set-Cookie", "app=1; Path=/")
		_, _ = io.WriteString(w, "app:"+r.URL.Path)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *upstream) last() *http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.requests[len(u.requests)-1]
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type fixture struct {
	relay   *Relay
	srv     *httptest.Server
	up      *upstream
	clock   *clock
	folder  string
	revoked map[string]bool
	state   PreviewState
	looks   int
	mu      sync.Mutex
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		up:      newUpstream(t),
		clock:   &clock{now: time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)},
		folder:  t.TempDir(),
		revoked: map[string]bool{},
		state:   StateRunning,
	}
	f.relay = New(Options{
		Preview: func(id string) (PreviewState, string) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.looks++
			if id != "p1" {
				return StateStopped, ""
			}
			if f.state != StateRunning {
				return f.state, ""
			}
			return StateRunning, f.up.srv.URL + "/"
		},
		DeviceActive: func(id string) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return !f.revoked[id]
		},
		Now: f.clock.Now,
	})
	f.srv = httptest.NewServer(f.relay)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) setState(s PreviewState) {
	f.mu.Lock()
	f.state = s
	f.mu.Unlock()
}

func (f *fixture) revoke(id string) {
	f.mu.Lock()
	f.revoked[id] = true
	f.mu.Unlock()
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// enter issues a ticket and trades it, answering the session cookie.
func (f *fixture) enter(t *testing.T, project, device string) *http.Cookie {
	t.Helper()
	tk, err := f.relay.IssueTicket(project, f.folder, device)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	resp, err := noRedirect().Get(f.srv.URL + EnterPath + "?t=" + url.QueryEscape(tk.Value))
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enter status = %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	t.Fatal("enter set no cookie")
	return nil
}

func (f *fixture) get(t *testing.T, path string, cookie *http.Cookie, header map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+path, nil)
	if cookie != nil {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := noRedirect().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(body)
}

func TestEnterSetsAStrictSessionCookie(t *testing.T) {
	f := newFixture(t)
	tk, _ := f.relay.IssueTicket("p1", f.folder, "dev1")
	if !tk.ExpiresAt.Equal(f.clock.Now().Add(DefaultTicketTTL)) {
		t.Fatalf("ExpiresAt = %v", tk.ExpiresAt)
	}
	resp, body := f.get(t, EnterPath+"?t="+tk.Value, nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `url=/`) {
		t.Fatalf("enter = %d %q", resp.StatusCode, body)
	}
	set := resp.Header.Get("Set-Cookie")
	for _, want := range []string{CookieName + "=", "Path=/", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(set, want) {
			t.Errorf("Set-Cookie %q lacks %q", set, want)
		}
	}
	if strings.Contains(set, "Domain=") {
		t.Errorf("cookie is not host-only: %q", set)
	}
	if resp.Header.Get("Referrer-Policy") != "no-referrer" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("enter headers = %v", resp.Header)
	}
	if f.up.count() != 0 {
		t.Fatal("enter reached the app")
	}
}

func TestTicketIsSingleUse(t *testing.T) {
	f := newFixture(t)
	tk, _ := f.relay.IssueTicket("p1", f.folder, "dev1")
	if resp, _ := f.get(t, EnterPath+"?t="+tk.Value, nil, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("first trade = %d", resp.StatusCode)
	}
	if resp, _ := f.get(t, EnterPath+"?t="+tk.Value, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second trade = %d, want 401", resp.StatusCode)
	}
}

func TestTicketExpires(t *testing.T) {
	f := newFixture(t)
	tk, _ := f.relay.IssueTicket("p1", f.folder, "dev1")
	f.clock.add(DefaultTicketTTL)
	if resp, _ := f.get(t, EnterPath+"?t="+tk.Value, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired trade = %d, want 401", resp.StatusCode)
	}
}

func TestTicketRefusals(t *testing.T) {
	f := newFixture(t)
	if _, err := f.relay.IssueTicket(" ", f.folder, "dev1"); err == nil {
		t.Fatal("ticket without a project was issued")
	}
	for _, q := range []string{"", "?t=", "?t=nope"} {
		if resp, body := f.get(t, EnterPath+q, nil, nil); resp.StatusCode != http.StatusUnauthorized || strings.Contains(body, "app:") {
			t.Errorf("enter%s = %d", q, resp.StatusCode)
		}
	}
	tk, _ := f.relay.IssueTicket("p1", f.folder, "dev1")
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+EnterPath+"?t="+tk.Value, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST enter = %d", resp.StatusCode)
	}
}

func TestTicketOfARevokedDeviceIsRefused(t *testing.T) {
	f := newFixture(t)
	tk, _ := f.relay.IssueTicket("p1", f.folder, "dev1")
	f.revoke("dev1")
	if resp, _ := f.get(t, EnterPath+"?t="+tk.Value, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked trade = %d", resp.StatusCode)
	}
}

func TestRevokingTheDeviceEndsItsSessions(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	other := f.enter(t, "p1", "dev2")
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("before revoke = %d", resp.StatusCode)
	}
	f.revoke("dev1")
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after revoke = %d", resp.StatusCode)
	}
	if resp, _ := f.get(t, "/", other, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("other device = %d", resp.StatusCode)
	}
	f.relay.RevokeDevice("dev2")
	f.relay.RevokeDevice("")
	if resp, _ := f.get(t, "/", other, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after RevokeDevice = %d", resp.StatusCode)
	}
	tk, _ := f.relay.IssueTicket("p1", f.folder, "dev3")
	f.relay.RevokeDevice("dev3")
	if resp, _ := f.get(t, EnterPath+"?t="+tk.Value, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ticket after RevokeDevice = %d", resp.StatusCode)
	}
}

func TestDeskTicketNeedsNoDevice(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "")
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("desk session = %d", resp.StatusCode)
	}
}

func TestNoSessionNeverReachesTheApp(t *testing.T) {
	f := newFixture(t)
	for _, c := range []*http.Cookie{nil, {Name: CookieName, Value: "guess"}} {
		resp, body := f.get(t, "/", c, nil)
		if resp.StatusCode != http.StatusUnauthorized || strings.Contains(body, "app:") {
			t.Fatalf("no session = %d %q", resp.StatusCode, body)
		}
	}
	if f.up.count() != 0 {
		t.Fatal("a request without a session reached the app")
	}
}

func TestSessionEndsAfterAnHourUnused(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	f.clock.add(30 * time.Minute)
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("used session = %d", resp.StatusCode)
	}
	if f.relay.Sessions() != 1 {
		t.Fatalf("Sessions = %d", f.relay.Sessions())
	}
	f.clock.add(DefaultSessionIdle)
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("idle session = %d", resp.StatusCode)
	}
	if f.relay.Sessions() != 0 {
		t.Fatalf("Sessions = %d after idle", f.relay.Sessions())
	}
}

func TestRefusedPaths(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	refused := []string{
		"/cdn-cgi/local/explorer/api/d1/database",
		"/cdn-cgi",
		"/CDN-CGI/local/explorer",
		"/%63dn-cgi/local/explorer",
		"/%2563dn-cgi/local/explorer",
		"//cdn-cgi/x",
		"/a/../cdn-cgi/x",
		"/__debug",
		"/__debug/x",
		"/__tars/anything",
		"/@fs/etc/hosts",
		fsPath(f.folder) + "/../secret",
		fsPath(f.folder) + "%2F..%2Fsecret",
		"/@fs",
	}
	for _, p := range refused {
		resp, body := f.get(t, p, c, nil)
		if resp.StatusCode != http.StatusForbidden || strings.Contains(body, "app:") {
			t.Errorf("%s = %d %q, want 403", p, resp.StatusCode, body)
		}
	}
	if f.up.count() != 0 {
		t.Fatalf("%d refused requests reached the app", f.up.count())
	}
	resp, _ := f.get(t, "/sw.js", c, map[string]string{"Service-Worker": "script"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("service worker script = %d", resp.StatusCode)
	}
	for _, p := range []string{fsPath(f.folder) + "/src/main.tsx", "/src/main.tsx", "/api/x", "/cdn-cgix", "/@vite/client"} {
		if resp, body := f.get(t, p, c, nil); resp.StatusCode != http.StatusOK || !strings.HasPrefix(body, "app:") {
			t.Errorf("%s = %d %q, want forwarded", p, resp.StatusCode, body)
		}
	}
}

func TestForwardedRequestsLoseTARSCookiesAndHeaders(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/x?y=1", nil)
	req.Header.Set("Cookie", "tars_session=console; app=1; "+CookieName+"="+c.Value+"; other=2")
	req.Header.Set("Authorization", "Bearer device-token")
	req.Header.Set("Tailscale-User-Login", "owner@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := f.up.last()
	if cookie := got.Header.Get("Cookie"); cookie != "app=1; other=2" {
		t.Errorf("upstream Cookie = %q", cookie)
	}
	for _, h := range []string{"Authorization", "Tailscale-User-Login", "X-Forwarded-For", "X-Forwarded-Host"} {
		if v := got.Header.Get(h); v != "" {
			t.Errorf("upstream %s = %q", h, v)
		}
	}
	upHost := strings.TrimPrefix(f.up.srv.URL, "http://")
	if got.Host != upHost {
		t.Errorf("upstream Host = %q, want %q", got.Host, upHost)
	}
	if got.URL.RawQuery != "y=1" || got.URL.Path != "/api/x" {
		t.Errorf("upstream URL = %v", got.URL)
	}
	setCookies := resp.Header.Values("Set-Cookie")
	if len(setCookies) != 1 || !strings.HasPrefix(setCookies[0], "app=1") {
		t.Errorf("Set-Cookie through relay = %q", setCookies)
	}
}

func TestOnlyCookieIsTheRelaysOwn(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	f.get(t, "/", c, nil)
	if cookie := f.up.last().Header.Get("Cookie"); cookie != "" {
		t.Fatalf("upstream Cookie = %q, want none", cookie)
	}
}

func TestPreviewStates(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	f.setState(StateStarting)
	resp, body := f.get(t, "/", c, nil)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, `http-equiv="refresh"`) {
		t.Fatalf("starting = %d %q", resp.StatusCode, body)
	}
	f.setState(StateRunning)
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("running = %d", resp.StatusCode)
	}
	f.setState(StateStopped)
	if resp, body := f.get(t, "/", c, nil); resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "asleep") {
		t.Fatalf("stopped = %d %q", resp.StatusCode, body)
	}
	f.setState(StateRunning)
	if resp, _ := f.get(t, "/", c, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after the preview stopped = %d, want the session gone", resp.StatusCode)
	}
}

func TestSessionIsBoundToItsProject(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p2", "dev1") // p2 has no running preview in the fixture
	if resp, body := f.get(t, "/", c, nil); resp.StatusCode != http.StatusServiceUnavailable || strings.Contains(body, "app:") {
		t.Fatalf("other project = %d %q", resp.StatusCode, body)
	}
}

func TestReadingThroughTheRelayCountsAsLooking(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	f.mu.Lock()
	before := f.looks
	f.mu.Unlock()
	f.get(t, "/a", c, nil)
	f.get(t, "/b", c, nil)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.looks-before != 2 {
		t.Fatalf("Preview asked %d times for 2 requests", f.looks-before)
	}
}

func TestUnreachableAppAnswersBadGateway(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	r := New(Options{Preview: func(string) (PreviewState, string) { return StateRunning, deadURL }})
	srv := httptest.NewServer(r)
	defer srv.Close()
	tk, _ := r.IssueTicket("p", "", "")
	cookie, ok := r.consume(tk.Value)
	if !ok {
		t.Fatal("consume failed")
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	bad := New(Options{Preview: func(string) (PreviewState, string) { return StateRunning, "::bad" }})
	tk, _ = bad.IssueTicket("p", "", "")
	cookie, _ = bad.consume(tk.Value)
	rec := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
	bad.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("bad base = %d", rec.Code)
	}
}

// TestRelayEndToEnd runs a page load and the HMR socket through the relay
// listener the way the phone's browser would.
func TestRelayEndToEnd(t *testing.T) {
	f := newFixture(t)
	c := f.enter(t, "p1", "dev1")
	if resp, body := f.get(t, "/", c, nil); resp.StatusCode != http.StatusOK || body != "app:/" {
		t.Fatalf("page = %d %q", resp.StatusCode, body)
	}

	conn, err := net.Dial("tcp", strings.TrimPrefix(f.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := "GET /?token=abc HTTP/1.1\r\nHost: desk.tail1234.ts.net:8443\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + wsKey() + "\r\nOrigin: https://desk.tail1234.ts.net:8443\r\n" +
		"Cookie: " + CookieName + "=" + c.Value + "; tars_session=console\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read upgrade response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d", resp.StatusCode)
	}
	if _, err := io.WriteString(conn, "frame-1"); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, len("frame-1"))
	if _, err := io.ReadFull(br, echo); err != nil || string(echo) != "frame-1" {
		t.Fatalf("echo = %q, %v", echo, err)
	}
	up := f.up.last()
	if up.Header.Get("Cookie") != "" || up.URL.RawQuery != "token=abc" {
		t.Fatalf("upgrade upstream: cookie %q query %q", up.Header.Get("Cookie"), up.URL.RawQuery)
	}
	if up.Host != strings.TrimPrefix(f.up.srv.URL, "http://") {
		t.Fatalf("upgrade Host = %q", up.Host)
	}

	// Without a session the socket is refused before reaching the app.
	n := f.up.count()
	conn2, err := net.Dial("tcp", strings.TrimPrefix(f.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn2.Close() }()
	_ = conn2.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn2, "GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	resp2, err := http.ReadResponse(bufio.NewReader(conn2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusUnauthorized || f.up.count() != n {
		t.Fatalf("socket without session = %d, upstream calls %d→%d", resp2.StatusCode, n, f.up.count())
	}
}

// fsPath is Vite's /@fs/ path of a folder on this system.
func fsPath(folder string) string {
	return "/@fs/" + strings.TrimPrefix(filepath.ToSlash(folder), "/")
}

// wsKey is a Sec-WebSocket-Key: 16 bytes, base64.
func wsKey() string { return base64.StdEncoding.EncodeToString([]byte("relay test nonce")) }

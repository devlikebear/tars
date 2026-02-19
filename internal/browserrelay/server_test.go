package browserrelay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIsLoopbackRemoteAddr(t *testing.T) {
	if !isLoopbackRemoteAddr("127.0.0.1:12345") {
		t.Fatalf("expected loopback for ipv4")
	}
	if !isLoopbackRemoteAddr("[::1]:43182") {
		t.Fatalf("expected loopback for ipv6")
	}
	if isLoopbackRemoteAddr("10.0.0.2:8888") {
		t.Fatalf("expected non-loopback")
	}
}

func TestRelayAttachAndCDPToken(t *testing.T) {
	srv, err := New(Options{
		Addr:            "127.0.0.1:0",
		RelayToken:      "relay-token",
		OriginAllowlist: []string{"chrome-extension://*"},
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Close(context.Background())
	})

	versionURL := "http://" + srv.Addr() + "/json/version"
	resp, err := http.Get(versionURL)
	if err != nil {
		t.Fatalf("get version before attach: %v", err)
	}
	defer resp.Body.Close()
	var before map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&before); err != nil {
		t.Fatalf("decode version before attach: %v", err)
	}
	if _, ok := before["webSocketDebuggerUrl"]; ok {
		t.Fatalf("expected no websocket debugger url before extension attach")
	}

	extConn, _, err := websocket.DefaultDialer.Dial(
		"ws://"+srv.Addr()+"/extension",
		http.Header{"Origin": []string{"chrome-extension://abc123"}},
	)
	if err != nil {
		t.Fatalf("dial extension: %v", err)
	}
	defer extConn.Close()

	resp2, err := http.Get(versionURL)
	if err != nil {
		t.Fatalf("get version after attach: %v", err)
	}
	defer resp2.Body.Close()
	var after map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&after); err != nil {
		t.Fatalf("decode version after attach: %v", err)
	}
	if strings.TrimSpace(asString(after["webSocketDebuggerUrl"])) == "" {
		t.Fatalf("expected websocket debugger url after attach")
	}

	_, badResp, badErr := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/cdp", nil)
	if badErr == nil {
		t.Fatalf("expected cdp token error")
	}
	if badResp == nil || badResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %+v", badResp)
	}

	goodHeader := http.Header{"Tars-Relay-Token": []string{"relay-token"}}
	cdpConn, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/cdp", goodHeader)
	if err != nil {
		t.Fatalf("dial cdp with token: %v", err)
	}
	defer cdpConn.Close()

	_ = cdpConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := cdpConn.WriteMessage(websocket.TextMessage, []byte(`{"id":1,"method":"Browser.getVersion"}`)); err != nil {
		t.Fatalf("write cdp message: %v", err)
	}
	_ = extConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := extConn.ReadMessage()
	if err != nil {
		t.Fatalf("read bridged cdp message from extension side: %v", err)
	}
	if !strings.Contains(string(msg), "Browser.getVersion") {
		t.Fatalf("unexpected bridged message: %s", string(msg))
	}
}

func TestRelayRejectsOrigin(t *testing.T) {
	srv, err := New(Options{
		Addr:            "127.0.0.1:0",
		RelayToken:      "relay-token",
		OriginAllowlist: []string{"chrome-extension://approved"},
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Close(context.Background())
	})

	dialer := websocket.Dialer{}
	_, resp, err := dialer.Dial("ws://"+srv.Addr()+"/extension", http.Header{
		"Origin": []string{"https://evil.example.com"},
	})
	if err == nil {
		t.Fatalf("expected origin reject error")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 origin reject, got %+v", resp)
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func TestWildcardOriginMatch(t *testing.T) {
	cases := []struct {
		origin   string
		pattern  string
		expected bool
	}{
		{origin: "chrome-extension://abcd", pattern: "chrome-extension://*", expected: true},
		{origin: "https://a.example.com", pattern: "https://*.example.com", expected: true},
		{origin: "https://evil.com", pattern: "https://*.example.com", expected: false},
	}
	for _, tc := range cases {
		if got := wildcardMatch(tc.pattern, tc.origin); got != tc.expected {
			t.Fatalf("wildcard match(%q,%q)=%v expected %v", tc.pattern, tc.origin, got, tc.expected)
		}
	}
}

func TestVersionJSONContainsCDPWhenAttached(t *testing.T) {
	srv, err := New(Options{Addr: "127.0.0.1:0", RelayToken: "t", OriginAllowlist: []string{"chrome-extension://*"}})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	u := url.URL{Scheme: "ws", Host: srv.Addr(), Path: "/extension"}
	extConn, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{"Origin": []string{"chrome-extension://ok"}})
	if err != nil {
		t.Fatalf("dial extension: %v", err)
	}
	defer extConn.Close()

	res, err := http.Get("http://" + srv.Addr() + "/json/list")
	if err != nil {
		t.Fatalf("get json list: %v", err)
	}
	defer res.Body.Close()
	var entries []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&entries); err != nil {
		t.Fatalf("decode json list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one target entry, got %d", len(entries))
	}
	if strings.TrimSpace(asString(entries[0]["webSocketDebuggerUrl"])) == "" {
		t.Fatalf("expected websocket debugger url in json list")
	}
}

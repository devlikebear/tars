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

func TestRelayRouteCdp_CreateTarget_ReturnsExtensionTabID(t *testing.T) {
	srv := &Server{}
	srv.setExtensionTarget("tab-42", "https://example.com", "Example")

	resp, forward, err := srv.routeCDPRequest([]byte(`{"id":1,"method":"Target.createTarget","params":{"url":"about:blank"}}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if forward != nil {
		t.Fatalf("expected no forward payload")
	}

	var decoded map[string]any
	if err := json.Unmarshal(resp, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, _ := decoded["result"].(map[string]any)
	targetID := strings.TrimSpace(asString(result["targetId"]))
	if targetID != "tab-42" {
		t.Fatalf("expected targetId tab-42, got %q", targetID)
	}
}

func TestRelayRouteCdp_AttachToTarget_ReturnsSessionID(t *testing.T) {
	srv := &Server{}
	resp, forward, err := srv.routeCDPRequest([]byte(`{"id":1,"method":"Target.attachToTarget","params":{"targetId":"tab-42","flatten":true}}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if forward != nil {
		t.Fatalf("expected no forward payload")
	}

	var decoded map[string]any
	if err := json.Unmarshal(resp, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, _ := decoded["result"].(map[string]any)
	sessionID := strings.TrimSpace(asString(result["sessionId"]))
	if sessionID == "" {
		t.Fatalf("expected non-empty sessionId")
	}
}

func TestRelayRouteCdp_SessionCommand_ForwardedToExtension(t *testing.T) {
	srv := &Server{}
	resp, forward, err := srv.routeCDPRequest([]byte(`{"id":9,"method":"Runtime.enable","params":{},"sessionId":"s-1"}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected no immediate response payload")
	}
	if len(forward) == 0 {
		t.Fatalf("expected forward payload")
	}
	var envelope map[string]any
	if err := json.Unmarshal(forward, &envelope); err != nil {
		t.Fatalf("decode forward payload: %v", err)
	}
	if strings.TrimSpace(asString(envelope["method"])) != "forwardCDPCommand" {
		t.Fatalf("unexpected forward method: %v", envelope["method"])
	}
	params, _ := envelope["params"].(map[string]any)
	if strings.TrimSpace(asString(params["method"])) != "Runtime.enable" {
		t.Fatalf("unexpected command method: %v", params["method"])
	}
	if strings.TrimSpace(asString(params["sessionId"])) != "s-1" {
		t.Fatalf("unexpected sessionId: %v", params["sessionId"])
	}
	commandParams, _ := params["params"].(map[string]any)
	if commandParams == nil {
		t.Fatalf("expected command params object")
	}
	if _, exists := commandParams["sessionId"]; exists {
		t.Fatalf("did not expect sessionId in command params: %+v", commandParams)
	}
}

func TestRelayRouteCdp_TargetSetDiscoverTargets_NoForwardEvenWithSession(t *testing.T) {
	srv := &Server{}
	resp, forward, synthetic, err := srv.routeCDPRequestForConn(nil, []byte(`{"id":10,"method":"Target.setDiscoverTargets","params":{"discover":true},"sessionId":"s-1"}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if len(resp) == 0 {
		t.Fatalf("expected local response payload")
	}
	if forward != nil {
		t.Fatalf("expected no forward payload")
	}
	if len(synthetic) == 0 {
		t.Fatalf("expected synthetic target events")
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if asNumber(decoded["id"]) != 10 {
		t.Fatalf("expected id 10, got %v", decoded["id"])
	}
	if strings.TrimSpace(asString(decoded["sessionId"])) != "s-1" {
		t.Fatalf("expected sessionId s-1, got %v", decoded["sessionId"])
	}
}

func TestRelayRouteCdp_TargetSetAutoAttach_ReplaysSyntheticTargets(t *testing.T) {
	srv := &Server{connectedTarget: relayTargetState{TargetID: "tab-1", URL: "https://example.com", Title: "Example"}}
	resp, forward, synthetic, err := srv.routeCDPRequestForConn(nil, []byte(`{"id":11,"method":"Target.setAutoAttach","params":{"autoAttach":true},"sessionId":"s-2"}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if len(resp) == 0 {
		t.Fatalf("expected local response")
	}
	if forward != nil {
		t.Fatalf("expected no forward payload")
	}
	if len(synthetic) == 0 {
		t.Fatalf("expected synthetic target events")
	}
	var evt map[string]any
	if err := json.Unmarshal(synthetic[0], &evt); err != nil {
		t.Fatalf("decode synthetic event: %v", err)
	}
	if strings.TrimSpace(asString(evt["method"])) != "Target.targetCreated" {
		t.Fatalf("unexpected synthetic method: %+v", evt)
	}
}

func TestRelayRouteCdp_AttachToTarget_EmitsAttachedEvent(t *testing.T) {
	srv := &Server{}
	resp, forward, synthetic, err := srv.routeCDPRequestForConn(nil, []byte(`{"id":12,"method":"Target.attachToTarget","params":{"targetId":"tab-42","flatten":true}}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if len(resp) == 0 {
		t.Fatalf("expected local response")
	}
	if forward != nil {
		t.Fatalf("expected no forward payload")
	}
	if len(synthetic) != 1 {
		t.Fatalf("expected one synthetic attached event, got %d", len(synthetic))
	}
	var evt map[string]any
	if err := json.Unmarshal(synthetic[0], &evt); err != nil {
		t.Fatalf("decode attached event: %v", err)
	}
	if strings.TrimSpace(asString(evt["method"])) != "Target.attachedToTarget" {
		t.Fatalf("unexpected attached event payload: %+v", evt)
	}
}

func TestRelayRouteCdp_AttachToTarget_WithCDPConn_UsesLocalSyntheticAttach(t *testing.T) {
	srv := &Server{}
	cdpConn := &websocket.Conn{}
	resp, forward, synthetic, err := srv.routeCDPRequestForConn(cdpConn, []byte(`{"id":21,"method":"Target.attachToTarget","params":{"targetId":"tab-42","flatten":true}}`))
	if err != nil {
		t.Fatalf("route cdp request: %v", err)
	}
	if len(resp) == 0 {
		t.Fatalf("expected immediate local response payload")
	}
	if forward != nil {
		t.Fatalf("expected no forward payload")
	}
	if len(synthetic) != 1 {
		t.Fatalf("expected synthetic attached event, got %d", len(synthetic))
	}

	var decoded map[string]any
	if err := json.Unmarshal(resp, &decoded); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	result, _ := decoded["result"].(map[string]any)
	sessionID := strings.TrimSpace(asString(result["sessionId"]))
	if sessionID == "" {
		t.Fatalf("expected local synthetic session id")
	}
}

func TestRelayProtocol_ForwardEvent_BroadcastToCDP(t *testing.T) {
	srv := &Server{}
	out, err := srv.routeExtensionMessage([]byte(`{
		"method":"forwardCDPEvent",
		"params":{"method":"Page.frameNavigated","params":{"frame":{"id":"f-1"}},"sessionId":"s-1"}
	}`))
	if err != nil {
		t.Fatalf("route extension message: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if strings.TrimSpace(asString(decoded["method"])) != "Page.frameNavigated" {
		t.Fatalf("unexpected event method: %v", decoded["method"])
	}
	if strings.TrimSpace(asString(decoded["sessionId"])) != "s-1" {
		t.Fatalf("unexpected sessionId: %v", decoded["sessionId"])
	}
}

func TestRelayProtocol_ForwardResponse_BroadcastToCDP(t *testing.T) {
	srv := &Server{}
	out, err := srv.routeExtensionMessage([]byte(`{
		"method":"forwardCDPResponse",
		"params":{"id":7,"result":{"ok":true},"sessionId":"s-9"}
	}`))
	if err != nil {
		t.Fatalf("route extension message: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if asNumber(decoded["id"]) != 7 {
		t.Fatalf("expected id 7, got %v", decoded["id"])
	}
	result, _ := decoded["result"].(map[string]any)
	if !asBool(result["ok"]) {
		t.Fatalf("expected result.ok=true")
	}
	if strings.TrimSpace(asString(decoded["sessionId"])) != "s-9" {
		t.Fatalf("unexpected sessionId: %v", decoded["sessionId"])
	}
}

func TestRelayProtocol_ExtensionReady_SetsTargetID(t *testing.T) {
	srv := &Server{}
	out, err := srv.routeExtensionMessage([]byte(`{
		"method":"extensionReady",
		"params":{"targetId":"tab-101","url":"https://example.com","title":"Example"}
	}`))
	if err != nil {
		t.Fatalf("route extension ready: %v", err)
	}
	if out != nil {
		t.Fatalf("expected no cdp forward payload")
	}
	target := srv.currentTarget()
	if target.TargetID != "tab-101" {
		t.Fatalf("expected targetId tab-101, got %q", target.TargetID)
	}
}

func TestRelayProtocol_TargetInfoChangedUpdatesConnectedTarget(t *testing.T) {
	srv := &Server{}
	srv.setExtensionTarget("tab-1", "about:blank", "Blank")
	_, err := srv.routeExtensionMessage([]byte(`{
		"method":"forwardCDPEvent",
		"params":{
			"method":"Target.targetInfoChanged",
			"params":{"targetInfo":{"targetId":"tab-1","url":"https://example.com","title":"Example"}}
		}
	}`))
	if err != nil {
		t.Fatalf("route targetInfoChanged: %v", err)
	}
	target := srv.currentTarget()
	if target.URL != "https://example.com" {
		t.Fatalf("expected updated url, got %q", target.URL)
	}
	if target.Title != "Example" {
		t.Fatalf("expected updated title, got %q", target.Title)
	}
}

func TestRelayProtocol_Keepalive_PingPong(t *testing.T) {
	srv := &Server{}
	if !srv.extLastPongAt.IsZero() {
		t.Fatalf("expected zero initial pong timestamp")
	}
	out, err := srv.routeExtensionMessage([]byte(`{"method":"pong"}`))
	if err != nil {
		t.Fatalf("route pong: %v", err)
	}
	if out != nil {
		t.Fatalf("expected no forward payload for pong")
	}
	srv.mu.RLock()
	lastPong := srv.extLastPongAt
	srv.mu.RUnlock()
	if lastPong.IsZero() {
		t.Fatalf("expected pong timestamp to be set")
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
	resp, err := http.Get(versionURL + "?token=relay-token")
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
		"ws://"+srv.Addr()+"/extension?token=relay-token",
		http.Header{"Origin": []string{"chrome-extension://abc123"}},
	)
	if err != nil {
		t.Fatalf("dial extension: %v", err)
	}
	defer extConn.Close()

	resp2, err := http.Get(versionURL + "?token=relay-token")
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
	if err := cdpConn.WriteMessage(websocket.TextMessage, []byte(`{"id":1,"method":"Runtime.enable","params":{},"sessionId":"relay-session-1"}`)); err != nil {
		t.Fatalf("write cdp message: %v", err)
	}
	_ = extConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := extConn.ReadMessage()
	if err != nil {
		t.Fatalf("read wrapped command at extension side: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(msg, &payload); err != nil {
		t.Fatalf("decode wrapped command: %v", err)
	}
	if strings.TrimSpace(asString(payload["method"])) != "forwardCDPCommand" {
		t.Fatalf("expected forwardCDPCommand, got %v", payload["method"])
	}
	params, _ := payload["params"].(map[string]any)
	commandParams, _ := params["params"].(map[string]any)
	if _, exists := commandParams["sessionId"]; exists {
		t.Fatalf("did not expect sessionId in command params: %+v", commandParams)
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
	_, resp, err := dialer.Dial("ws://"+srv.Addr()+"/extension?token=relay-token", http.Header{
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

func asNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
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
	q := u.Query()
	q.Set("token", "t")
	u.RawQuery = q.Encode()
	extConn, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{"Origin": []string{"chrome-extension://ok"}})
	if err != nil {
		t.Fatalf("dial extension: %v", err)
	}
	defer extConn.Close()

	res, err := http.Get("http://" + srv.Addr() + "/json/list?token=t")
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

func TestRelayAcceptsCDPTokenFromQuery(t *testing.T) {
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

	extConn, _, err := websocket.DefaultDialer.Dial(
		"ws://"+srv.Addr()+"/extension?token=relay-token",
		http.Header{"Origin": []string{"chrome-extension://abc123"}},
	)
	if err != nil {
		t.Fatalf("dial extension: %v", err)
	}
	defer extConn.Close()

	cdpConn, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/cdp?token=relay-token", nil)
	if err != nil {
		t.Fatalf("dial cdp with query token: %v", err)
	}
	defer cdpConn.Close()

	_ = cdpConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := cdpConn.WriteMessage(websocket.TextMessage, []byte(`{"id":1,"method":"Runtime.enable","params":{},"sessionId":"relay-session-2"}`)); err != nil {
		t.Fatalf("write cdp message: %v", err)
	}
	_ = extConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := extConn.ReadMessage()
	if err != nil {
		t.Fatalf("read wrapped cdp message from extension side: %v", err)
	}
	if !strings.Contains(string(msg), "forwardCDPCommand") {
		t.Fatalf("unexpected bridged message: %s", string(msg))
	}
	var payload map[string]any
	if err := json.Unmarshal(msg, &payload); err != nil {
		t.Fatalf("decode bridged payload: %v", err)
	}
	params, _ := payload["params"].(map[string]any)
	commandParams, _ := params["params"].(map[string]any)
	if _, exists := commandParams["sessionId"]; exists {
		t.Fatalf("did not expect sessionId in command params: %+v", commandParams)
	}
}

func TestRelayProtocol_ForwardEvent_BroadcastToMultipleCDPClients(t *testing.T) {
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
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	extConn, _, err := websocket.DefaultDialer.Dial(
		"ws://"+srv.Addr()+"/extension?token=relay-token",
		http.Header{"Origin": []string{"chrome-extension://abc123"}},
	)
	if err != nil {
		t.Fatalf("dial extension: %v", err)
	}
	defer extConn.Close()

	cdp1, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/cdp?token=relay-token", nil)
	if err != nil {
		t.Fatalf("dial cdp1: %v", err)
	}
	defer cdp1.Close()
	cdp2, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/cdp?token=relay-token", nil)
	if err != nil {
		t.Fatalf("dial cdp2: %v", err)
	}
	defer cdp2.Close()

	eventPayload := `{"method":"forwardCDPEvent","params":{"method":"Page.frameNavigated","params":{"frame":{"id":"f-1"}},"sessionId":"s-1"}}`
	if err := extConn.WriteMessage(websocket.TextMessage, []byte(eventPayload)); err != nil {
		t.Fatalf("write event payload: %v", err)
	}

	readEvent := func(t *testing.T, conn *websocket.Conn) {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read cdp event: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(msg, &payload); err != nil {
			t.Fatalf("decode cdp event: %v", err)
		}
		if strings.TrimSpace(asString(payload["method"])) != "Page.frameNavigated" {
			t.Fatalf("unexpected event payload: %+v", payload)
		}
	}
	readEvent(t, cdp1)
	readEvent(t, cdp2)
}

func TestRelayRejectsExtensionWithoutToken(t *testing.T) {
	srv, err := New(Options{
		Addr:            "127.0.0.1:0",
		RelayToken:      "relay-token",
		OriginAllowlist: []string{"chrome-extension://*"},
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	dialer := websocket.Dialer{}
	_, resp, err := dialer.Dial("ws://"+srv.Addr()+"/extension", http.Header{
		"Origin": []string{"chrome-extension://ok"},
	})
	if err == nil {
		t.Fatalf("expected token reject error")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 token reject, got %+v", resp)
	}
}

func TestRelayJSONVersionRequiresToken(t *testing.T) {
	srv, err := New(Options{
		Addr:            "127.0.0.1:0",
		RelayToken:      "relay-token",
		OriginAllowlist: []string{"chrome-extension://*"},
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	res, err := http.Get("http://" + srv.Addr() + "/json/version")
	if err != nil {
		t.Fatalf("get json version: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for /json/version without token, got %d", res.StatusCode)
	}
}

func TestRelayExtensionStatusRequiresToken(t *testing.T) {
	srv, err := New(Options{
		Addr:            "127.0.0.1:0",
		RelayToken:      "relay-token",
		OriginAllowlist: []string{"chrome-extension://*"},
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	res, err := http.Get("http://" + srv.Addr() + "/extension/status")
	if err != nil {
		t.Fatalf("get extension status: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for /extension/status without token, got %d", res.StatusCode)
	}
}

func TestRelayJSONActivateAndCloseWithToken(t *testing.T) {
	srv, err := New(Options{
		Addr:            "127.0.0.1:0",
		RelayToken:      "relay-token",
		OriginAllowlist: []string{"chrome-extension://*"},
	})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	check := func(path string) {
		t.Helper()
		res, err := http.Get("http://" + srv.Addr() + path + "?token=relay-token")
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, res.StatusCode)
		}
	}
	check("/json/activate/tab-1")
	check("/json/close/tab-1")
}

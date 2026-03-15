package browserrelay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Options configures relay server behavior.
type Options struct {
	Addr            string
	RelayToken      string
	AllowQueryToken bool
	OriginAllowlist []string
}

// Server provides OpenClaw-style extension relay endpoints.
type Server struct {
	opts Options

	listener net.Listener
	httpSrv  *http.Server
	addr     string
	started  atomic.Bool

	mu sync.RWMutex

	extConn       *websocket.Conn
	extAttachedAt time.Time
	extPingCancel context.CancelFunc
	extLastPongAt time.Time
	extLastPingAt time.Time

	cdpClients map[*websocket.Conn]struct{}

	connectedTarget  relayTargetState
	connectedTargets map[string]relayTargetState
	connectedSession string
	sessionSeq       uint64

	pending    map[string]relayPendingRequest
	pendingSeq uint64

	extWriteMu sync.Mutex
	cdpWriteMu sync.Mutex
}

type relayPendingRequest struct {
	RelayID    string
	CDPConn    *websocket.Conn
	OriginalID any
	SessionID  string
	Timer      *time.Timer
}

type relayTargetState struct {
	TargetID string
	URL      string
	Title    string
}

type relayCDPRequest struct {
	ID        any             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type relayForwardCommandEnvelope struct {
	ID     any                      `json:"id,omitempty"`
	Method string                   `json:"method"`
	Params relayForwardCommandParam `json:"params"`
}

type relayForwardCommandParam struct {
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type relayForwardEventEnvelope struct {
	Method string                 `json:"method"`
	Params relayForwardEventParam `json:"params"`
}

type relayForwardEventParam struct {
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type relayExtensionReadyEnvelope struct {
	Method string                   `json:"method"`
	Params relayExtensionReadyParam `json:"params"`
}

type relayExtensionReadyParam struct {
	TargetID string `json:"targetId"`
	URL      string `json:"url,omitempty"`
	Title    string `json:"title,omitempty"`
}

const relayKeepaliveInterval = 5 * time.Second
const relayPendingTimeout = 30 * time.Second
const relayStalePongTimeout = 20 * time.Second

func New(opts Options) (*Server, error) {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1:43182"
	}
	token := strings.TrimSpace(opts.RelayToken)
	if token == "" {
		generated, err := newRelayToken()
		if err != nil {
			return nil, err
		}
		token = generated
	}
	allow := normalizeAllowlist(opts.OriginAllowlist)
	if len(allow) == 0 {
		allow = []string{"chrome-extension://*"}
	}
	return &Server{
		opts:             Options{Addr: addr, RelayToken: token, AllowQueryToken: opts.AllowQueryToken, OriginAllowlist: allow},
		cdpClients:       map[*websocket.Conn]struct{}{},
		connectedTargets: map[string]relayTargetState{},
		pending:          map[string]relayPendingRequest{},
	}, nil
}

func newRelayToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate relay token: %w", err)
	}
	return "relay-" + hex.EncodeToString(buf), nil
}

func (s *Server) Start(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("nil relay server")
	}
	if s.started.Load() {
		return nil
	}
	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.addr = ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/json", s.handleJSON)
	mux.HandleFunc("/json/", s.handleJSON)
	mux.HandleFunc("/json/version", s.handleVersion)
	mux.HandleFunc("/json/list", s.handleList)
	mux.HandleFunc("/json/activate/", s.handleActivate)
	mux.HandleFunc("/json/close/", s.handleCloseTarget)
	mux.HandleFunc("/extension/status", s.handleExtensionStatus)
	mux.HandleFunc("/extension", s.handleExtension)
	mux.HandleFunc("/cdp", s.handleCDP)

	s.httpSrv = &http.Server{Handler: mux}
	s.started.Store(true)
	go func() {
		_ = s.httpSrv.Serve(ln)
	}()
	go func() {
		<-ctx.Done()
		_ = s.Close(context.Background())
	}()
	return nil
}

func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(s.addr) == "" {
		return strings.TrimSpace(s.opts.Addr)
	}
	return s.addr
}

func (s *Server) RelayToken() string {
	if s == nil {
		return ""
	}
	return s.opts.RelayToken
}

func (s *Server) ExtensionConnected() bool {
	if s == nil {
		return false
	}
	return s.hasExtension()
}

func (s *Server) AttachedTabs() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.extConn == nil {
		return 0
	}
	seen := map[string]struct{}{}
	targetID := strings.TrimSpace(s.connectedTarget.TargetID)
	if targetID != "" {
		seen[targetID] = struct{}{}
	}
	for _, target := range s.connectedTargets {
		id := strings.TrimSpace(target.TargetID)
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	if len(seen) == 0 {
		return 1
	}
	return len(seen)
}

func (s *Server) AuthRequired() bool {
	if s == nil {
		return false
	}
	return true
}

func (s *Server) JSONAuthRequired() bool {
	if s == nil {
		return false
	}
	return true
}

func (s *Server) CDPWebSocketURL() string {
	if s == nil {
		return ""
	}
	addr := strings.TrimSpace(s.Addr())
	if addr == "" {
		return ""
	}
	token := strings.TrimSpace(s.RelayToken())
	if token == "" || !s.opts.AllowQueryToken {
		return "ws://" + addr + "/cdp"
	}
	values := neturl.Values{}
	values.Set("token", token)
	return "ws://" + addr + "/cdp?" + values.Encode()
}

func (s *Server) Close(ctx context.Context) error {
	if s == nil || !s.started.Load() {
		return nil
	}
	s.started.Store(false)

	s.mu.Lock()
	ext := s.extConn
	cdpClients := make([]*websocket.Conn, 0, len(s.cdpClients))
	for conn := range s.cdpClients {
		cdpClients = append(cdpClients, conn)
	}
	pingCancel := s.extPingCancel
	s.extConn = nil
	s.extPingCancel = nil
	s.connectedTarget = relayTargetState{}
	s.connectedTargets = map[string]relayTargetState{}
	s.connectedSession = ""
	s.cdpClients = map[*websocket.Conn]struct{}{}
	pending := s.pending
	s.pending = map[string]relayPendingRequest{}
	s.mu.Unlock()

	for relayID, pendingReq := range pending {
		if pendingReq.Timer != nil {
			pendingReq.Timer.Stop()
		}
		s.sendPendingError(pendingReq, relayID, "relay closed")
	}

	if pingCancel != nil {
		pingCancel()
	}
	if ext != nil {
		_ = ext.Close()
	}
	for _, conn := range cdpClients {
		_ = conn.Close()
	}

	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("tars browser relay\n"))
}

func (s *Server) handleJSON(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	s.handleList(w, r)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	payload := map[string]any{
		"Browser": "Tars Relay",
	}
	s.addQueryTokenStatus(payload)
	if s.hasExtension() {
		payload["webSocketDebuggerUrl"] = s.CDPWebSocketURL()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	list := []map[string]any{}
	targets := s.listTargetsSnapshot()
	for _, target := range targets {
		targetID := strings.TrimSpace(target.TargetID)
		if targetID == "" {
			targetID = "tars-relay"
		}
		targetTitle := strings.TrimSpace(target.Title)
		if targetTitle == "" {
			targetTitle = "Tars Relay"
		}
		targetURL := strings.TrimSpace(target.URL)
		if targetURL == "" {
			targetURL = "about:blank"
		}
		list = append(list, map[string]any{
			"id":                   targetID,
			"title":                targetTitle,
			"type":                 "page",
			"url":                  targetURL,
			"webSocketDebuggerUrl": s.CDPWebSocketURL(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleExtension(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if !originAllowed(origin, s.opts.OriginAllowlist) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.mu.Lock()
	old := s.extConn
	oldPingCancel := s.extPingCancel
	s.extConn = conn
	s.extAttachedAt = time.Now().UTC()
	s.extLastPongAt = time.Now().UTC()
	s.extLastPingAt = time.Time{}
	s.connectedSession = ""
	s.connectedTarget = relayTargetState{}
	s.connectedTargets = map[string]relayTargetState{}
	extPingCtx, extPingCancel := context.WithCancel(context.Background())
	s.extPingCancel = extPingCancel
	s.mu.Unlock()
	if oldPingCancel != nil {
		oldPingCancel()
	}
	if old != nil {
		_ = old.Close()
	}

	go s.extensionKeepalive(extPingCtx, conn)
	go s.forwardExtensionToCDP(conn)
}

func (s *Server) handleCDP(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}

	s.mu.RLock()
	ext := s.extConn
	s.mu.RUnlock()
	if ext == nil {
		http.Error(w, "extension not attached", http.StatusServiceUnavailable)
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.mu.Lock()
	if s.cdpClients == nil {
		s.cdpClients = map[*websocket.Conn]struct{}{}
	}
	s.cdpClients[conn] = struct{}{}
	s.mu.Unlock()

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		resp, forward, syntheticEvents, routeErr := s.routeCDPRequestForConn(conn, payload)
		if routeErr != nil {
			if resp != nil {
				s.writeToCDP(conn, websocket.TextMessage, resp)
			}
			continue
		}
		if resp != nil {
			s.writeToCDP(conn, websocket.TextMessage, resp)
		}
		for _, eventPayload := range syntheticEvents {
			s.writeToCDP(conn, websocket.TextMessage, eventPayload)
		}
		if forward != nil {
			if err := s.writeToExtension(forward); err != nil {
				if pendingRelayID := s.extractForwardRelayID(forward); pendingRelayID != "" {
					s.failPending(pendingRelayID, err.Error())
				}
				break
			}
		}
	}

	s.mu.Lock()
	delete(s.cdpClients, conn)
	s.mu.Unlock()
	s.clearPendingForConnection(conn, "cdp client disconnected")
	_ = conn.Close()
}

func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	targetID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/json/activate/"))
	if targetID == "" {
		http.Error(w, "target id is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if strings.TrimSpace(s.connectedTarget.TargetID) != targetID {
		for _, target := range s.connectedTargets {
			if strings.TrimSpace(target.TargetID) == targetID {
				s.connectedTarget = target
				break
			}
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      targetID,
		"success": true,
	})
}

func (s *Server) handleCloseTarget(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	targetID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/json/close/"))
	if targetID == "" {
		http.Error(w, "target id is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if strings.TrimSpace(s.connectedTarget.TargetID) == targetID {
		s.connectedTarget = relayTargetState{}
	}
	for sessionID, target := range s.connectedTargets {
		if strings.TrimSpace(target.TargetID) == targetID {
			delete(s.connectedTargets, sessionID)
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      targetID,
		"success": true,
	})
}

func (s *Server) handleExtensionStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRelayRequest(w, r) {
		return
	}
	payload := map[string]any{
		"connected":     s.hasExtension(),
		"attached_tabs": s.AttachedTabs(),
	}
	s.addQueryTokenStatus(payload)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) addQueryTokenStatus(payload map[string]any) {
	if payload == nil || s == nil || !s.opts.AllowQueryToken {
		return
	}
	payload["query_token_enabled"] = true
	payload["query_token_warning"] = "query token auth is enabled and may leak through browser history or logs"
}

func (s *Server) forwardExtensionToCDP(conn *websocket.Conn) {
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		method := relayEnvelopeMethod(payload)
		if method == "forwardCDPResponse" {
			s.handleForwardCDPResponse(payload)
			continue
		}
		forwarded, parseErr := s.routeExtensionMessage(payload)
		if parseErr != nil || forwarded == nil {
			continue
		}
		s.broadcastToCDP(forwarded)
	}

	s.mu.Lock()
	pingCancel := s.extPingCancel
	s.extPingCancel = nil
	if s.extConn == conn {
		s.extConn = nil
		s.connectedSession = ""
		s.connectedTarget = relayTargetState{}
		s.connectedTargets = map[string]relayTargetState{}
	}
	pending := s.pending
	s.pending = map[string]relayPendingRequest{}
	s.mu.Unlock()
	if pingCancel != nil {
		pingCancel()
	}
	for relayID, req := range pending {
		if req.Timer != nil {
			req.Timer.Stop()
		}
		s.sendPendingError(req, relayID, "extension disconnected")
	}
	s.closeAllCDPClients("extension disconnected")
	_ = conn.Close()
}

func (s *Server) hasExtension() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.extConn != nil
}

func (s *Server) currentTarget() relayTargetState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectedTarget
}

func (s *Server) listTargetsSnapshot() []relayTargetState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.extConn == nil {
		return nil
	}
	out := make([]relayTargetState, 0, len(s.connectedTargets)+1)
	seen := map[string]struct{}{}
	appendTarget := func(target relayTargetState) {
		targetID := strings.TrimSpace(target.TargetID)
		key := targetID
		if key == "" {
			key = strings.TrimSpace(target.URL) + "|" + strings.TrimSpace(target.Title)
		}
		if key == "" {
			key = "about:blank"
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	appendTarget(s.connectedTarget)
	for _, target := range s.connectedTargets {
		appendTarget(target)
	}
	if len(out) == 0 {
		out = append(out, relayTargetState{TargetID: "tars-relay", URL: "about:blank", Title: "Tars Relay"})
	}
	return out
}

func (s *Server) routeCDPRequest(payload []byte) (response []byte, forward []byte, err error) {
	resp, fwd, _, routeErr := s.routeCDPRequestForConn(nil, payload)
	return resp, fwd, routeErr
}

func (s *Server) routeCDPRequestForConn(cdpConn *websocket.Conn, payload []byte) (response []byte, forward []byte, syntheticEvents [][]byte, err error) {
	var req relayCDPRequest
	if decodeErr := json.Unmarshal(payload, &req); decodeErr != nil {
		return nil, nil, nil, decodeErr
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		return nil, nil, nil, fmt.Errorf("cdp method is required")
	}
	sessionID := strings.TrimSpace(req.SessionID)

	switch method {
	case "Browser.getVersion":
		return marshalCDPResponse(req.ID, map[string]any{
			"protocolVersion": "1.3",
			"product":         "TarsRelay/1.0",
			"revision":        "0",
			"userAgent":       "Tars Relay",
			"jsVersion":       "0",
		}, nil, sessionID), nil, nil, nil
	case "Browser.setDownloadBehavior", "Target.activateTarget", "Target.closeTarget":
		return marshalCDPResponse(req.ID, map[string]any{}, nil, sessionID), nil, nil, nil
	case "Target.setAutoAttach", "Target.setDiscoverTargets":
		return marshalCDPResponse(req.ID, map[string]any{}, nil, sessionID), nil, s.syntheticTargetEventsForSession(sessionID), nil
	case "Target.getTargets":
		return marshalCDPResponse(req.ID, map[string]any{
			"targetInfos": s.currentTargetInfos(),
		}, nil, sessionID), nil, nil, nil
	case "Target.getTargetInfo":
		targetID := s.extractTargetID(req.Params)
		targetInfo := s.targetInfoByID(targetID)
		return marshalCDPResponse(req.ID, map[string]any{
			"targetInfo": targetInfo,
		}, nil, sessionID), nil, nil, nil
	}

	if sessionID == "" {
		switch method {
		case "Target.createTarget":
			targetID := s.ensureTargetID()
			return marshalCDPResponse(req.ID, map[string]any{"targetId": targetID}, nil, ""), nil, nil, nil
		case "Target.attachToTarget":
			session := s.ensureSessionID()
			target := s.currentTarget()
			targetID := strings.TrimSpace(target.TargetID)
			if targetID == "" {
				targetID = s.ensureTargetID()
			}
			targetURL := strings.TrimSpace(target.URL)
			if targetURL == "" {
				targetURL = "about:blank"
			}
			targetTitle := strings.TrimSpace(target.Title)
			if targetTitle == "" {
				targetTitle = "Tars Relay"
			}
			s.setTargetForSession(session, target)
			attachedEvent := marshalCDPEvent("Target.attachedToTarget", map[string]any{
				"sessionId": session,
				"targetInfo": map[string]any{
					"targetId": targetID,
					"type":     "page",
					"title":    targetTitle,
					"url":      targetURL,
					"attached": true,
				},
				"waitingForDebugger": false,
			}, "")
			return marshalCDPResponse(req.ID, map[string]any{"sessionId": session}, nil, ""), nil, [][]byte{attachedEvent}, nil
		default:
			return marshalCDPResponse(req.ID, map[string]any{}, nil, ""), nil, nil, nil
		}
	}

	forwardID := req.ID
	if cdpConn != nil {
		forwardID = s.registerPending(cdpConn, req.ID, sessionID)
	}
	envelope := relayForwardCommandEnvelope{
		ID:     forwardID,
		Method: "forwardCDPCommand",
		Params: relayForwardCommandParam{
			Method:    method,
			Params:    req.Params,
			SessionID: sessionID,
		},
	}
	forwardPayload, marshalErr := json.Marshal(envelope)
	if marshalErr != nil {
		if cdpConn != nil {
			s.failPending(asStringAny(forwardID), marshalErr.Error())
		}
		return nil, nil, nil, marshalErr
	}
	return nil, forwardPayload, nil, nil
}

func (s *Server) currentTargetInfo() map[string]any {
	target := s.currentTarget()
	targetID := strings.TrimSpace(target.TargetID)
	if targetID == "" {
		targetID = s.ensureTargetID()
	}
	targetURL := strings.TrimSpace(target.URL)
	if targetURL == "" {
		targetURL = "about:blank"
	}
	targetTitle := strings.TrimSpace(target.Title)
	if targetTitle == "" {
		targetTitle = "Tars Relay"
	}
	return map[string]any{
		"targetId": targetID,
		"type":     "page",
		"title":    targetTitle,
		"url":      targetURL,
		"attached": true,
	}
}

func (s *Server) currentTargetInfos() []map[string]any {
	targets := s.listTargetsSnapshot()
	if len(targets) == 0 {
		return []map[string]any{s.currentTargetInfo()}
	}
	out := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		targetID := strings.TrimSpace(target.TargetID)
		if targetID == "" {
			targetID = "tars-relay"
		}
		targetURL := strings.TrimSpace(target.URL)
		if targetURL == "" {
			targetURL = "about:blank"
		}
		targetTitle := strings.TrimSpace(target.Title)
		if targetTitle == "" {
			targetTitle = "Tars Relay"
		}
		out = append(out, map[string]any{
			"targetId": targetID,
			"type":     "page",
			"title":    targetTitle,
			"url":      targetURL,
			"attached": true,
		})
	}
	return out
}

func (s *Server) targetInfoByID(targetID string) map[string]any {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return s.currentTargetInfo()
	}
	targets := s.listTargetsSnapshot()
	for _, target := range targets {
		if strings.TrimSpace(target.TargetID) != targetID {
			continue
		}
		return map[string]any{
			"targetId": targetID,
			"type":     "page",
			"title":    strings.TrimSpace(target.Title),
			"url":      strings.TrimSpace(target.URL),
			"attached": true,
		}
	}
	info := s.currentTargetInfo()
	info["targetId"] = targetID
	return info
}

func (s *Server) extractTargetID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var params struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return ""
	}
	return strings.TrimSpace(params.TargetID)
}

func (s *Server) routeExtensionMessage(payload []byte) ([]byte, error) {
	var envelope struct {
		ID        any             `json:"id,omitempty"`
		Method    string          `json:"method,omitempty"`
		Params    json.RawMessage `json:"params,omitempty"`
		Result    json.RawMessage `json:"result,omitempty"`
		Error     json.RawMessage `json:"error,omitempty"`
		SessionID string          `json:"sessionId,omitempty"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	method := strings.TrimSpace(envelope.Method)
	switch method {
	case "extensionReady":
		var ready relayExtensionReadyEnvelope
		if err := json.Unmarshal(payload, &ready); err != nil {
			return nil, err
		}
		s.setExtensionTarget(ready.Params.TargetID, ready.Params.URL, ready.Params.Title)
		return nil, nil
	case "forwardCDPEvent":
		var evt relayForwardEventEnvelope
		if err := json.Unmarshal(payload, &evt); err != nil {
			return nil, err
		}
		eventMethod := strings.TrimSpace(evt.Params.Method)
		eventParams := parseJSONRawOrEmptyObject(evt.Params.Params)
		updateTargetFromCDPEvent(s, eventMethod, eventParams, strings.TrimSpace(evt.Params.SessionID))
		out := map[string]any{
			"method": eventMethod,
			"params": eventParams,
		}
		if sid := strings.TrimSpace(evt.Params.SessionID); sid != "" {
			out["sessionId"] = sid
		}
		return json.Marshal(out)
	case "forwardCDPResponse":
		var resp struct {
			Method string `json:"method"`
			Params struct {
				ID        any             `json:"id,omitempty"`
				Result    json.RawMessage `json:"result,omitempty"`
				Error     json.RawMessage `json:"error,omitempty"`
				SessionID string          `json:"sessionId,omitempty"`
			} `json:"params"`
		}
		if err := json.Unmarshal(payload, &resp); err != nil {
			return nil, err
		}
		out := map[string]any{
			"id": resp.Params.ID,
		}
		if len(resp.Params.Result) > 0 {
			out["result"] = parseJSONRawOrEmptyObject(resp.Params.Result)
		} else {
			out["result"] = map[string]any{}
		}
		if len(resp.Params.Error) > 0 {
			out["error"] = parseJSONRawOrEmptyObject(resp.Params.Error)
		}
		if sid := strings.TrimSpace(resp.Params.SessionID); sid != "" {
			out["sessionId"] = sid
		}
		return json.Marshal(out)
	case "pong":
		s.mu.Lock()
		s.extLastPongAt = time.Now().UTC()
		s.mu.Unlock()
		return nil, nil
	case "":
		if envelope.ID != nil {
			out := map[string]any{
				"id": envelope.ID,
			}
			if len(envelope.Result) > 0 {
				out["result"] = parseJSONRawOrEmptyObject(envelope.Result)
			} else {
				out["result"] = map[string]any{}
			}
			if len(envelope.Error) > 0 {
				out["error"] = parseJSONRawOrEmptyObject(envelope.Error)
			}
			if sid := strings.TrimSpace(envelope.SessionID); sid != "" {
				out["sessionId"] = sid
			}
			return json.Marshal(out)
		}
		return nil, nil
	default:
		// Backward-compatible fallback: forward unknown raw CDP events.
		if envelope.ID == nil {
			return payload, nil
		}
		return nil, nil
	}
}

func relayEnvelopeMethod(payload []byte) string {
	var envelope struct {
		Method string `json:"method,omitempty"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Method)
}

func (s *Server) writeToExtension(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	s.mu.RLock()
	ext := s.extConn
	s.mu.RUnlock()
	if ext == nil {
		return fmt.Errorf("extension not attached")
	}
	s.extWriteMu.Lock()
	err := ext.WriteMessage(websocket.TextMessage, payload)
	s.extWriteMu.Unlock()
	return err
}

func (s *Server) broadcastToCDP(payload []byte) {
	if len(payload) == 0 {
		return
	}
	s.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(s.cdpClients))
	for conn := range s.cdpClients {
		clients = append(clients, conn)
	}
	s.mu.RUnlock()
	for _, conn := range clients {
		if conn == nil {
			continue
		}
		s.writeToCDP(conn, websocket.TextMessage, payload)
	}
}

func (s *Server) closeAllCDPClients(reason string) {
	s.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.cdpClients))
	for conn := range s.cdpClients {
		clients = append(clients, conn)
	}
	s.cdpClients = map[*websocket.Conn]struct{}{}
	s.mu.Unlock()
	for _, conn := range clients {
		if conn == nil {
			continue
		}
		closePayload := websocket.FormatCloseMessage(websocket.CloseGoingAway, strings.TrimSpace(reason))
		_ = conn.WriteControl(websocket.CloseMessage, closePayload, time.Now().Add(500*time.Millisecond))
		_ = conn.Close()
	}
}

func (s *Server) extractForwardRelayID(payload []byte) string {
	var envelope relayForwardCommandEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(asStringAny(envelope.ID))
}

func (s *Server) registerPending(cdpConn *websocket.Conn, originalID any, sessionID string) string {
	if cdpConn == nil {
		return strings.TrimSpace(asStringAny(originalID))
	}
	relaySeq := atomic.AddUint64(&s.pendingSeq, 1)
	relayID := fmt.Sprintf("relay-%d", relaySeq)
	timer := time.AfterFunc(relayPendingTimeout, func() {
		s.failPending(relayID, "relay command timeout")
	})
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]relayPendingRequest{}
	}
	s.pending[relayID] = relayPendingRequest{
		RelayID:    relayID,
		CDPConn:    cdpConn,
		OriginalID: originalID,
		SessionID:  strings.TrimSpace(sessionID),
		Timer:      timer,
	}
	s.mu.Unlock()
	return relayID
}

func (s *Server) failPending(relayID, message string) {
	relayID = strings.TrimSpace(relayID)
	if relayID == "" {
		return
	}
	s.mu.Lock()
	pending, exists := s.pending[relayID]
	if exists {
		delete(s.pending, relayID)
	}
	s.mu.Unlock()
	if !exists {
		return
	}
	if pending.Timer != nil {
		pending.Timer.Stop()
	}
	s.sendPendingError(pending, relayID, message)
}

func (s *Server) sendPendingError(pending relayPendingRequest, relayID, message string) {
	if pending.CDPConn == nil {
		return
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = "relay request failed"
	}
	errorPayload := map[string]any{
		"message": trimmed,
	}
	resp := marshalCDPResponse(pending.OriginalID, nil, errorPayload, strings.TrimSpace(pending.SessionID))
	s.writeToCDP(pending.CDPConn, websocket.TextMessage, resp)
}

func (s *Server) clearPendingForConnection(conn *websocket.Conn, reason string) {
	if conn == nil {
		return
	}
	s.mu.Lock()
	keys := make([]string, 0, len(s.pending))
	for relayID, pending := range s.pending {
		if pending.CDPConn == conn {
			keys = append(keys, relayID)
		}
	}
	s.mu.Unlock()
	for _, relayID := range keys {
		s.failPending(relayID, reason)
	}
}

func (s *Server) handleForwardCDPResponse(payload []byte) {
	var resp struct {
		Method string `json:"method"`
		Params struct {
			ID        any             `json:"id,omitempty"`
			Result    json.RawMessage `json:"result,omitempty"`
			Error     json.RawMessage `json:"error,omitempty"`
			SessionID string          `json:"sessionId,omitempty"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}
	relayID := strings.TrimSpace(asStringAny(resp.Params.ID))
	if relayID == "" {
		return
	}
	s.mu.Lock()
	pending, exists := s.pending[relayID]
	if exists {
		delete(s.pending, relayID)
	}
	s.mu.Unlock()
	if !exists {
		return
	}
	if pending.Timer != nil {
		pending.Timer.Stop()
	}
	out := map[string]any{
		"id": pending.OriginalID,
	}
	if len(resp.Params.Result) > 0 {
		out["result"] = parseJSONRawOrEmptyObject(resp.Params.Result)
	} else {
		out["result"] = map[string]any{}
	}
	if len(resp.Params.Error) > 0 {
		out["error"] = parseJSONRawOrEmptyObject(resp.Params.Error)
	}
	sessionID := strings.TrimSpace(resp.Params.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(pending.SessionID)
	}
	if sessionID != "" {
		out["sessionId"] = sessionID
	}
	body, _ := json.Marshal(out)
	s.writeToCDP(pending.CDPConn, websocket.TextMessage, body)
}

func (s *Server) syntheticTargetEventsForSession(sessionID string) [][]byte {
	targetInfos := s.currentTargetInfos()
	events := make([][]byte, 0, len(targetInfos))
	for _, info := range targetInfos {
		events = append(events, marshalCDPEvent("Target.targetCreated", map[string]any{
			"targetInfo": info,
		}, strings.TrimSpace(sessionID)))
	}
	return events
}

func marshalCDPEvent(method string, params any, sessionID string) []byte {
	out := map[string]any{
		"method": strings.TrimSpace(method),
		"params": params,
	}
	if strings.TrimSpace(sessionID) != "" {
		out["sessionId"] = strings.TrimSpace(sessionID)
	}
	payload, _ := json.Marshal(out)
	return payload
}

func (s *Server) setTargetForSession(sessionID string, target relayTargetState) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	if s.connectedTargets == nil {
		s.connectedTargets = map[string]relayTargetState{}
	}
	s.connectedTargets[sessionID] = relayTargetState{
		TargetID: strings.TrimSpace(target.TargetID),
		URL:      strings.TrimSpace(target.URL),
		Title:    strings.TrimSpace(target.Title),
	}
	s.mu.Unlock()
}

func updateTargetFromCDPEvent(s *Server, method string, params any, sessionID string) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	method = strings.TrimSpace(method)
	if method == "" {
		return
	}
	root, ok := params.(map[string]any)
	if !ok {
		return
	}
	if method == "Target.detachedFromTarget" {
		detachedSession := strings.TrimSpace(asStringAny(root["sessionId"]))
		if detachedSession == "" {
			detachedSession = sessionID
		}
		if detachedSession == "" {
			return
		}
		s.mu.Lock()
		delete(s.connectedTargets, detachedSession)
		s.mu.Unlock()
		return
	}
	if method == "Target.attachedToTarget" {
		attachedSession := strings.TrimSpace(asStringAny(root["sessionId"]))
		if attachedSession == "" {
			return
		}
		info, _ := root["targetInfo"].(map[string]any)
		targetID := strings.TrimSpace(asStringAny(info["targetId"]))
		targetURL := strings.TrimSpace(asStringAny(info["url"]))
		targetTitle := strings.TrimSpace(asStringAny(info["title"]))
		s.mu.Lock()
		s.connectedTargets[attachedSession] = relayTargetState{
			TargetID: targetID,
			URL:      targetURL,
			Title:    targetTitle,
		}
		if s.connectedTarget.TargetID == "" && targetID != "" {
			s.connectedTarget = relayTargetState{TargetID: targetID, URL: targetURL, Title: targetTitle}
		}
		s.mu.Unlock()
		return
	}
	if method != "Target.targetInfoChanged" {
		return
	}
	info, ok := root["targetInfo"].(map[string]any)
	if !ok {
		return
	}
	targetID := strings.TrimSpace(asStringAny(info["targetId"]))
	targetURL := strings.TrimSpace(asStringAny(info["url"]))
	targetTitle := strings.TrimSpace(asStringAny(info["title"]))
	if targetID == "" && targetURL == "" && targetTitle == "" {
		return
	}
	s.mu.Lock()
	current := s.connectedTarget
	if targetID == "" {
		targetID = current.TargetID
	}
	if targetURL == "" {
		targetURL = current.URL
	}
	if targetTitle == "" {
		targetTitle = current.Title
	}
	s.connectedTarget = relayTargetState{
		TargetID: targetID,
		URL:      targetURL,
		Title:    targetTitle,
	}
	if sessionID != "" {
		s.connectedTargets[sessionID] = relayTargetState{
			TargetID: targetID,
			URL:      targetURL,
			Title:    targetTitle,
		}
	}
	s.mu.Unlock()
}

func asStringAny(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case float64, float32, int, int64, int32, uint64, uint32, uint:
		return fmt.Sprint(value)
	default:
		return ""
	}
}

func parseJSONRawOrEmptyObject(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func marshalCDPResponse(id any, result any, errorPayload any, sessionID string) []byte {
	out := map[string]any{
		"id": id,
	}
	if result == nil {
		out["result"] = map[string]any{}
	} else {
		out["result"] = result
	}
	if errorPayload != nil {
		out["error"] = errorPayload
	}
	if strings.TrimSpace(sessionID) != "" {
		out["sessionId"] = strings.TrimSpace(sessionID)
	}
	payload, _ := json.Marshal(out)
	return payload
}

func (s *Server) ensureTargetID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	targetID := strings.TrimSpace(s.connectedTarget.TargetID)
	if targetID == "" {
		targetID = "tab-relay-default"
		s.connectedTarget.TargetID = targetID
	}
	return targetID
}

func (s *Server) ensureSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.connectedSession) == "" {
		seq := s.sessionSeq + 1
		s.sessionSeq = seq
		s.connectedSession = fmt.Sprintf("relay-session-%d", seq)
	}
	return s.connectedSession
}

func (s *Server) setExtensionTarget(targetID, url, title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectedTarget = relayTargetState{
		TargetID: strings.TrimSpace(targetID),
		URL:      strings.TrimSpace(url),
		Title:    strings.TrimSpace(title),
	}
}

func (s *Server) writeToCDP(conn *websocket.Conn, messageType int, payload []byte) {
	if conn == nil || len(payload) == 0 {
		return
	}
	s.cdpWriteMu.Lock()
	err := conn.WriteMessage(messageType, payload)
	s.cdpWriteMu.Unlock()
	if err == nil {
		return
	}
	_ = conn.Close()
	s.mu.Lock()
	delete(s.cdpClients, conn)
	s.mu.Unlock()
	s.clearPendingForConnection(conn, "cdp write failed")
}

func (s *Server) extensionKeepalive(ctx context.Context, conn *websocket.Conn) {
	if conn == nil {
		return
	}
	ticker := time.NewTicker(relayKeepaliveInterval)
	defer ticker.Stop()
	pingPayload, _ := json.Marshal(map[string]any{"method": "ping"})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			lastPong := s.extLastPongAt
			s.mu.RUnlock()
			if !lastPong.IsZero() && time.Since(lastPong) > relayStalePongTimeout {
				_ = conn.Close()
				return
			}
			s.extWriteMu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, pingPayload)
			s.extWriteMu.Unlock()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.extLastPingAt = time.Now().UTC()
			s.mu.Unlock()
		}
	}
}

func originAllowed(origin string, allowlist []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	if len(allowlist) == 0 {
		return true
	}
	for _, pattern := range allowlist {
		if wildcardMatch(pattern, origin) {
			return true
		}
	}
	return false
}

func normalizeAllowlist(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func wildcardMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	idx := 0
	if parts[0] != "" {
		if !strings.HasPrefix(value, parts[0]) {
			return false
		}
		idx = len(parts[0])
	}
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		next := strings.Index(value[idx:], part)
		if next < 0 {
			return false
		}
		idx += next + len(part)
	}
	last := parts[len(parts)-1]
	if last == "" {
		return true
	}
	return strings.HasSuffix(value, last)
}

func isLoopbackRemoteAddr(remote string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remote))
	if err != nil {
		host = strings.TrimSpace(remote)
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func relayTokenFromRequest(r *http.Request, allowQueryToken bool) string {
	if r == nil {
		return ""
	}
	headerToken := strings.TrimSpace(r.Header.Get("Tars-Relay-Token"))
	if headerToken != "" {
		return headerToken
	}
	if !allowQueryToken {
		return ""
	}
	queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if queryToken != "" {
		return queryToken
	}
	return strings.TrimSpace(r.URL.Query().Get("relay_token"))
}

func (s *Server) authorizeRelayRequest(w http.ResponseWriter, r *http.Request) bool {
	if r == nil {
		http.Error(w, "request is required", http.StatusBadRequest)
		return false
	}
	if !isLoopbackRemoteAddr(r.RemoteAddr) {
		http.Error(w, "loopback required", http.StatusForbidden)
		return false
	}
	if relayTokenFromRequest(r, s.opts.AllowQueryToken) != strings.TrimSpace(s.opts.RelayToken) {
		http.Error(w, "missing or invalid relay token", http.StatusUnauthorized)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

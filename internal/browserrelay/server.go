package browserrelay

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
	OriginAllowlist []string
}

// Server provides OpenClaw-style extension relay endpoints.
type Server struct {
	opts Options

	listener net.Listener
	httpSrv  *http.Server
	addr     string
	started  atomic.Bool

	mu            sync.RWMutex
	extConn       *websocket.Conn
	extAttachedAt time.Time
	cdpConn       *websocket.Conn

	extWriteMu sync.Mutex
	cdpWriteMu sync.Mutex
}

func New(opts Options) (*Server, error) {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1:43182"
	}
	token := strings.TrimSpace(opts.RelayToken)
	if token == "" {
		token = fmt.Sprintf("relay-%d", time.Now().UnixNano())
	}
	allow := normalizeAllowlist(opts.OriginAllowlist)
	if len(allow) == 0 {
		allow = []string{"chrome-extension://*"}
	}
	return &Server{opts: Options{Addr: addr, RelayToken: token, OriginAllowlist: allow}}, nil
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
	mux.HandleFunc("/json/version", s.handleVersion)
	mux.HandleFunc("/json/list", s.handleList)
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

func (s *Server) Close(ctx context.Context) error {
	if s == nil || !s.started.Load() {
		return nil
	}
	s.started.Store(false)

	s.mu.Lock()
	ext := s.extConn
	cdp := s.cdpConn
	s.extConn = nil
	s.cdpConn = nil
	s.mu.Unlock()

	if ext != nil {
		_ = ext.Close()
	}
	if cdp != nil {
		_ = cdp.Close()
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

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	payload := map[string]any{
		"Browser": "Tars Relay",
	}
	if s.hasExtension() {
		payload["webSocketDebuggerUrl"] = "ws://" + s.Addr() + "/cdp"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	list := []map[string]any{}
	if s.hasExtension() {
		list = append(list, map[string]any{
			"id":                   "tars-relay",
			"title":                "Tars Relay",
			"type":                 "page",
			"url":                  "about:blank",
			"webSocketDebuggerUrl": "ws://" + s.Addr() + "/cdp",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleExtension(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRemoteAddr(r.RemoteAddr) {
		http.Error(w, "loopback required", http.StatusForbidden)
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
	s.extConn = conn
	s.extAttachedAt = time.Now().UTC()
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}

	go s.forwardExtensionToCDP(conn)
}

func (s *Server) handleCDP(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRemoteAddr(r.RemoteAddr) {
		http.Error(w, "loopback required", http.StatusForbidden)
		return
	}
	if strings.TrimSpace(r.Header.Get("Tars-Relay-Token")) != s.opts.RelayToken {
		http.Error(w, "missing or invalid relay token", http.StatusUnauthorized)
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
	old := s.cdpConn
	s.cdpConn = conn
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		s.extWriteMu.Lock()
		err = ext.WriteMessage(messageType, payload)
		s.extWriteMu.Unlock()
		if err != nil {
			break
		}
	}

	s.mu.Lock()
	if s.cdpConn == conn {
		s.cdpConn = nil
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (s *Server) forwardExtensionToCDP(conn *websocket.Conn) {
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		s.mu.RLock()
		cdp := s.cdpConn
		s.mu.RUnlock()
		if cdp == nil {
			continue
		}
		s.cdpWriteMu.Lock()
		err = cdp.WriteMessage(messageType, payload)
		s.cdpWriteMu.Unlock()
		if err != nil {
			_ = cdp.Close()
			s.mu.Lock()
			if s.cdpConn == cdp {
				s.cdpConn = nil
			}
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	if s.extConn == conn {
		s.extConn = nil
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (s *Server) hasExtension() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.extConn != nil
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

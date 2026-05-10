package tarsserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

var errTerminalUnsupported = errors.New("external terminal launch is only supported on macOS")

type terminalOpenResult struct {
	OK      bool   `json:"ok"`
	CWD     string `json:"cwd"`
	App     string `json:"app"`
	Message string `json:"message,omitempty"`
}

type terminalOpenFunc func(context.Context, string) (terminalOpenResult, error)

const (
	defaultTerminalCols = 80
	defaultTerminalRows = 24
	minTerminalCols     = 20
	minTerminalRows     = 5
	maxTerminalCols     = 500
	maxTerminalRows     = 200
)

type terminalSize struct {
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
}

type terminalWSMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Message string `json:"message,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

type terminalSession interface {
	io.Reader
	io.Writer
	Resize(terminalSize) error
	Close() error
}

type terminalStartFunc func(context.Context, string, terminalSize) (terminalSession, error)

type terminalHandlerDeps struct {
	OpenExternal terminalOpenFunc
	StartSession terminalStartFunc
}

func newTerminalAPIHandler(workspaceDir string, store *session.Store, logger zerolog.Logger) http.Handler {
	return newTerminalAPIHandlerWithDeps(workspaceDir, store, terminalHandlerDeps{
		OpenExternal: openExternalTerminal,
		StartSession: startPTYTerminalSession,
	}, logger)
}

func newTerminalAPIHandlerWithOpener(workspaceDir string, store *session.Store, opener terminalOpenFunc, logger zerolog.Logger) http.Handler {
	return newTerminalAPIHandlerWithDeps(workspaceDir, store, terminalHandlerDeps{
		OpenExternal: opener,
		StartSession: startPTYTerminalSession,
	}, logger)
}

func newTerminalAPIHandlerWithDeps(workspaceDir string, store *session.Store, deps terminalHandlerDeps, logger zerolog.Logger) http.Handler {
	openExternal := deps.OpenExternal
	if openExternal == nil {
		openExternal = openExternalTerminal
	}
	startSession := deps.StartSession
	if startSession == nil {
		startSession = startPTYTerminalSession
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/terminal/open", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if strings.TrimSpace(serverauth.RoleFromRequest(r)) != serverauth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			CWD       string `json:"cwd"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}

		sess, err := resolveTerminalSession(workspaceDir, store, r, sessionID, logger)
		if err != nil {
			writeTerminalSessionResolveError(w, err)
			return
		}

		cwd, err := resolveTerminalCWD(sess, req.CWD)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		result, err := openExternal(r.Context(), cwd)
		if err != nil {
			if errors.Is(err, errTerminalUnsupported) {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": err.Error()})
				return
			}
			logger.Error().Err(err).Str("cwd", cwd).Msg("open external terminal failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result.OK = true
		if strings.TrimSpace(result.CWD) == "" {
			result.CWD = cwd
		}
		if strings.TrimSpace(result.App) == "" {
			result.App = "Terminal"
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/v1/terminal/ws", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if strings.TrimSpace(serverauth.RoleFromRequest(r)) != serverauth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}
		sess, err := resolveTerminalSession(workspaceDir, store, r, sessionID, logger)
		if err != nil {
			writeTerminalSessionResolveError(w, err)
			return
		}
		cwd, err := resolveTerminalCWD(sess, r.URL.Query().Get("cwd"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		size := terminalSizeFromRequest(r)
		term, err := startSession(r.Context(), cwd, size)
		if err != nil {
			logger.Error().Err(err).Str("cwd", cwd).Msg("start integrated terminal failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		serveTerminalWebSocket(w, r, term, cwd, size, logger)
	})

	return mux
}

var errTerminalSessionNotFound = errors.New("session not found")

func resolveTerminalSession(workspaceDir string, store *session.Store, r *http.Request, sessionID string, logger zerolog.Logger) (session.Session, error) {
	reqStore, _, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
	if err != nil {
		logger.Error().Err(err).Msg("resolve workspace session store failed")
		return session.Session{}, fmt.Errorf("resolve workspace failed: %w", err)
	}
	if reqStore == nil {
		return session.Session{}, fmt.Errorf("session store is not configured")
	}
	actualSessionID := strings.TrimSpace(sessionID)
	if strings.EqualFold(actualSessionID, "main") {
		mainSession, err := reqStore.EnsureMain()
		if err != nil {
			logger.Error().Err(err).Msg("resolve main session failed")
			return session.Session{}, fmt.Errorf("resolve main session failed: %w", err)
		}
		actualSessionID = strings.TrimSpace(mainSession.ID)
	}
	sess, err := reqStore.Get(actualSessionID)
	if err != nil {
		return session.Session{}, errTerminalSessionNotFound
	}
	return sess, nil
}

func writeTerminalSessionResolveError(w http.ResponseWriter, err error) {
	if errors.Is(err, errTerminalSessionNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func resolveTerminalCWD(sess session.Session, rawCWD string) (string, error) {
	cwd := strings.TrimSpace(rawCWD)
	if cwd == "" {
		cwd = strings.TrimSpace(sess.CurrentDir)
	}
	if cwd == "" && len(sess.WorkDirs) > 0 {
		cwd = strings.TrimSpace(sess.WorkDirs[0])
	}
	if cwd == "" {
		return "", fmt.Errorf("cwd is required")
	}
	if !filepath.IsAbs(cwd) {
		base := strings.TrimSpace(sess.CurrentDir)
		if base == "" && len(sess.WorkDirs) > 0 {
			base = strings.TrimSpace(sess.WorkDirs[0])
		}
		if base == "" {
			return "", fmt.Errorf("current_dir is required for relative cwd")
		}
		cwd = filepath.Join(base, cwd)
	}

	canonicalCWD := canonicalWorkspacePath(cwd)
	allowed := false
	for _, dir := range sess.WorkDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if workspacePathWithinRoot(dir, canonicalCWD) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("cwd must be inside a session working directory")
	}

	info, err := os.Stat(canonicalCWD)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("directory not found")
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd is not a directory")
	}
	return canonicalCWD, nil
}

func terminalSizeFromRequest(r *http.Request) terminalSize {
	if r == nil {
		return normalizeTerminalSize(0, 0)
	}
	cols, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("cols")))
	rows, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("rows")))
	return normalizeTerminalSize(cols, rows)
}

func normalizeTerminalSize(cols int, rows int) terminalSize {
	return terminalSize{
		Cols: clampTerminalDimension(cols, defaultTerminalCols, minTerminalCols, maxTerminalCols),
		Rows: clampTerminalDimension(rows, defaultTerminalRows, minTerminalRows, maxTerminalRows),
	}
}

func clampTerminalDimension(value int, fallback int, minValue int, maxValue int) int {
	if value <= 0 {
		value = fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func terminalWinsize(size terminalSize) *pty.Winsize {
	normalized := normalizeTerminalSize(size.Cols, size.Rows)
	return &pty.Winsize{Cols: uint16(normalized.Cols), Rows: uint16(normalized.Rows)}
}

func serveTerminalWebSocket(w http.ResponseWriter, r *http.Request, term terminalSession, cwd string, size terminalSize, logger zerolog.Logger) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = term.Close()
		return
	}
	defer conn.Close()
	defer term.Close()

	var writeMu sync.Mutex
	if err := writeTerminalWS(conn, &writeMu, terminalWSMessage{Type: "ready", CWD: cwd, Cols: size.Cols, Rows: size.Rows}); err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, readErr := term.Read(buf)
			if n > 0 {
				msg := terminalWSMessage{
					Type: "output",
					Data: terminalEncodeData(buf[:n]),
				}
				if err := writeTerminalWS(conn, &writeMu, msg); err != nil {
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, os.ErrClosed) {
					logger.Debug().Err(readErr).Msg("integrated terminal read ended")
				}
				_ = writeTerminalWS(conn, &writeMu, terminalWSMessage{Type: "exit"})
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}
		var msg terminalWSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		switch strings.TrimSpace(msg.Type) {
		case "input":
			data, err := terminalDecodeData(msg.Data)
			if err != nil {
				_ = writeTerminalWS(conn, &writeMu, terminalWSMessage{Type: "error", Message: err.Error()})
				continue
			}
			if len(data) > 0 {
				if _, err := term.Write(data); err != nil {
					_ = writeTerminalWS(conn, &writeMu, terminalWSMessage{Type: "error", Message: err.Error()})
				}
			}
		case "resize":
			size := normalizeTerminalSize(msg.Cols, msg.Rows)
			if err := term.Resize(size); err != nil {
				_ = writeTerminalWS(conn, &writeMu, terminalWSMessage{Type: "error", Message: err.Error()})
			}
		case "close":
			return
		}
	}
}

func writeTerminalWS(conn *websocket.Conn, writeMu *sync.Mutex, msg terminalWSMessage) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteJSON(msg)
}

func terminalEncodeData(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

func terminalDecodeData(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func openExternalTerminal(ctx context.Context, cwd string) (terminalOpenResult, error) {
	if runtime.GOOS != "darwin" {
		return terminalOpenResult{}, errTerminalUnsupported
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/open", "-a", "Terminal", cwd)
	if err := cmd.Run(); err != nil {
		return terminalOpenResult{}, err
	}
	return terminalOpenResult{
		OK:      true,
		CWD:     cwd,
		App:     "Terminal",
		Message: "opened external terminal",
	}, nil
}

type ptyTerminalSession struct {
	file      *os.File
	cmd       *exec.Cmd
	done      chan error
	closeOnce sync.Once
}

func startPTYTerminalSession(ctx context.Context, cwd string, size terminalSize) (terminalSession, error) {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shell)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	winSize := terminalWinsize(size)
	file, err := pty.StartWithSize(cmd, winSize)
	if err != nil {
		return nil, err
	}
	session := &ptyTerminalSession{
		file: file,
		cmd:  cmd,
		done: make(chan error, 1),
	}
	go func() {
		session.done <- cmd.Wait()
		close(session.done)
	}()
	return session, nil
}

func (s *ptyTerminalSession) Read(p []byte) (int, error) {
	return s.file.Read(p)
}

func (s *ptyTerminalSession) Write(p []byte) (int, error) {
	return s.file.Write(p)
}

func (s *ptyTerminalSession) Resize(size terminalSize) error {
	return pty.Setsize(s.file, terminalWinsize(size))
}

func (s *ptyTerminalSession) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.file != nil {
			closeErr = s.file.Close()
		}
		if s.cmd == nil || s.cmd.Process == nil {
			return
		}
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.cmd.Process.Signal(os.Interrupt)
		select {
		case <-s.done:
		case <-time.After(300 * time.Millisecond):
			_ = s.cmd.Process.Kill()
			select {
			case <-s.done:
			case <-time.After(time.Second):
			}
		}
	})
	return closeErr
}

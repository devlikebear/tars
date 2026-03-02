package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
)

type ServerConfig = config.MCPServer

type ServerStatus struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type ToolInfo struct {
	Server      string          `json:"server"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type Client struct {
	mu          sync.RWMutex
	servers     []ServerConfig
	sessions    map[string]*pooledSession
	serverModes map[string]rpcMode
	allowlist   map[string]struct{}
	timeout     time.Duration
	reqID       int64
}

type pooledSession struct {
	server      ServerConfig
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	reader      *bufio.Reader
	mode        rpcMode
	mu          sync.Mutex
	initialized bool
	closed      bool
}

type rpcMode int

const (
	rpcModeContentLength rpcMode = iota
	rpcModeJSONLine
)

func (m rpcMode) String() string {
	switch m {
	case rpcModeJSONLine:
		return "jsonline"
	default:
		return "content-length"
	}
}

func NewClient(servers []ServerConfig) *Client {
	client := &Client{
		timeout:     15 * time.Second,
		sessions:    map[string]*pooledSession{},
		serverModes: map[string]rpcMode{},
		allowlist:   map[string]struct{}{},
	}
	client.SetServers(servers)
	return client
}

func (c *Client) SetCommandAllowlist(commands []string) {
	normalized := normalizeCommandAllowlist(commands)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allowlist = normalized
}

func (c *Client) SetServers(servers []ServerConfig) {
	normalized := normalizeServers(servers)
	c.mu.Lock()
	defer c.mu.Unlock()

	nextNames := map[string]struct{}{}
	for _, server := range normalized {
		nextNames[server.Name] = struct{}{}
		inferred := inferRPCMode(server)
		if prev, ok := c.serverModes[server.Name]; ok && prev == rpcModeJSONLine && inferred == rpcModeContentLength {
			continue
		}
		c.serverModes[server.Name] = inferred
	}
	for name, sess := range c.sessions {
		if _, ok := nextNames[name]; ok {
			continue
		}
		sess.close()
		delete(c.sessions, name)
		delete(c.serverModes, name)
	}
	c.servers = normalized
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, sess := range c.sessions {
		sess.close()
		delete(c.sessions, name)
	}
}

func (s *pooledSession) close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
}

func (c *Client) withPersistentSession(ctx context.Context, server ServerConfig, fn func(context.Context, *session) error) error {
	err := c.withPersistentSessionOnce(ctx, server, fn)
	if err == nil {
		return nil
	}
	switchedMode := false
	if c.currentMode(server.Name) == rpcModeContentLength && shouldFallbackToJSONLine(err) {
		c.setServerMode(server.Name, rpcModeJSONLine)
		switchedMode = true
	}
	c.dropSession(server.Name)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		if switchedMode {
			return c.withPersistentSessionOnce(ctx, server, fn)
		}
		return err
	}
	return c.withPersistentSessionOnce(ctx, server, fn)
}

func (c *Client) withPersistentSessionOnce(ctx context.Context, server ServerConfig, fn func(context.Context, *session) error) error {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ps, err := c.getOrStartSession(server)
	if err != nil {
		return err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed {
		return fmt.Errorf("mcp session closed: %s", server.Name)
	}

	sess := &session{
		stdin:  ps.stdin,
		reader: ps.reader,
		abort:  ps.close,
		mode:   ps.mode,
	}
	if !ps.initialized {
		if err := c.initializeSession(callCtx, sess); err != nil {
			ps.close()
			return fmt.Errorf("initialize mcp session (%s): %w", sess.mode.String(), err)
		}
		ps.initialized = true
	}
	if err := fn(callCtx, sess); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "broken pipe") || strings.Contains(strings.ToLower(err.Error()), "eof") {
			ps.close()
		}
		return fmt.Errorf("mcp request failed (%s): %w", sess.mode.String(), err)
	}
	return nil
}

func (c *Client) getOrStartSession(server ServerConfig) (*pooledSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessions == nil {
		c.sessions = map[string]*pooledSession{}
	}
	if existing, ok := c.sessions[server.Name]; ok && !existing.closed {
		return existing, nil
	}

	cmd := exec.Command(server.Command, server.Args...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	if len(server.Env) > 0 {
		cmd.Env = append(os.Environ(), commandEnv(server.Env)...)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server %s: %w", server.Name, err)
	}
	ps := &pooledSession{
		server: server,
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		mode:   rpcModeContentLength,
	}
	if mode, ok := c.serverModes[server.Name]; ok {
		ps.mode = mode
	}
	c.sessions[server.Name] = ps
	return ps, nil
}

func (c *Client) dropSession(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sess, ok := c.sessions[name]
	if !ok {
		return
	}
	sess.close()
	delete(c.sessions, name)
}

func commandEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(extra))
	for k, v := range extra {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		pairs = append(pairs, key+"="+v)
	}
	return pairs
}

func inferRPCMode(server ServerConfig) rpcMode {
	if strings.TrimSpace(server.Command) == "mcp-server-sequential-thinking" {
		return rpcModeJSONLine
	}
	joinedArgs := strings.Join(server.Args, " ")
	if strings.Contains(joinedArgs, "@modelcontextprotocol/server-sequential-thinking") {
		return rpcModeJSONLine
	}
	return rpcModeContentLength
}

func shouldFallbackToJSONLine(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "missing content-length header") ||
		strings.Contains(msg, "invalid content-length header") ||
		strings.Contains(msg, "read rpc header line")
}

func (c *Client) currentMode(serverName string) rpcMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	mode, ok := c.serverModes[serverName]
	if !ok {
		return rpcModeContentLength
	}
	return mode
}

func (c *Client) setServerMode(serverName string, mode rpcMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serverModes == nil {
		c.serverModes = map[string]rpcMode{}
	}
	c.serverModes[serverName] = mode
}

func (c *Client) serverSnapshot() []ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ServerConfig, len(c.servers))
	copy(out, c.servers)
	return out
}

func normalizeServers(servers []ServerConfig) []ServerConfig {
	copyServers := make([]ServerConfig, 0, len(servers))
	for _, s := range servers {
		name := strings.TrimSpace(s.Name)
		command := strings.TrimSpace(s.Command)
		if name == "" || command == "" {
			continue
		}
		copied := ServerConfig{
			Name:    name,
			Command: command,
			Args:    append([]string(nil), s.Args...),
		}
		if len(s.Env) > 0 {
			copied.Env = make(map[string]string, len(s.Env))
			for k, v := range s.Env {
				copied.Env[k] = v
			}
		}
		copyServers = append(copyServers, copied)
	}
	return copyServers
}

func normalizeCommandAllowlist(commands []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, command := range commands {
		trimmed := strings.ToLower(strings.TrimSpace(command))
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func isCommandAllowed(command string, allowlist map[string]struct{}) bool {
	if len(allowlist) == 0 {
		return false
	}
	trimmed := strings.ToLower(strings.TrimSpace(command))
	if trimmed == "" {
		return false
	}
	if _, ok := allowlist[trimmed]; ok {
		return true
	}
	base := strings.ToLower(strings.TrimSpace(filepath.Base(trimmed)))
	_, ok := allowlist[base]
	return ok
}

func (c *Client) validateServerCommands() error {
	c.mu.RLock()
	servers := append([]ServerConfig(nil), c.servers...)
	allowlist := map[string]struct{}{}
	for command := range c.allowlist {
		allowlist[command] = struct{}{}
	}
	c.mu.RUnlock()

	for _, server := range servers {
		if isCommandAllowed(server.Command, allowlist) {
			continue
		}
		return fmt.Errorf(
			"mcp server %q command %q is blocked by mcp_command_allowlist_json",
			strings.TrimSpace(server.Name),
			strings.TrimSpace(server.Command),
		)
	}
	return nil
}

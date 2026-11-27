// Package toolbridge serves TARS tools to a CLI provider over MCP.
//
// A CLI provider (claude-code-cli) runs its own tools in its own process and
// never sees the tool schemas TARS hands a native provider. The bridge gives
// such a turn a Model Context Protocol endpoint on a loopback port: the
// provider lists the turn's TARS tools there and calls them, and each call
// runs in the server with the turn's own context, exactly as the agent loop
// would have run it.
//
// One listener serves every turn. A turn is a random bearer token that maps
// to the tools it may call; the token is made when the turn opens and is
// worthless the moment it closes. Nothing is served without a live token, so
// another local process cannot reach a tool.
//
// The transport is the streamable HTTP one, reduced to what a tool server
// needs: JSON-RPC over POST, answered as plain JSON. There is no stream from
// the server and no session.
package toolbridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/pkg/tools"
)

// ServerName is the MCP server name the bridge is mounted under. Claude Code
// names the tools mcp__<ServerName>__<tool>.
const ServerName = "tars"

// Call is one finished tool call, reported to Turn.AfterCall.
type Call struct {
	Name    string
	Args    string
	Result  string
	IsError bool
}

// Turn is what one chat turn exposes.
type Turn struct {
	// Context is the turn's context. Calls run under it (cut short when the
	// provider drops the request), so tools see the same session values a
	// native turn gives them.
	Context context.Context
	// Tools are the tools the provider may list and call.
	Tools []tools.Tool
	// Instructions is sent to the provider as the server's instructions.
	Instructions string
	// Redact, when set, is applied to a result before it leaves the server.
	Redact func(string) string
	// Busy, when set, is called when a call starts; the function it returns
	// is called when the call ends. A turn uses it to stop its idle clock
	// while the provider waits on TARS.
	Busy func() (done func())
	// AfterCall, when set, is told about each finished call.
	AfterCall func(ctx context.Context, call Call)
}

// Endpoint is where a provider reaches an open turn.
type Endpoint struct {
	URL   string
	Token string
}

// Headers are the request headers the provider must send.
func (e Endpoint) Headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + e.Token}
}

type openTurn struct {
	Turn
	byName map[string]tools.Tool
	// calls run one at a time, as they do in the agent loop: the tools were
	// never written to run side by side within one turn.
	calls sync.Mutex
}

// Bridge is the listener and its open turns. The zero value is not usable;
// call New.
type Bridge struct {
	mu     sync.Mutex
	turns  map[string]*openTurn
	ln     net.Listener
	server *http.Server
	closed bool
}

// New returns a bridge. It listens only once a turn is opened.
func New() *Bridge {
	return &Bridge{turns: map[string]*openTurn{}}
}

// Open exposes turn until the returned function is called. It fails when the
// turn has no tools or the loopback listener cannot start.
func (b *Bridge) Open(turn Turn) (Endpoint, func(), error) {
	if b == nil {
		return Endpoint{}, func() {}, errors.New("tool bridge is not configured")
	}
	if len(turn.Tools) == 0 {
		return Endpoint{}, func() {}, errors.New("tool bridge: no tools to expose")
	}
	if turn.Context == nil {
		turn.Context = context.Background()
	}
	open := &openTurn{Turn: turn, byName: make(map[string]tools.Tool, len(turn.Tools))}
	for _, t := range turn.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" || t.Execute == nil {
			continue
		}
		open.byName[name] = t
	}
	if len(open.byName) == 0 {
		return Endpoint{}, func() {}, errors.New("tool bridge: no callable tools")
	}
	token, err := newToken()
	if err != nil {
		return Endpoint{}, func() {}, fmt.Errorf("tool bridge: token: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	addr, err := b.listenLocked()
	if err != nil {
		return Endpoint{}, func() {}, err
	}
	key := tokenKey(token)
	b.turns[key] = open
	closeTurn := func() {
		b.mu.Lock()
		delete(b.turns, key)
		b.mu.Unlock()
	}
	return Endpoint{URL: "http://" + addr + "/mcp", Token: token}, closeTurn, nil
}

// Close stops the listener. Open turns stop answering.
func (b *Bridge) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	server := b.server
	b.server, b.ln, b.closed = nil, nil, true
	b.turns = map[string]*openTurn{}
	b.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Close()
}

func (b *Bridge) listenLocked() (string, error) {
	if b.closed {
		return "", errors.New("tool bridge is closed")
	}
	if b.ln != nil {
		return b.ln.Addr().String(), nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("tool bridge: listen: %w", err)
	}
	server := &http.Server{Handler: b, ReadHeaderTimeout: 10 * time.Second}
	b.ln, b.server = ln, server
	go func() { _ = server.Serve(ln) }()
	return ln.Addr().String(), nil
}

func (b *Bridge) turnFor(r *http.Request) *openTurn {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return nil
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// The map is keyed by the token's hash, so the lookup's timing says
	// nothing about how much of a guessed token was right.
	return b.turns[tokenKey(token)]
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (t *openTurn) sortedTools() []tools.Tool {
	out := make([]tools.Tool, 0, len(t.byName))
	for _, tl := range t.byName {
		out = append(out, tl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

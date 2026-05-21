package mcp

import (
	"context"

	"github.com/devlikebear/tars/internal/config"
	internal "github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/pkg/tools"
)

const (
	TransportStdio          = config.MCPTransportStdio
	TransportStreamableHTTP = config.MCPTransportStreamableHTTP
	TransportSSE            = config.MCPTransportSSE
	TransportWebSocket      = config.MCPTransportWebSocket
)

// ServerConfig is the public MCP server description understood by this
// package. It intentionally mirrors the portable MCP fields without exposing
// TARS' full runtime config package.
type ServerConfig struct {
	Name          string
	Command       string
	Args          []string
	Env           map[string]string
	Transport     string
	URL           string
	Headers       map[string]string
	AuthMode      string
	AuthTokenEnv  string
	OAuthProvider string
	Source        string
}

type ServerStatus = internal.ServerStatus
type ToolInfo = internal.ToolInfo

// Client wraps TARS' MCP client behind public config and tool types.
type Client struct {
	inner *internal.Client
}

func NewClient(servers []ServerConfig) *Client {
	return &Client{inner: internal.NewClient(toInternalServers(servers))}
}

func (c *Client) SetCommandAllowlist(commands []string) {
	c.ensure().SetCommandAllowlist(commands)
}

func (c *Client) SetServers(servers []ServerConfig) {
	c.ensure().SetServers(toInternalServers(servers))
}

func (c *Client) Close() {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.Close()
}

func (c *Client) ListServers(ctx context.Context) ([]ServerStatus, error) {
	return c.ensure().ListServers(ctx)
}

func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	return c.ensure().ListTools(ctx)
}

func (c *Client) BuildTools(ctx context.Context) ([]tools.Tool, error) {
	return c.ensure().BuildTools(ctx)
}

func (c *Client) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (tools.Result, error) {
	return c.ensure().CallTool(ctx, serverName, toolName, args)
}

func MCPToolName(serverName, toolName string) string {
	return internal.MCPToolName(serverName, toolName)
}

func (c *Client) ensure() *internal.Client {
	if c == nil {
		return internal.NewClient(nil)
	}
	if c.inner == nil {
		c.inner = internal.NewClient(nil)
	}
	return c.inner
}

func toInternalServers(servers []ServerConfig) []config.MCPServer {
	if len(servers) == 0 {
		return nil
	}
	out := make([]config.MCPServer, 0, len(servers))
	for _, server := range servers {
		out = append(out, config.MCPServer{
			Name:          server.Name,
			Command:       server.Command,
			Args:          append([]string(nil), server.Args...),
			Env:           cloneStringMap(server.Env),
			Transport:     server.Transport,
			URL:           server.URL,
			Headers:       cloneStringMap(server.Headers),
			AuthMode:      server.AuthMode,
			AuthTokenEnv:  server.AuthTokenEnv,
			OAuthProvider: server.OAuthProvider,
			Source:        server.Source,
		})
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

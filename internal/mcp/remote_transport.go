package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/auth"
	"github.com/devlikebear/tars/internal/config"
	"github.com/gorilla/websocket"
)

func (c *Client) requestStreamableHTTP(ctx context.Context, sess *session, req rpcRequest) (json.RawMessage, error) {
	ps := sess.pooled
	if ps == nil || ps.httpClient == nil {
		return nil, fmt.Errorf("streamable http session not initialized")
	}
	resp, err := c.doHTTPRPC(ctx, ps, ps.server.URL, req, true)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("missing http rpc response")
	}
	return resp.Result, nil
}

func (c *Client) notifyStreamableHTTP(ctx context.Context, sess *session, req rpcRequest) error {
	ps := sess.pooled
	if ps == nil || ps.httpClient == nil {
		return fmt.Errorf("streamable http session not initialized")
	}
	_, err := c.doHTTPRPC(ctx, ps, ps.server.URL, req, false)
	return err
}

func (c *Client) requestLegacySSE(ctx context.Context, sess *session, req rpcRequest) (json.RawMessage, error) {
	ps := sess.pooled
	if ps == nil || ps.httpClient == nil {
		return nil, fmt.Errorf("legacy sse session not initialized")
	}
	if err := c.ensureLegacySSEStream(ctx, ps); err != nil {
		return nil, err
	}
	resp, err := c.doHTTPRPC(ctx, ps, ps.ssePostURL, req, false)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return resp.Result, nil
	}
	return c.readLegacySSEResponse(ctx, ps, req.ID)
}

func (c *Client) notifyLegacySSE(ctx context.Context, sess *session, req rpcRequest) error {
	ps := sess.pooled
	if ps == nil || ps.httpClient == nil {
		return fmt.Errorf("legacy sse session not initialized")
	}
	if err := c.ensureLegacySSEStream(ctx, ps); err != nil {
		return err
	}
	_, err := c.doHTTPRPC(ctx, ps, ps.ssePostURL, req, false)
	return err
}

func (c *Client) requestWebSocket(ctx context.Context, sess *session, req rpcRequest) (json.RawMessage, error) {
	ps := sess.pooled
	if ps == nil || ps.wsConn == nil {
		return nil, fmt.Errorf("websocket session not initialized")
	}
	if err := writeWebSocketJSON(ctx, ps.wsConn, req); err != nil {
		return nil, err
	}
	resp, err := readWebSocketResponse(ctx, ps.wsConn, req.ID)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp rpc error (%d): %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *Client) notifyWebSocket(ctx context.Context, sess *session, req rpcRequest) error {
	ps := sess.pooled
	if ps == nil || ps.wsConn == nil {
		return fmt.Errorf("websocket session not initialized")
	}
	return writeWebSocketJSON(ctx, ps.wsConn, req)
}

func (c *Client) doHTTPRPC(ctx context.Context, ps *pooledSession, endpoint string, req rpcRequest, acceptSSE bool) (*rpcResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build http rpc request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if acceptSSE {
		httpReq.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json, text/event-stream")
	}
	if ps.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", ps.sessionID)
	}
	headers, err := authHeadersForServer(ps.server)
	if err != nil {
		return nil, err
	}
	copyHeaders(httpReq.Header, headers)

	resp, err := ps.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http rpc request: %w", err)
	}
	defer resp.Body.Close()
	if sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sessionID != "" {
		ps.sessionID = sessionID
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("http rpc status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if req.ID == 0 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(contentType, "text/event-stream") {
		return readSSEJSONRPC(ctx, bufio.NewReader(resp.Body), req.ID)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read http rpc response: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, fmt.Errorf("decode http rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp rpc error (%d): %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return &rpcResp, nil
}

func (c *Client) ensureLegacySSEStream(ctx context.Context, ps *pooledSession) error {
	if ps.sseReader != nil && ps.sseBody != nil && ps.ssePostURL != "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ps.server.URL, nil)
	if err != nil {
		return fmt.Errorf("build sse connect request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	headers, err := authHeadersForServer(ps.server)
	if err != nil {
		return err
	}
	copyHeaders(req.Header, headers)
	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect legacy sse transport: %w", err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return fmt.Errorf("legacy sse connect status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	reader := bufio.NewReader(resp.Body)
	baseURL, _ := url.Parse(ps.server.URL)
	for {
		event, err := readSSEEvent(reader)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}
		if strings.EqualFold(strings.TrimSpace(event.Event), "endpoint") {
			postURL, err := resolveEndpointURL(baseURL, event.Data)
			if err != nil {
				_ = resp.Body.Close()
				return err
			}
			ps.sseBody = resp.Body
			ps.sseReader = reader
			ps.ssePostURL = postURL
			return nil
		}
	}
}

func (c *Client) readLegacySSEResponse(ctx context.Context, ps *pooledSession, reqID int64) (json.RawMessage, error) {
	if ps.sseReader == nil {
		return nil, fmt.Errorf("legacy sse stream is not open")
	}
	resp, err := readSSEJSONRPC(ctx, ps.sseReader, reqID)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func readSSEJSONRPC(ctx context.Context, reader *bufio.Reader, reqID int64) (*rpcResponse, error) {
	for {
		event, err := readSSEEventWithContext(ctx, reader)
		if err != nil {
			return nil, err
		}
		payload := strings.TrimSpace(event.Data)
		if payload == "" || !json.Valid([]byte(payload)) {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			continue
		}
		if resp.ID != reqID {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp rpc error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return &resp, nil
	}
}

type sseEvent struct {
	Event string
	Data  string
}

func readSSEEventWithContext(ctx context.Context, reader *bufio.Reader) (sseEvent, error) {
	type readResult struct {
		event sseEvent
		err   error
	}
	readCh := make(chan readResult, 1)
	go func() {
		event, err := readSSEEvent(reader)
		readCh <- readResult{event: event, err: err}
	}()
	select {
	case <-ctx.Done():
		return sseEvent{}, ctx.Err()
	case result := <-readCh:
		return result.event, result.err
	}
}

func readSSEEvent(reader *bufio.Reader) (sseEvent, error) {
	var event sseEvent
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return sseEvent{}, fmt.Errorf("read sse line: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			event.Data = strings.Join(dataLines, "\n")
			return event, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "event":
			event.Event = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
}

func resolveEndpointURL(base *url.URL, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("legacy sse endpoint event missing data")
	}
	endpoint, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse legacy sse endpoint: %w", err)
	}
	if base == nil {
		return endpoint.String(), nil
	}
	return base.ResolveReference(endpoint).String(), nil
}

func (c *Client) dialWebSocket(ctx context.Context, server ServerConfig) (*websocket.Conn, error) {
	headers, err := authHeadersForServer(server)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, server.URL, headers)
	if err != nil {
		return nil, fmt.Errorf("dial websocket mcp server %s: %w", server.Name, err)
	}
	return conn, nil
}

func writeWebSocketJSON(ctx context.Context, conn *websocket.Conn, payload any) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	} else {
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	}
	return conn.WriteJSON(payload)
}

func readWebSocketResponse(ctx context.Context, conn *websocket.Conn, reqID int64) (*rpcResponse, error) {
	for {
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetReadDeadline(deadline)
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read websocket rpc response: %w", err)
		}
		var resp rpcResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		if resp.ID != reqID {
			continue
		}
		return &resp, nil
	}
}

func authHeadersForServer(server ServerConfig) (http.Header, error) {
	headers := http.Header{}
	for key, value := range server.Headers {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		headers.Set(trimmedKey, value)
	}
	switch strings.TrimSpace(strings.ToLower(server.AuthMode)) {
	case "", config.MCPAuthModeNone:
		return headers, nil
	case config.MCPAuthModeBearer:
		envKey := strings.TrimSpace(server.AuthTokenEnv)
		if envKey == "" {
			return nil, fmt.Errorf("mcp server %q bearer auth requires auth_token_env", server.Name)
		}
		token := strings.TrimSpace(os.Getenv(envKey))
		if token == "" {
			return nil, fmt.Errorf("mcp server %q bearer token env %q is empty", server.Name, envKey)
		}
		headers.Set("Authorization", "Bearer "+token)
		return headers, nil
	case config.MCPAuthModeOAuth:
		token, err := auth.ResolveToken(auth.ResolveOptions{
			Provider:      "mcp",
			AuthMode:      "oauth",
			OAuthProvider: server.OAuthProvider,
		})
		if err != nil {
			return nil, fmt.Errorf("resolve mcp oauth token for %q: %w", server.Name, err)
		}
		headers.Set("Authorization", "Bearer "+token)
		return headers, nil
	default:
		return nil, fmt.Errorf("unsupported mcp auth mode %q", server.AuthMode)
	}
}

func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

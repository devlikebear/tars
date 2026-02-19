package vaultclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// SecretReader reads secrets from a backing store.
type SecretReader interface {
	ReadKV(ctx context.Context, secretPath string) (map[string]string, error)
}

// ClientOptions configures the Vault read-only client.
type ClientOptions struct {
	Enabled             bool
	Addr                string
	AuthMode            string
	Token               string
	Namespace           string
	Timeout             time.Duration
	KVMount             string
	KVVersion           int
	AppRoleMount        string
	AppRoleRoleID       string
	AppRoleSecretID     string
	SecretPathAllowlist []string
	HTTPClient          *http.Client
}

type client struct {
	enabled   bool
	addr      string
	authMode  string
	token     string
	namespace string
	timeout   time.Duration
	kvMount   string
	kvVersion int

	approleMount    string
	approleRoleID   string
	approleSecretID string

	allowlist []string
	http      *http.Client

	mu            sync.Mutex
	cachedAppRole string
}

// New builds a read-only Vault client.
func New(opts ClientOptions) (SecretReader, error) {
	if !opts.Enabled {
		return &client{enabled: false}, nil
	}
	addr := strings.TrimRight(strings.TrimSpace(opts.Addr), "/")
	if addr == "" {
		addr = "http://127.0.0.1:8200"
	}
	authMode := strings.TrimSpace(strings.ToLower(opts.AuthMode))
	if authMode == "" {
		authMode = "token"
	}
	if authMode != "token" && authMode != "approle" {
		return nil, fmt.Errorf("unsupported vault auth mode: %s", authMode)
	}
	kvMount := strings.Trim(strings.TrimSpace(opts.KVMount), "/")
	if kvMount == "" {
		kvMount = "secret"
	}
	kvVersion := opts.KVVersion
	if kvVersion <= 0 {
		kvVersion = 2
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	allowlist := normalizeAllowlist(opts.SecretPathAllowlist)
	if authMode == "token" && strings.TrimSpace(opts.Token) == "" {
		return nil, fmt.Errorf("vault token is required")
	}
	if authMode == "approle" {
		if strings.TrimSpace(opts.AppRoleRoleID) == "" || strings.TrimSpace(opts.AppRoleSecretID) == "" {
			return nil, fmt.Errorf("vault approle role_id and secret_id are required")
		}
	}
	approleMount := strings.Trim(strings.TrimSpace(opts.AppRoleMount), "/")
	if approleMount == "" {
		approleMount = "approle"
	}

	return &client{
		enabled:         true,
		addr:            addr,
		authMode:        authMode,
		token:           strings.TrimSpace(opts.Token),
		namespace:       strings.TrimSpace(opts.Namespace),
		timeout:         timeout,
		kvMount:         kvMount,
		kvVersion:       kvVersion,
		approleMount:    approleMount,
		approleRoleID:   strings.TrimSpace(opts.AppRoleRoleID),
		approleSecretID: strings.TrimSpace(opts.AppRoleSecretID),
		allowlist:       allowlist,
		http:            httpClient,
	}, nil
}

func (c *client) ReadKV(ctx context.Context, secretPath string) (map[string]string, error) {
	if c == nil || !c.enabled {
		return nil, fmt.Errorf("vault is disabled")
	}
	cleanPath := strings.Trim(strings.TrimSpace(secretPath), "/")
	if cleanPath == "" {
		return nil, fmt.Errorf("vault secret path is required")
	}
	if !c.allowed(cleanPath) {
		return nil, fmt.Errorf("vault secret path not allowed by allowlist: %s", cleanPath)
	}

	values, err := c.readKVOnce(ctx, cleanPath, false)
	if err == nil {
		return values, nil
	}
	if c.authMode == "approle" && strings.Contains(strings.ToLower(err.Error()), "status 403") {
		c.mu.Lock()
		c.cachedAppRole = ""
		c.mu.Unlock()
		return c.readKVOnce(ctx, cleanPath, true)
	}
	return nil, err
}

func (c *client) readKVOnce(ctx context.Context, secretPath string, forceRefresh bool) (map[string]string, error) {
	token, err := c.resolveToken(ctx, forceRefresh)
	if err != nil {
		return nil, err
	}
	requestURL := c.buildReadURL(secretPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build vault request failed: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vault status %d", resp.StatusCode)
	}

	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode vault response failed: %w", err)
	}
	data := payload.Data
	if c.kvVersion == 2 {
		inner, _ := data["data"].(map[string]any)
		if inner == nil {
			inner = map[string]any{}
		}
		data = inner
	}
	out := map[string]string{}
	for k, v := range data {
		out[k] = stringifyValue(v)
	}
	return out, nil
}

func (c *client) resolveToken(ctx context.Context, forceRefresh bool) (string, error) {
	if c.authMode == "token" {
		return c.token, nil
	}
	if !forceRefresh {
		c.mu.Lock()
		cached := c.cachedAppRole
		c.mu.Unlock()
		if cached != "" {
			return cached, nil
		}
	}
	payload := map[string]string{
		"role_id":   c.approleRoleID,
		"secret_id": c.approleSecretID,
	}
	raw, _ := json.Marshal(payload)
	loginMount := strings.TrimPrefix(path.Clean("/"+c.approleMount), "/")
	loginURL := c.addr + "/v1/auth/" + loginMount + "/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("build approle login request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault approle login failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("vault approle login status %d", resp.StatusCode)
	}
	var result struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode approle login response failed: %w", err)
	}
	token := strings.TrimSpace(result.Auth.ClientToken)
	if token == "" {
		return "", fmt.Errorf("vault approle login did not return client token")
	}
	c.mu.Lock()
	c.cachedAppRole = token
	c.mu.Unlock()
	return token, nil
}

func (c *client) buildReadURL(secretPath string) string {
	mount := path.Clean("/" + c.kvMount)
	if c.kvVersion == 2 {
		return c.addr + "/v1" + mount + "/data/" + secretPath
	}
	return c.addr + "/v1" + mount + "/" + secretPath
}

func (c *client) allowed(secretPath string) bool {
	if len(c.allowlist) == 0 {
		return true
	}
	for _, prefix := range c.allowlist {
		if strings.HasPrefix(secretPath, prefix) {
			return true
		}
	}
	return false
}

func normalizeAllowlist(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		v := strings.Trim(strings.TrimSpace(item), "/")
		if v == "" {
			continue
		}
		if !strings.HasSuffix(v, "/") {
			v += "/"
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func stringifyValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

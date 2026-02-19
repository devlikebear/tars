package vaultclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadKVTokenModeV2(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/secret/data/sites/sample" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Vault-Token"); got != "token-123" {
			t.Fatalf("unexpected token header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"username":"alice","password":"redacted"}}}`))
	}))
	defer ts.Close()

	c, err := New(ClientOptions{
		Enabled:             true,
		Addr:                ts.URL,
		AuthMode:            "token",
		Token:               "token-123",
		KVMount:             "secret",
		KVVersion:           2,
		Timeout:             time.Second,
		SecretPathAllowlist: []string{"sites/"},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	values, err := c.ReadKV(context.Background(), "sites/sample")
	if err != nil {
		t.Fatalf("read kv: %v", err)
	}
	if values["username"] != "alice" {
		t.Fatalf("unexpected username: %v", values["username"])
	}
}

func TestReadKVPathAllowlistBlocked(t *testing.T) {
	c, err := New(ClientOptions{
		Enabled:             true,
		Addr:                "http://127.0.0.1:8200",
		AuthMode:            "token",
		Token:               "token-123",
		KVMount:             "secret",
		KVVersion:           2,
		Timeout:             time.Second,
		SecretPathAllowlist: []string{"sites/"},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = c.ReadKV(context.Background(), "infra/admin")
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestReadKVAppRoleMode(t *testing.T) {
	var loginCalls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/approle/login":
			loginCalls++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode login payload: %v", err)
			}
			if payload["role_id"] != "role-1" || payload["secret_id"] != "secret-1" {
				t.Fatalf("unexpected approle payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"auth":{"client_token":"approle-token"}}`))
		case "/v1/secret/data/sites/sample":
			if got := r.Header.Get("X-Vault-Token"); got != "approle-token" {
				t.Fatalf("unexpected token header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"data":{"ok":"yes"}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	c, err := New(ClientOptions{
		Enabled:             true,
		Addr:                ts.URL,
		AuthMode:            "approle",
		AppRoleMount:        "approle",
		AppRoleRoleID:       "role-1",
		AppRoleSecretID:     "secret-1",
		KVMount:             "secret",
		KVVersion:           2,
		Timeout:             time.Second,
		SecretPathAllowlist: []string{"sites/"},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	values, err := c.ReadKV(context.Background(), "sites/sample")
	if err != nil {
		t.Fatalf("read kv: %v", err)
	}
	if values["ok"] != "yes" {
		t.Fatalf("unexpected value: %+v", values)
	}
	if loginCalls != 1 {
		t.Fatalf("expected one approle login call, got %d", loginCalls)
	}
}

func TestNewDisabledClient(t *testing.T) {
	c, err := New(ClientOptions{Enabled: false})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := c.ReadKV(context.Background(), "sites/sample"); err == nil {
		t.Fatalf("expected disabled error")
	}
}

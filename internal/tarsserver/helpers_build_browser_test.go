package tarsserver

import (
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
)

func TestBuildBrowserRelay_UsesConfiguredToken(t *testing.T) {
	cfg := config.Default()
	cfg.BrowserRelayEnabled = true
	cfg.BrowserRelayAddr = "127.0.0.1:0"
	cfg.BrowserRelayToken = "fixed-relay-token"

	relay, err := buildBrowserRelay(cfg)
	if err != nil {
		t.Fatalf("build relay: %v", err)
	}
	if relay == nil {
		t.Fatalf("expected relay instance")
	}
	if relay.RelayToken() != "fixed-relay-token" {
		t.Fatalf("expected configured relay token, got %q", relay.RelayToken())
	}
}

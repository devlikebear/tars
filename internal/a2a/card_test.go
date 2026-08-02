package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverAgentCardSelectsStrictHTTPJSONV1Interface(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != AgentCardPath {
			t.Fatalf("unexpected discovery path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "name":"review-agent",
  "description":"Reviews a proposed change",
  "supportedInterfaces":[
    {"url":"https://legacy.example.test/rpc","protocolBinding":"JSONRPC","protocolVersion":"1.0"},
    {"url":"` + server.URL + `/a2a","protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}
  ],
  "version":"2026.8.0",
  "capabilities":{},
  "defaultInputModes":["text/plain"],
  "defaultOutputModes":["text/plain","application/json"],
  "skills":[{"id":"review","name":"Review","description":"Review a change","tags":["review"]}]
}`))
	}))
	t.Cleanup(server.Close)

	card, endpoint, err := Discover(context.Background(), server.URL, DiscoveryOptions{
		HTTPClient:        server.Client(),
		AllowLoopbackHTTP: true,
	})
	if err != nil {
		t.Fatalf("discover card: %v", err)
	}
	if card.Name != "review-agent" || endpoint.ProtocolBinding != BindingHTTPJSON {
		t.Fatalf("unexpected discovery result: %#v %#v", card, endpoint)
	}
	if endpoint.ProtocolVersion != ProtocolVersion || endpoint.URL != server.URL+"/a2a" {
		t.Fatalf("unexpected selected endpoint: %#v", endpoint)
	}
}

func TestDiscoverAgentCardRejectsRedirectsAndOversizedBodies(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Redirect(w, &http.Request{}, "https://agent.example.test/card", http.StatusFound)
		}))
		t.Cleanup(server.Close)

		_, _, err := Discover(context.Background(), server.URL, DiscoveryOptions{
			HTTPClient:        server.Client(),
			AllowLoopbackHTTP: true,
		})
		if err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Fatalf("expected redirect rejection, got %v", err)
		}
	})

	t.Run("body limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.Repeat("x", 257)))
		}))
		t.Cleanup(server.Close)

		_, _, err := Discover(context.Background(), server.URL, DiscoveryOptions{
			HTTPClient:        server.Client(),
			AllowLoopbackHTTP: true,
			MaxResponseBytes:  256,
		})
		if err == nil || !strings.Contains(err.Error(), "response limit") {
			t.Fatalf("expected response limit rejection, got %v", err)
		}
	})
}

func TestDiscoverAgentCardRejectsUnsafeOrIncompatibleEndpoints(t *testing.T) {
	valid := AgentCard{
		Name:        "agent",
		Description: "agent description",
		SupportedInterfaces: []AgentInterface{{
			URL: "https://agent.example.test/a2a", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion,
		}},
		Version:            "1.0.0",
		Capabilities:       &AgentCapabilities{},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills: []AgentSkill{{
			ID: "agent", Name: "Agent", Description: "Run work", Tags: []string{"work"},
		}},
	}

	tests := []struct {
		name   string
		mutate func(*AgentCard)
		want   string
	}{
		{name: "old protocol", mutate: func(card *AgentCard) { card.SupportedInterfaces[0].ProtocolVersion = "0.3" }, want: "1.0"},
		{name: "wrong binding", mutate: func(card *AgentCard) { card.SupportedInterfaces[0].ProtocolBinding = "JSONRPC" }, want: "HTTP+JSON"},
		{name: "http", mutate: func(card *AgentCard) { card.SupportedInterfaces[0].URL = "http://agent.example.test/a2a" }, want: "https"},
		{name: "credentials", mutate: func(card *AgentCard) { card.SupportedInterfaces[0].URL = "https://user:pass@agent.example.test/a2a" }, want: "userinfo"},
		{name: "missing skills", mutate: func(card *AgentCard) { card.Skills = nil }, want: "skills"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			card := valid
			card.SupportedInterfaces = append([]AgentInterface(nil), valid.SupportedInterfaces...)
			test.mutate(&card)
			_, err := SelectHTTPJSONV1(card, EndpointPolicy{})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestDiscoveryBlocksPrivateHostsByDefault(t *testing.T) {
	_, _, err := Discover(context.Background(), "http://127.0.0.1:43180", DiscoveryOptions{})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected private http endpoint to be blocked, got %v", err)
	}
}

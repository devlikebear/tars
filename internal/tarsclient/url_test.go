package tarsclient

import "testing"

func TestResolveURL_DefaultBaseAndQuery(t *testing.T) {
	got, err := resolveURL("", "/v1/agentruntime/runs?limit=10")
	if err != nil {
		t.Fatalf("resolveURL: %v", err)
	}
	want := "http://127.0.0.1:43180/v1/agentruntime/runs?limit=10"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveURL_BasePath(t *testing.T) {
	got, err := resolveURL("http://127.0.0.1:43180/api", "v1/status")
	if err != nil {
		t.Fatalf("resolveURL: %v", err)
	}
	want := "http://127.0.0.1:43180/api/v1/status"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveURL_BasePathStripsBaseQueryAndFragment(t *testing.T) {
	got, err := resolveURL("http://127.0.0.1:43180/api?debug=1#old", "v1/status?limit=10#section")
	if err != nil {
		t.Fatalf("resolveURL: %v", err)
	}
	want := "http://127.0.0.1:43180/api/v1/status?limit=10#section"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveURL_InvalidBase(t *testing.T) {
	if _, err := resolveURL(":", "/v1/status"); err == nil {
		t.Fatalf("expected invalid server url error")
	}
}

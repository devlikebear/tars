package tarsclient

import "testing"

func TestResolveURLCases(t *testing.T) {
	t.Run("default base", func(t *testing.T) {
		got, err := ResolveURL("", "/v1/status")
		if err != nil {
			t.Fatalf("ResolveURL: %v", err)
		}
		want := "http://127.0.0.1:43180/v1/status"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("base path with query and fragment", func(t *testing.T) {
		got, err := ResolveURL("http://127.0.0.1:43180/proxy?debug=1#old", "v1/status?limit=10#section")
		if err != nil {
			t.Fatalf("ResolveURL: %v", err)
		}
		want := "http://127.0.0.1:43180/proxy/v1/status?limit=10#section"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("invalid base", func(t *testing.T) {
		if _, err := ResolveURL(":", "/v1/status"); err == nil {
			t.Fatalf("expected invalid base error")
		}
	})
}

func TestConsoleURLCases(t *testing.T) {
	t.Run("default base", func(t *testing.T) {
		got, err := ConsoleURL("")
		if err != nil {
			t.Fatalf("ConsoleURL: %v", err)
		}
		want := "http://127.0.0.1:43180/console"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("base path strips query and fragment", func(t *testing.T) {
		got, err := ConsoleURL("http://127.0.0.1:43180/proxy?debug=1#old")
		if err != nil {
			t.Fatalf("ConsoleURL: %v", err)
		}
		want := "http://127.0.0.1:43180/proxy/console"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})
}

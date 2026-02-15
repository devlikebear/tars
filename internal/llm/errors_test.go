package llm

import (
	"errors"
	"testing"
)

func TestProviderError_Error_HTTP(t *testing.T) {
	err := newHTTPError("bifrost", 429, "  too many requests \n")

	got := err.Error()
	if got != "bifrost status 429: too many requests" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestProviderError_Error_NonHTTPWithCause(t *testing.T) {
	cause := errors.New("dial tcp timeout")
	err := newProviderError("anthropic", "request", cause)

	got := err.Error()
	if got != "anthropic request: dial tcp timeout" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestProviderError_Error_NonHTTPWithMessage(t *testing.T) {
	err := &ProviderError{
		Provider:  "codex-cli",
		Operation: "parse",
		Message:   "invalid output",
	}

	got := err.Error()
	if got != "codex-cli parse: invalid output" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestProviderError_Unwrap(t *testing.T) {
	cause := errors.New("boom")
	err := newProviderError("anthropic", "stream", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("expected errors.Is to match cause")
	}
}

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestScanSSELines(t *testing.T) {
	input := strings.Join([]string{
		"event: message",
		"data: {\"type\":\"delta\",\"text\":\"hello\"}",
		"",
		"data: plain",
		"",
		"data:   ",
	}, "\n")
	got := make([]string, 0, 2)
	if err := scanSSELines(strings.NewReader(input), func(payload []byte) error {
		got = append(got, string(payload))
		return nil
	}); err != nil {
		t.Fatalf("scanSSELines: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 payloads, got %d (%v)", len(got), got)
	}
	if got[0] != "{\"type\":\"delta\",\"text\":\"hello\"}" {
		t.Fatalf("unexpected first payload: %q", got[0])
	}
	if got[1] != "plain" {
		t.Fatalf("unexpected second payload: %q", got[1])
	}
}

func TestScanSSELines_CallbackError(t *testing.T) {
	want := errors.New("boom")
	err := scanSSELines(strings.NewReader("data: x\n"), func([]byte) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected callback error, got %v", err)
	}
}

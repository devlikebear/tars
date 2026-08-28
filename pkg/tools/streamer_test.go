package tools

import (
	"context"
	"testing"
)

type recordingEmitter struct {
	events []emittedLine
}

type emittedLine struct {
	toolCallID string
	stream     string
	text       string
}

func (r *recordingEmitter) EmitToolLine(toolCallID, stream, text string) {
	r.events = append(r.events, emittedLine{toolCallID, stream, text})
}

func TestWithLineEmitter_RoundTrip(t *testing.T) {
	rec := &recordingEmitter{}
	ctx := WithLineEmitter(context.Background(), rec)
	got := LineEmitterFromContext(ctx)
	if got == nil {
		t.Fatalf("expected emitter from ctx, got nil")
	}
}

func TestLineEmitterFromContext_NilSafe(t *testing.T) {
	if got := LineEmitterFromContext(nil); got != nil { //nolint:staticcheck // intentional nil ctx for guard test
		t.Fatalf("expected nil for nil ctx, got %v", got)
	}
	if got := LineEmitterFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil for empty ctx, got %v", got)
	}
}

func TestWithLineEmitter_NilEmitterPassthrough(t *testing.T) {
	parent := context.Background()
	ctx := WithLineEmitter(parent, nil)
	if got := LineEmitterFromContext(ctx); got != nil {
		t.Fatalf("expected nil emitter, got %v", got)
	}
}

func TestBindLineEmitter_ForwardsLines(t *testing.T) {
	rec := &recordingEmitter{}
	streamer := BindLineEmitter(rec, "call-123")
	if streamer == nil {
		t.Fatalf("expected streamer, got nil")
	}
	streamer.EmitLine(StreamStdout, "hello")
	streamer.EmitLine(StreamStderr, "world")
	if len(rec.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(rec.events))
	}
	if got := rec.events[0]; got.toolCallID != "call-123" || got.stream != StreamStdout || got.text != "hello" {
		t.Fatalf("event 0 mismatch: %+v", got)
	}
	if got := rec.events[1]; got.toolCallID != "call-123" || got.stream != StreamStderr || got.text != "world" {
		t.Fatalf("event 1 mismatch: %+v", got)
	}
}

func TestBindLineEmitter_TrimsToolCallID(t *testing.T) {
	rec := &recordingEmitter{}
	streamer := BindLineEmitter(rec, "  call-7  ")
	streamer.EmitLine(StreamStdout, "x")
	if rec.events[0].toolCallID != "call-7" {
		t.Fatalf("expected trimmed id, got %q", rec.events[0].toolCallID)
	}
}

func TestBindLineEmitter_NilEmitterReturnsNil(t *testing.T) {
	if got := BindLineEmitter(nil, "id"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestWithToolOutputStreamer_RoundTrip(t *testing.T) {
	rec := &recordingEmitter{}
	streamer := BindLineEmitter(rec, "x")
	ctx := WithToolOutputStreamer(context.Background(), streamer)
	got := ToolOutputStreamerFromContext(ctx)
	if got == nil {
		t.Fatalf("expected streamer, got nil")
	}
	got.EmitLine(StreamStdout, "via-ctx")
	if len(rec.events) != 1 || rec.events[0].text != "via-ctx" {
		t.Fatalf("expected line forwarded, got %+v", rec.events)
	}
}

func TestToolOutputStreamerFromContext_NilSafe(t *testing.T) {
	if got := ToolOutputStreamerFromContext(nil); got != nil { //nolint:staticcheck
		t.Fatalf("expected nil for nil ctx, got %v", got)
	}
	if got := ToolOutputStreamerFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil for empty ctx, got %v", got)
	}
}

package gateway

import (
	"path/filepath"
	"testing"
)

func TestSnapshotStore_ReadWriteRoundTrip(t *testing.T) {
	store := newSnapshotStore(filepath.Join(t.TempDir(), "gateway"))
	runs := []Run{
		{ID: "run_1", Status: RunStatusCompleted, CreatedAt: "2026-02-17T10:00:00Z", UpdatedAt: "2026-02-17T10:01:00Z"},
		{ID: "run_2", Status: RunStatusCanceled, CreatedAt: "2026-02-17T10:02:00Z", UpdatedAt: "2026-02-17T10:03:00Z"},
	}
	channels := map[string][]ChannelMessage{
		"general": {
			{ID: "msg_1", ChannelID: "general", Text: "hello", Timestamp: "2026-02-17T10:00:00Z"},
			{ID: "msg_2", ChannelID: "general", Text: "world", Timestamp: "2026-02-17T10:01:00Z"},
		},
	}

	if err := store.writeRuns(runs); err != nil {
		t.Fatalf("write runs: %v", err)
	}
	if err := store.writeChannels(channels); err != nil {
		t.Fatalf("write channels: %v", err)
	}

	gotRuns, err := store.readRuns()
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	if len(gotRuns) != len(runs) {
		t.Fatalf("expected %d runs, got %d", len(runs), len(gotRuns))
	}
	if gotRuns[0].ID != "run_1" || gotRuns[1].ID != "run_2" {
		t.Fatalf("unexpected run order: %+v", gotRuns)
	}

	gotChannels, err := store.readChannels()
	if err != nil {
		t.Fatalf("read channels: %v", err)
	}
	msgs := gotChannels["general"]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 channel messages, got %d", len(msgs))
	}
	if msgs[0].ID != "msg_1" || msgs[1].ID != "msg_2" {
		t.Fatalf("unexpected channel messages: %+v", msgs)
	}
}

func TestSnapshotStore_ReadMissingReturnsEmpty(t *testing.T) {
	store := newSnapshotStore(filepath.Join(t.TempDir(), "gateway"))

	runs, err := store.readRuns()
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected empty runs, got %d", len(runs))
	}

	channels, err := store.readChannels()
	if err != nil {
		t.Fatalf("read channels: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("expected empty channels, got %d", len(channels))
	}
}

func TestTrimHelpers(t *testing.T) {
	runs := []Run{
		{ID: "run_1"},
		{ID: "run_2"},
		{ID: "run_3"},
	}
	trimmedRuns := trimRuns(runs, 2)
	if len(trimmedRuns) != 2 {
		t.Fatalf("expected 2 trimmed runs, got %d", len(trimmedRuns))
	}
	if trimmedRuns[0].ID != "run_2" || trimmedRuns[1].ID != "run_3" {
		t.Fatalf("unexpected trimmed runs: %+v", trimmedRuns)
	}

	channels := map[string][]ChannelMessage{
		"a": {
			{ID: "msg_1"},
			{ID: "msg_2"},
			{ID: "msg_3"},
		},
	}
	trimmedChannels := trimChannels(channels, 2)
	msgs := trimmedChannels["a"]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 trimmed channel messages, got %d", len(msgs))
	}
	if msgs[0].ID != "msg_2" || msgs[1].ID != "msg_3" {
		t.Fatalf("unexpected trimmed channel messages: %+v", msgs)
	}
}

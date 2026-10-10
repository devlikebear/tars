package initiative

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLedgerAppendRecentAndRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "initiative", "ledger.jsonl")
	l := OpenLedger(path, 600)
	for i := range 6 {
		if err := l.Append(Entry{At: at(10, i), Decision: Decision{Intent: IntentGreet, Speak: true, Reason: "arrived"}}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotation to %s.1: %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 600 {
		t.Fatalf("current ledger size = %v err=%v", info, err)
	}
	recent, err := l.Recent(100)
	if err != nil || len(recent) == 0 {
		t.Fatalf("recent = %v err=%v", recent, err)
	}
	if last := recent[len(recent)-1]; !last.At.Equal(at(10, 5)) || last.Intent != IntentGreet {
		t.Fatalf("newest entry = %+v", last)
	}
	if two, _ := l.Recent(1); len(two) != 1 {
		t.Fatalf("Recent(1) returned %d entries", len(two))
	}
}

func TestLedgerRecentMissingFile(t *testing.T) {
	recent, err := OpenLedger(filepath.Join(t.TempDir(), "none.jsonl"), 0).Recent(10)
	if err != nil || len(recent) != 0 {
		t.Fatalf("recent = %v err=%v", recent, err)
	}
}

func TestLedgerNeverStoresUserText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l := OpenLedger(path, 0)
	if err := l.Append(Entry{At: at(10, 0), Text: TextSignals{QuietRequested: true, Probabilities: map[string]float64{"quiet_requested": 0.9}, Source: "systemone"}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, field := range []string{"recent_user", "profile", "text\":\""} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("ledger line carries %q: %s", field, raw)
		}
	}
}

func TestHistoryFromCountsTodaysSpeech(t *testing.T) {
	now := at(15, 0)
	entries := []Entry{
		{At: now.Add(-20 * time.Hour), Decision: Decision{Intent: IntentGreet, Speak: true}}, // yesterday
		{At: at(9, 0), Decision: Decision{Intent: IntentGreet, Speak: true}},
		{At: at(11, 0), Decision: Decision{Intent: IntentCheckIn, Speak: true}},
		{At: at(14, 30), Decision: Decision{Intent: IntentBodyOnly}, Body: "delivered"},
		{At: at(14, 35), Decision: Decision{Intent: IntentBodyOnly}, Body: "dropped:unknown_provider"},
		{At: at(14, 40), Decision: Decision{Intent: IntentNone}},
	}
	h := historyFrom(entries, now, seoul)
	if h.Today != 2 || !h.LastSpokeAt.Equal(at(11, 0)) || !h.LastCheckInAt.Equal(at(11, 0)) || !h.LastBodyAt.Equal(at(14, 35)) {
		t.Fatalf("history = %+v", h)
	}
}

// TestHistoryFromSupersededDoesNotCountAsFailure checks that restoring
// history from the ledger agrees with applyLocked's own live accounting
// (tars#1220 review f1): a Superseded attempt (Composed true, Delivery
// skipped — the claim was lost, or the user had already spoken) is
// neither reset to 0 nor counted as a failure on restart, exactly like
// applyLocked treats it while the runtime is actually running. Before this
// fix, historyFrom's own if/else (Delivered → 0, anything else → ++)
// still counted DeliverySkipped as a failure, so a server restart after a
// run of harmless Superseded attempts would fabricate backoff for a
// backend that was composing successfully every time.
func TestHistoryFromSupersededDoesNotCountAsFailure(t *testing.T) {
	now := at(15, 0)
	entries := []Entry{
		// One real compose failure: counts toward SpeakFailCount.
		{At: at(9, 0), Decision: Decision{Intent: IntentGreet, Speak: true}, Composed: true, Delivery: DeliveryError},
		// Two Superseded attempts after it (claim lost, then the user
		// spoke first) — Compose ran both times but the text was
		// discarded unwritten. SpeakFailCount must stay at 1, not climb
		// to 3.
		{At: at(10, 0), Decision: Decision{Intent: IntentCheckIn, Speak: true}, Composed: true, Delivery: DeliverySkipped, DeliveryReason: deliveryReasonBusy},
		{At: at(11, 0), Decision: Decision{Intent: IntentCheckIn, Speak: true}, Composed: true, Delivery: DeliverySkipped, DeliveryReason: deliveryReasonUserSpoke},
	}
	h := historyFrom(entries, now, seoul)
	if h.SpeakFailCount != 1 {
		t.Fatalf("SpeakFailCount = %d, want 1 (Superseded attempts must not count as failures)", h.SpeakFailCount)
	}
	if h.SpeakCallsToday != 3 {
		t.Fatalf("SpeakCallsToday = %d, want 3 (every Composed attempt counts, Superseded included)", h.SpeakCallsToday)
	}
}

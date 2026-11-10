package initiative

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func boolPtr(v bool) *bool { return &v }

func intentPtr(v Intent) *Intent { return &v }

func TestReadLedgerEntriesReadsRotatedFileFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	if err := os.WriteFile(path+".1", []byte(`{"id":"a","at":"2026-10-01T10:00:00Z","mode":"shadow"}`+"\nnot json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"b","at":"2026-10-02T10:00:00Z","mode":"shadow"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLedgerEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("entries = %+v", got)
	}
	none, err := ReadLedgerEntries(filepath.Join(dir, "missing.jsonl"))
	if err != nil || len(none) != 0 {
		t.Fatalf("missing ledger = %v, %v", none, err)
	}
}

func TestBuildDatasetJoinsOutcomesAndLabels(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	entries := []Entry{
		{ID: "old", At: t0.Add(-48 * time.Hour), Mode: ModeShadow, Decision: none("nothing")},
		{At: t0, Mode: ModeShadow, Decision: none("nothing"), Error: "backend said: secret words", Body: "happy"},
		{ID: "s1", At: t0.Add(time.Hour), Mode: ModeLive, Decision: speak(IntentGreet, "arrived"),
			Signals: GoSignals{JustArrived: true, Reachable: true}, Delivery: DeliveryDelivered, DeliveryReason: "delivered", Channel: "console"},
		{Kind: entryKindOutcome, Ref: "s1", At: t0.Add(90 * time.Minute), Outcome: OutcomeReplied},
		{Kind: entryKindOutcome, Ref: "s1", At: t0.Add(91 * time.Minute), Outcome: OutcomeIgnored},
		{Kind: entryKindOutcome, Ref: "unknown", At: t0.Add(2 * time.Hour), Outcome: OutcomeIgnored},
	}
	labels := map[string]DatasetLabel{"s1": {Speak: boolPtr(false)}}
	got := BuildDataset(entries, t0.Add(-time.Hour), labels)
	if len(got) != 2 {
		t.Fatalf("records = %+v", got)
	}
	if got[0].ID != "at:2026-10-01T09:00:00Z" {
		t.Fatalf("fallback id = %q", got[0].ID)
	}
	s1 := got[1]
	if s1.Outcome != OutcomeReplied || s1.OutcomeAt == nil || !s1.OutcomeAt.Equal(t0.Add(90*time.Minute)) {
		t.Fatalf("outcome = %q at %v", s1.Outcome, s1.OutcomeAt)
	}
	if s1.Label.Speak == nil || *s1.Label.Speak || s1.Channel != "console" || !s1.Signals.JustArrived {
		t.Fatalf("record = %+v", s1)
	}

	var buf bytes.Buffer
	if err := WriteDataset(&buf, got); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, banned := range []string{"secret", "happy", `"error"`, `"body"`} {
		if strings.Contains(out, banned) {
			t.Fatalf("export leaks %q: %s", banned, out)
		}
	}
	if !strings.Contains(out, `"label":{"intent":null,"speak":null,"quiet_requested":null,"user_strained":null,"special_day":null}`) {
		t.Fatalf("empty label slots missing: %s", out)
	}

	back, err := ReadDataset(strings.NewReader(out + "\n  \n"))
	if err != nil || len(back) != 2 {
		t.Fatalf("round trip = %v, %v", back, err)
	}
	merged := LabelsFrom(back)
	if len(merged) != 1 || merged["s1"].Speak == nil {
		t.Fatalf("labels = %+v", merged)
	}
	if _, err := ReadDataset(strings.NewReader("{}\n{oops\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("bad line error = %v", err)
	}
}

func TestScenariosFromDataset(t *testing.T) {
	arrived := DatasetRecord{ID: "a", Signals: GoSignals{JustArrived: true, Reachable: true}, Decision: speak(IntentGreet, "arrived")}
	corrected := arrived
	corrected.ID = "b"
	corrected.Label = DatasetLabel{Intent: intentPtr(IntentNone), Speak: boolPtr(false), QuietRequested: boolPtr(true)}
	textOnly := arrived
	textOnly.ID = "c"
	textOnly.Label = DatasetLabel{SpecialDay: boolPtr(true)}
	sameText := arrived
	sameText.ID = "d"
	sameText.Label = DatasetLabel{SpecialDay: boolPtr(false)}
	records := []DatasetRecord{arrived, corrected, textOnly, sameText, {ID: "e"}}

	labeled := ScenariosFromDataset(records, false)
	if len(labeled) != 1 || labeled[0].Name != "ledger b (corrected)" || !labeled[0].Text.QuietRequested {
		t.Fatalf("labeled = %+v", labeled)
	}
	if _, err := labeled[0].Run(); err != nil {
		t.Fatalf("corrected scenario should pass: %v", err)
	}

	all := ScenariosFromDataset(records, true)
	var names []string
	for _, s := range all {
		names = append(names, s.Name)
		if _, err := s.Run(); err != nil {
			t.Errorf("%s: %v", s.Name, err)
		}
	}
	if strings.Join(names, ",") != "ledger a,ledger b (corrected),ledger d" {
		t.Fatalf("names = %v", names)
	}
}

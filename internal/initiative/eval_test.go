package initiative

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// The fixed evaluation set (tars#1003). Every policy change — Decide's
// switch, textSignalsMatter, ResolveReplies — has to keep these files
// passing; docs/initiative-tuning.md explains how to add a scenario from an
// exported ledger.

// readEvalLines decodes each non-blank line of testdata/eval/<name> into a
// fresh T, rejecting unknown fields so a misspelled signal fails loudly
// instead of silently reading as false.
func readEvalLines[T any](t *testing.T, name string) []T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "eval", name))
	if err != nil {
		t.Fatal(err)
	}
	var out []T
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for sc.Scan() {
		line++
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var v T
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%s:%d: %v", name, line, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatalf("%s has no scenarios", name)
	}
	return out
}

func TestEvalDecide(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range readEvalLines[DecideScenario](t, "decide.jsonl") {
		if seen[s.Name] {
			t.Errorf("duplicate scenario name %q", s.Name)
		}
		seen[s.Name] = true
		if _, err := s.Run(); err != nil {
			t.Errorf("%s: %v", s.Name, err)
		}
	}
}

// TestEvalDecideCoversEveryOutcome walks every combination of the boolean
// Go and text signals and fails when Decide can reach an (intent, reason)
// pair no scenario in decide.jsonl pins. A policy change that adds a new
// intent or reason therefore has to add a scenario for it. GoSignals'
// fields are read by reflection, so a new signal joins the walk without
// touching this test.
func TestEvalDecideCoversEveryOutcome(t *testing.T) {
	pinned := map[string]bool{}
	for _, s := range readEvalLines[DecideScenario](t, "decide.jsonl") {
		if s.Want.Intent != "" && s.Want.Reason != "" {
			pinned[string(s.Want.Intent)+"/"+s.Want.Reason] = true
		}
	}
	gt := reflect.TypeOf(GoSignals{})
	var fields []int
	for i := 0; i < gt.NumField(); i++ {
		if gt.Field(i).Type.Kind() == reflect.Bool {
			fields = append(fields, i)
		}
	}
	n := len(fields) + 3
	if n > 24 {
		t.Skipf("%d boolean signals: too many to walk exhaustively", n)
	}
	reached := map[string]bool{}
	for mask := 0; mask < 1<<n; mask++ {
		var g GoSignals
		gv := reflect.ValueOf(&g).Elem()
		for bit, idx := range fields {
			gv.Field(idx).SetBool(mask&(1<<bit) != 0)
		}
		k := len(fields)
		d := Decide(g, TextSignals{
			QuietRequested: mask&(1<<k) != 0,
			UserStrained:   mask&(1<<(k+1)) != 0,
			SpecialDay:     mask&(1<<(k+2)) != 0,
		})
		reached[string(d.Intent)+"/"+d.Reason] = true
	}
	var missing []string
	for key := range reached {
		if !pinned[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("Decide can reach outcomes no scenario in testdata/eval/decide.jsonl pins: %v", missing)
	}
}

func TestEvalTextSignalsMatter(t *testing.T) {
	type scenario struct {
		Name    string    `json:"name"`
		Signals GoSignals `json:"signals"`
		Want    bool      `json:"want"`
	}
	for _, s := range readEvalLines[scenario](t, "text_signals_matter.jsonl") {
		if got := textSignalsMatter(s.Signals); got != s.Want {
			t.Errorf("%s: textSignalsMatter = %t, want %t", s.Name, got, s.Want)
		}
	}
}

func TestEvalResolveReplies(t *testing.T) {
	type pending struct {
		ID string    `json:"id"`
		At time.Time `json:"at"`
	}
	type resolution struct {
		ID      string `json:"id"`
		Outcome string `json:"outcome"`
	}
	type scenario struct {
		Name           string       `json:"name"`
		Now            time.Time    `json:"now"`
		MainUserAt     time.Time    `json:"main_user_at"`
		TextStatus     string       `json:"text_status"`
		QuietRequested bool         `json:"quiet_requested"`
		ReplyWindow    string       `json:"reply_window"`
		Pending        []pending    `json:"pending"`
		WantResolved   []resolution `json:"want_resolved"`
		WantRemaining  []string     `json:"want_remaining"`
	}
	statuses := map[string]TextReadStatus{
		"ok":          TextReadOK,
		"temporary":   TextReadTemporarilyUnavailable,
		"unavailable": TextReadUnavailable,
	}
	for _, s := range readEvalLines[scenario](t, "replies.jsonl") {
		status, ok := statuses[s.TextStatus]
		if !ok {
			t.Errorf("%s: unknown text_status %q", s.Name, s.TextStatus)
			continue
		}
		window, err := time.ParseDuration(s.ReplyWindow)
		if err != nil {
			t.Errorf("%s: reply_window: %v", s.Name, err)
			continue
		}
		var in []PendingReply
		for _, p := range s.Pending {
			in = append(in, PendingReply{EntryID: p.ID, At: p.At})
		}
		resolved, remaining := ResolveReplies(in, s.MainUserAt, status, s.QuietRequested, s.Now, window)
		var gotResolved []resolution
		for _, r := range resolved {
			gotResolved = append(gotResolved, resolution{ID: r.EntryID, Outcome: r.Outcome})
		}
		var gotRemaining []string
		for _, p := range remaining {
			gotRemaining = append(gotRemaining, p.EntryID)
		}
		if !reflect.DeepEqual(gotResolved, s.WantResolved) || !reflect.DeepEqual(gotRemaining, s.WantRemaining) {
			t.Errorf("%s: resolved %v remaining %v, want %v / %v", s.Name, gotResolved, gotRemaining, s.WantResolved, s.WantRemaining)
		}
	}
}

func TestDecideScenarioRunReportsMismatch(t *testing.T) {
	yes := true
	cases := []struct {
		name string
		s    DecideScenario
		ok   bool
	}{
		{"no expectation", DecideScenario{Name: "x"}, false},
		{"intent", DecideScenario{Want: ScenarioWant{Intent: IntentGreet}}, false},
		{"speak", DecideScenario{Want: ScenarioWant{Speak: &yes}}, false},
		{"reason", DecideScenario{Want: ScenarioWant{Reason: "arrived"}}, false},
		{"pass", DecideScenario{Want: ScenarioWant{Intent: IntentNone, Reason: "nothing"}}, true},
	}
	for _, c := range cases {
		if _, err := c.s.Run(); (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

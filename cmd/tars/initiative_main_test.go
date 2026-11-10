package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
)

func runInitiative(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &out, &errOut)
	cmd.SetArgs(append([]string{"initiative"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func writeInitiativeLedger(t *testing.T, path string, entries []initiative.Entry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInitiativeExportFromWorkspaceLedger(t *testing.T) {
	cfgPath, workspace := writeEmbedTestConfig(t, "initiative:\n  daily_cap: 4\n")
	ledger := filepath.Join(workspace, "_shared", "initiative", "ledger.jsonl")
	now := time.Now().UTC().Truncate(time.Second)
	writeInitiativeLedger(t, ledger, []initiative.Entry{
		{ID: "old", At: now.Add(-30 * 24 * time.Hour), Mode: "shadow", Decision: initiative.Decision{Intent: initiative.IntentNone, Reason: "nothing"}},
		{ID: "g1", At: now.Add(-time.Hour), Mode: "live", Decision: initiative.Decision{Intent: initiative.IntentGreet, Speak: true, Reason: "arrived"},
			Signals: initiative.GoSignals{JustArrived: true, Reachable: true}, Delivery: initiative.DeliveryDelivered, Error: "raw backend words"},
		{Kind: "outcome", Ref: "g1", At: now.Add(-30 * time.Minute), Outcome: initiative.OutcomeIgnored},
	})

	out := filepath.Join(t.TempDir(), "export.jsonl")
	_, errOut, err := runInitiative(t, "export", "--config", cfgPath, "--out", out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "exported 2 records (0 labeled)") {
		t.Fatalf("stderr = %q", errOut)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "raw backend words") || !strings.Contains(string(data), `"outcome":"ignored"`) {
		t.Fatalf("export = %s", data)
	}

	// Label the greet as one that should not have happened, then export
	// again from the ledger: the label carries over.
	slot := `"label":{"intent":null,"speak":null`
	i := strings.LastIndex(string(data), slot)
	labeled := string(data)[:i] + `"label":{"intent":"none","speak":false` + string(data)[i+len(slot):]
	labelsPath := filepath.Join(t.TempDir(), "labeled.jsonl")
	if err := os.WriteFile(labelsPath, []byte(labeled), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, errOut, err := runInitiative(t, "export", "--config", cfgPath, "--labels", labelsPath, "--since", "48h")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "exported 1 records (1 labeled)") || !strings.Contains(stdout, `"id":"g1"`) || !strings.Contains(stdout, `"speak":false`) {
		t.Fatalf("re-export = %q / %q", stdout, errOut)
	}
}

func TestInitiativeExportErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	if _, _, err := runInitiative(t, "export", "--config", missing); err == nil || !strings.Contains(err.Error(), "--ledger") {
		t.Fatalf("missing config err = %v", err)
	}
	ledger := filepath.Join(t.TempDir(), "ledger.jsonl")
	if _, _, err := runInitiative(t, "export", "--config", missing, "--ledger", ledger, "--since", "soon"); err == nil || !strings.Contains(err.Error(), "--since") {
		t.Fatalf("bad since err = %v", err)
	}
	if _, _, err := runInitiative(t, "export", "--config", missing, "--ledger", ledger, "--labels", missing); err == nil {
		t.Fatal("missing labels file should fail")
	}
	_, errOut, err := runInitiative(t, "export", "--config", missing, "--ledger", ledger)
	if err != nil || !strings.Contains(errOut, "note: config not loaded") || !strings.Contains(errOut, "exported 0 records") {
		t.Fatalf("empty ledger = %q, %v", errOut, err)
	}
	if _, _, err := runInitiative(t, "export", "--config", missing, "--ledger", ledger, "--out", filepath.Join(t.TempDir(), "no", "such", "dir.jsonl")); err == nil {
		t.Fatal("unwritable --out should fail")
	}
}

func TestInitiativeScenarios(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "labeled.jsonl")
	lines := `{"id":"a","decision":{"intent":"greet","speak":true,"reason":"arrived"},"signals":{"just_arrived":true,"reachable":true},"text":{},"label":{"intent":"none","speak":false}}
{"id":"b","decision":{"intent":"greet","speak":true,"reason":"arrived"},"signals":{"just_arrived":true,"reachable":true},"text":{},"label":{}}
`
	if err := os.WriteFile(in, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, errOut, err := runInitiative(t, "scenarios", "--in", in, "--all")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(stdout, "\n") != 2 || !strings.Contains(stdout, `"name":"ledger a (corrected)"`) {
		t.Fatalf("stdout = %s", stdout)
	}
	if !strings.Contains(errOut, "current policy fails ledger a (corrected)") || !strings.Contains(errOut, "the current policy fails 1") {
		t.Fatalf("stderr = %s", errOut)
	}
	out := filepath.Join(dir, "scenarios.jsonl")
	if _, _, err := runInitiative(t, "scenarios", "--in", in, "--out", out); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(out); strings.Count(string(data), "\n") != 1 {
		t.Fatalf("labeled only = %s", data)
	}
	if _, _, err := runInitiative(t, "scenarios"); err == nil {
		t.Fatal("--in should be required")
	}
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte("{oops\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runInitiative(t, "scenarios", "--in", bad); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("bad input err = %v", err)
	}
}

func TestInitiativeCalibrate(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "labeled.jsonl")
	var b strings.Builder
	for i, p := range []float64{0.1, 0.2, 0.3, 0.35, 0.4, 0.45, 0.5, 0.8, 0.9, 0.95} {
		truth := p >= 0.4
		fmt.Fprintf(&b, `{"id":"r%d","text":{"source":"systemone","probabilities":{"quiet_requested":%v}},"label":{"quiet_requested":%t}}`+"\n", i, p, truth)
	}
	if err := os.WriteFile(in, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.yaml")
	stdout, errOut, err := runInitiative(t, "calibrate", "--config", missing, "--in", in, "--min-per-class", "3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "default thresholds") {
		t.Fatalf("stderr = %s", errOut)
	}
	for _, want := range []string{"quiet_requested: 10 labeled readings (6 true)", "current   0.550", "candidate 0.375", "to apply: initiative.thresholds.quiet_requested: 0.375", "user_strained: 0 labeled", "candidate none"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, stdout)
		}
	}
	stdout, _, err = runInitiative(t, "calibrate", "--config", missing, "--in", in, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var reports []initiative.ThresholdReport
	if err := json.Unmarshal([]byte(stdout), &reports); err != nil || len(reports) != 3 {
		t.Fatalf("json = %s (%v)", stdout, err)
	}
	if _, _, err := runInitiative(t, "calibrate"); err == nil {
		t.Fatal("--in should be required")
	}
	if _, _, err := runInitiative(t, "calibrate", "--in", filepath.Join(dir, "absent.jsonl")); err == nil {
		t.Fatal("missing input should fail")
	}
}

func TestInitiativeAdapt(t *testing.T) {
	dir := t.TempDir()
	ledger := filepath.Join(dir, "ledger.jsonl")
	now := time.Now().UTC()
	var entries []initiative.Entry
	for i := 0; i < 10; i++ {
		outcome := initiative.OutcomeIgnored
		if i < 2 {
			outcome = initiative.OutcomeReplied
		}
		entries = append(entries, initiative.Entry{Kind: "outcome", At: now.Add(-time.Duration(i+1) * time.Hour), Outcome: outcome})
	}
	writeInitiativeLedger(t, ledger, entries)
	cfgPath, _ := writeEmbedTestConfig(t, "initiative:\n  daily_cap: 4\n  cooldown: 30m\n  timezone: Asia/Seoul\n")
	stdout, _, err := runInitiative(t, "adapt", "--config", cfgPath, "--ledger", ledger)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2 replied, 8 ignored", "verdict: back_off (mostly_ignored)", "daily_cap: 4 -> 3", "cooldown:  30m0s -> 1h0m0s"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, stdout)
		}
	}
	stdout, _, err = runInitiative(t, "adapt", "--config", cfgPath, "--ledger", ledger, "--json", "--min-outcomes", "50")
	if err != nil {
		t.Fatal(err)
	}
	var s initiative.PacingSuggestion
	if err := json.Unmarshal([]byte(stdout), &s); err != nil || s.Verdict != initiative.PacingInsufficient {
		t.Fatalf("json = %s (%v)", stdout, err)
	}
	if _, _, err := runInitiative(t, "adapt", "--config", filepath.Join(dir, "nope.yaml")); err == nil {
		t.Fatal("adapt without config or ledger should fail")
	}
}

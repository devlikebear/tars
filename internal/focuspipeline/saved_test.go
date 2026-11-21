package focuspipeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func finishedPipeline(t *testing.T) Pipeline {
	t.Helper()
	tpl, _ := FindTemplate("", WebAppTemplateID)
	p := NewFromTemplate("s", "A sign-up page", tpl, t0)
	for i := range p.Stages {
		p.Stages[i].Status = StatusDone
	}
	p.Plan = &Plan{Goal: "A sign-up page for the book club\nwith two fields", Stages: []StageID{StagePlan, "make", "review"}}
	return p
}

func TestSaveDue(t *testing.T) {
	p := finishedPipeline(t)
	if SaveDue(p) {
		t.Fatal("due without asking")
	}
	p.SaveVersion = true
	if !SaveDue(p) {
		t.Fatal("not due when finished")
	}
	stopped := p
	stopped.Stages = append([]Stage(nil), p.Stages...)
	stopped.Stages[2].Status = StatusBlocked
	if SaveDue(stopped) {
		t.Fatal("due for a stopped pipeline")
	}
	saved := RecordSaved(p, SavedVersion{Commit: "abc"}, t0)
	if SaveDue(saved) || saved.Saved.Commit != "abc" || !saved.Saved.At.Equal(t0) || p.Saved != nil {
		t.Fatalf("recorded: %+v (original %+v)", saved.Saved, p.Saved)
	}
	if again := RecordSaved(saved, SavedVersion{Error: SaveFailed}, t0.Add(time.Hour)); again.Saved.Commit != "abc" {
		t.Fatalf("recorded twice: %+v", again.Saved)
	}
	if clone := saved.clone(); clone.Saved == saved.Saved {
		t.Fatal("clone shares Saved")
	}
}

func TestSaveMessage(t *testing.T) {
	p := finishedPipeline(t)
	if got := SaveMessage(p); got != "A sign-up page for the book club" {
		t.Fatalf("from the plan: %q", got)
	}
	p.Plan = nil
	if got := SaveMessage(p); got != "A sign-up page" {
		t.Fatalf("from the goal: %q", got)
	}
	p.Goal = strings.Repeat("가", 100)
	if got := []rune(SaveMessage(p)); len(got) != saveMessageRunes || got[len(got)-1] != '…' {
		t.Fatalf("long: %q", string(got))
	}
	p.Goal = "  "
	if got := SaveMessage(p); got != "Save the finished work" {
		t.Fatalf("empty: %q", got)
	}
}

func TestSaveVersionIsNotStoredUnlessAsked(t *testing.T) {
	raw, err := json.Marshal(finishedPipeline(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "save_version") || strings.Contains(string(raw), `"saved"`) {
		t.Fatalf("stored:\n%s", raw)
	}
}

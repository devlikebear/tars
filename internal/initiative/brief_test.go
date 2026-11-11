package initiative

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func briefPrev() StatusSnapshot {
	return StatusSnapshot{
		At: at(9, 0),
		Pipelines: map[string]PipelinePoint{
			"p-run":   {Title: "Fix login", Stage: "build"},
			"p-wait":  {Title: "Docs", Stage: "review", Gate: "triage", Waiting: true},
			"p-done":  {Title: "Release", Stage: "merge"},
			"p-still": {Title: "Same", Stage: "build"},
		},
		Cron: map[string]CronPoint{
			"c-ok":   {Name: "backup"},
			"c-fail": {Name: "sync", Failed: true},
		},
	}
}

func TestBriefNoSnapshotOrNoChangeIsEmpty(t *testing.T) {
	prev := briefPrev()
	if got := Brief(StatusSnapshot{}, prev, "en"); got != nil {
		t.Fatalf("no snapshot: %+v", got)
	}
	now := prev
	now.At = at(12, 0)
	if got := Brief(prev, now, "en"); got != nil {
		t.Fatalf("no change: %+v", got)
	}
}

func TestBriefEachKind(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*StatusSnapshot)
		want   BriefItem
		wantKo string
	}{
		{"new gate", func(s *StatusSnapshot) {
			s.Pipelines["p-run"] = PipelinePoint{Title: "Fix login", Stage: "review", Gate: "triage", Waiting: true}
		}, BriefItem{BriefWaiting, BriefBlock, `"Fix login" is waiting at G2 (review)`, "/console/focus/p-run"}, `"Fix login" — G2(리뷰)에서 기다려요`},
		{"waiting without gate", func(s *StatusSnapshot) {
			s.Pipelines["p-run"] = PipelinePoint{Title: "Fix login", Stage: "build", Waiting: true}
		}, BriefItem{BriefWaiting, BriefBlock, `"Fix login" needs your input`, "/console/focus/p-run"}, `"Fix login" — 입력을 기다려요`},
		{"waiting at another gate", func(s *StatusSnapshot) {
			s.Pipelines["p-wait"] = PipelinePoint{Title: "Docs", Stage: "pr", Gate: "pr", Waiting: true}
		}, BriefItem{BriefWaiting, BriefBlock, `"Docs" is waiting at G3 (PR)`, "/console/focus/p-wait"}, `"Docs" — G3(PR)에서 기다려요`},
		{"new pipeline already waiting", func(s *StatusSnapshot) {
			s.Pipelines["p-new"] = PipelinePoint{Title: "New", Stage: "plan", Gate: "plan", Waiting: true}
		}, BriefItem{BriefWaiting, BriefBlock, `"New" is waiting at G1 (plan)`, "/console/focus/p-new"}, `"New" — G1(계획)에서 기다려요`},
		{"finished", func(s *StatusSnapshot) {
			s.Pipelines["p-done"] = PipelinePoint{Title: "Release", Stage: "merge", Finished: true}
		}, BriefItem{BriefFinished, BriefInfo, `"Release" finished`, "/console/focus/p-done"}, `"Release" — 끝났어요`},
		{"advanced", func(s *StatusSnapshot) {
			s.Pipelines["p-run"] = PipelinePoint{Title: "Fix login", Stage: "review"}
		}, BriefItem{BriefAdvanced, BriefInfo, `"Fix login" moved to review`, "/console/focus/p-run"}, `"Fix login" — review 단계로 넘어갔어요`},
		{"cron failed", func(s *StatusSnapshot) {
			s.Cron["c-ok"] = CronPoint{Name: "backup", Failed: true}
		}, BriefItem{BriefFailed, BriefBlock, `Cron job "backup" failed`, "/console/cron"}, `cron 작업 "backup" 실패`},
		{"cron recovered", func(s *StatusSnapshot) {
			s.Cron["c-fail"] = CronPoint{Name: "sync"}
		}, BriefItem{BriefRecovered, BriefInfo, `Cron job "sync" works again`, "/console/cron"}, `cron 작업 "sync" 다시 정상`},
		{"one chat approval", func(s *StatusSnapshot) {
			s.ApprovalSessions = []string{"sess-1"}
		}, BriefItem{BriefWaiting, BriefBlock, "1 approval is waiting for you", "/console/chat/sess-1"}, "승인 1건이 기다려요"},
		{"queued approvals", func(s *StatusSnapshot) {
			s.ApprovalSessions = []string{"sess-1"}
			s.QueuedApprovals = 1
		}, BriefItem{BriefWaiting, BriefBlock, "2 approvals are waiting for you", "/console/ops"}, "승인 2건이 기다려요"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := briefPrev()
			now := cloneSnapshot(prev)
			now.At = at(12, 0)
			tc.mutate(&now)
			got := Brief(prev, now, "en")
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("Brief = %+v, want [%+v]", got, tc.want)
			}
			ko := Brief(prev, now, "ko-KR")
			if len(ko) != 1 || ko[0].Text != tc.wantKo || ko[0].Href != tc.want.Href || ko[0].Kind != tc.want.Kind {
				t.Fatalf("Brief ko = %+v, want text %q", ko, tc.wantKo)
			}
		})
	}
}

// Missing on either side says nothing: a deleted pipeline did not finish,
// a job gone from the list did not recover, a new running pipeline is not
// news, and a failing job that keeps failing is not new.
func TestBriefIgnoresAppearingAndDisappearing(t *testing.T) {
	prev := briefPrev()
	now := cloneSnapshot(prev)
	now.At = at(12, 0)
	delete(now.Pipelines, "p-done")
	delete(now.Cron, "c-fail")
	now.Pipelines["p-new"] = PipelinePoint{Title: "New", Stage: "build"}
	now.Pipelines["p-new-done"] = PipelinePoint{Title: "Old news", Finished: true}
	if got := Brief(prev, now, "en"); got != nil {
		t.Fatalf("Brief = %+v, want nothing", got)
	}
}

func TestBriefOrdersBlockFirstAndKeepsThree(t *testing.T) {
	prev := briefPrev()
	now := cloneSnapshot(prev)
	now.At = at(12, 0)
	now.Pipelines["p-done"] = PipelinePoint{Title: "Release", Stage: "merge", Finished: true}                  // finished, info
	now.Pipelines["p-still"] = PipelinePoint{Title: "Same", Stage: "review"}                                   // advanced, info
	now.Cron["c-ok"] = CronPoint{Name: "backup", Failed: true}                                                 // failed, block
	now.Pipelines["p-run"] = PipelinePoint{Title: "Fix login", Stage: "review", Gate: "triage", Waiting: true} // waiting, block
	got := Brief(prev, now, "en")
	var kinds []string
	for _, it := range got {
		kinds = append(kinds, it.Kind)
	}
	if want := []string{BriefWaiting, BriefFailed, BriefFinished}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
}

func TestBriefCleansTitles(t *testing.T) {
	prev := StatusSnapshot{At: at(9, 0), Pipelines: map[string]PipelinePoint{"p": {Title: "a", Stage: "build"}}}
	long := strings.Repeat("가", 80)
	now := StatusSnapshot{At: at(10, 0), Pipelines: map[string]PipelinePoint{"p": {Title: "  say \"hi\"\n" + long, Stage: "build", Waiting: true, Gate: "blocked"}}}
	got := Brief(prev, now, "en")
	if len(got) != 1 || strings.Contains(got[0].Text, "\n") || strings.Count(got[0].Text, `"`) != 2 || !strings.Contains(got[0].Text, "…") || !strings.HasSuffix(got[0].Text, "a blocked step") {
		t.Fatalf("Brief = %+v", got)
	}
	now.Pipelines["p"] = PipelinePoint{Stage: "build", Waiting: true}
	if got := Brief(prev, now, "ko"); len(got) != 1 || got[0].Text != `"제목 없음" — 입력을 기다려요` {
		t.Fatalf("Brief ko untitled = %+v", got)
	}
	if got := Brief(prev, now, "en"); len(got) != 1 || got[0].Text != `"untitled" needs your input` {
		t.Fatalf("Brief en untitled = %+v", got)
	}
}

func cloneSnapshot(s StatusSnapshot) StatusSnapshot {
	out := s
	out.Pipelines = map[string]PipelinePoint{}
	for k, v := range s.Pipelines {
		out.Pipelines[k] = v
	}
	out.Cron = map[string]CronPoint{}
	for k, v := range s.Cron {
		out.Cron[k] = v
	}
	out.ApprovalSessions = append([]string(nil), s.ApprovalSessions...)
	return out
}

func TestBriefStoreFreezesTheLastSnapshotAtArrival(t *testing.T) {
	path := t.TempDir() + "/snapshot.json"
	state := briefPrev()
	captures := 0
	capture := func(time.Time) StatusSnapshot {
		captures++
		return cloneSnapshot(state)
	}
	store := OpenBriefStore(path, capture)

	t0 := at(9, 0)
	if en, ko := store.Lines(t0); en != nil || ko != nil {
		t.Fatalf("lines before any presence = %v %v", en, ko)
	}
	p := RecordPresence(PresenceState{}, t0)
	if err := store.Seen(p, t0); err != nil {
		t.Fatal(err)
	}
	// Within a minute of the last capture, a continuing signal takes none.
	p = RecordPresence(p, t0.Add(30*time.Second))
	_ = store.Seen(p, t0.Add(30*time.Second))
	if captures != 1 {
		t.Fatalf("captures = %d, want 1 (throttled)", captures)
	}
	// The first arrival had nothing before it: no brief.
	if en, _ := store.Lines(t0.Add(time.Minute)); en != nil {
		t.Fatalf("first arrival lines = %v", en)
	}
	p = RecordPresence(p, t0.Add(2*time.Minute))
	_ = store.Seen(p, t0.Add(2*time.Minute)) // last seen: everything as in briefPrev

	// While away, a pipeline reaches a gate.
	state.Pipelines["p-run"] = PipelinePoint{Title: "Fix login", Stage: "review", Gate: "triage", Waiting: true}
	back := t0.Add(5 * time.Hour)
	p = RecordPresence(p, back)
	if err := store.Seen(p, back); err != nil {
		t.Fatal(err)
	}

	// A restart in between keeps the frozen snapshot.
	reopened := OpenBriefStore(path, capture)
	en, ko := reopened.Lines(back.Add(time.Minute))
	if len(en) != 1 || en[0].Kind != BriefWaiting || len(ko) != 1 || ko[0].Href != en[0].Href || ko[0].Text == en[0].Text {
		t.Fatalf("lines = %+v / %+v", en, ko)
	}
	// Further signals in the same arrival keep the frozen one.
	p = RecordPresence(p, back.Add(3*time.Minute))
	_ = reopened.Seen(p, back.Add(3*time.Minute))
	if en, _ := reopened.Lines(back.Add(4 * time.Minute)); len(en) != 1 {
		t.Fatalf("lines after more signals = %+v", en)
	}

	var nilStore *BriefStore
	if err := nilStore.Seen(p, back); err != nil {
		t.Fatal(err)
	}
	if en, ko := nilStore.Lines(back); en != nil || ko != nil {
		t.Fatal("nil store gave lines")
	}
}

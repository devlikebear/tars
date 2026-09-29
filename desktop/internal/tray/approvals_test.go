package tray

import (
	"testing"

	"github.com/devlikebear/tars/desktop/internal/activity"
)

func ids(list []activity.Approval) map[string]bool {
	out := map[string]bool{}
	for _, a := range list {
		out[a.RequestID] = true
	}
	return out
}

func TestApprovals(t *testing.T) {
	var a Approvals
	r1 := activity.Approval{RequestID: "r1", SessionID: "s1"}
	r2 := activity.Approval{RequestID: "r2", SessionID: "s2"}

	added, removed := a.Update([]activity.Approval{r1, {SessionID: "no id"}})
	if len(added) != 1 || added[0].RequestID != "r1" || len(removed) != 0 {
		t.Fatalf("first poll: +%v -%v", added, removed)
	}
	added, removed = a.Update([]activity.Approval{r1, r2})
	if len(added) != 1 || added[0].RequestID != "r2" || len(removed) != 0 {
		t.Fatalf("second poll notifies only the new one: +%v -%v", added, removed)
	}
	if got, ok := a.Lookup("r2"); !ok || got.SessionID != "s2" {
		t.Fatalf("lookup = %+v, %v", got, ok)
	}
	added, removed = a.Update([]activity.Approval{r2})
	if len(added) != 0 || len(removed) != 1 || removed[0].RequestID != "r1" {
		t.Fatalf("answered elsewhere: +%v -%v", added, removed)
	}
	if _, ok := a.Lookup("r1"); ok {
		t.Fatal("a removed approval must not be found")
	}
	a.Update([]activity.Approval{r1, r2})
	if gone := a.Forget(); !ids(gone)["r1"] || !ids(gone)["r2"] || len(gone) != 2 {
		t.Fatalf("forget = %v", gone)
	}
	if added, _ := a.Update([]activity.Approval{r1}); len(added) != 1 {
		t.Fatal("after forgetting, a question is new again")
	}
}

func TestNotificationText(t *testing.T) {
	title, subtitle, body := NotificationText(activity.Approval{SessionID: "s1", ToolName: "Bash", Preview: "rm -rf build", Reason: "deletes files"})
	if title != "Approval needed" || subtitle != "s1" || body != "Bash: rm -rf build\ndeletes files" {
		t.Fatalf("%q %q %q", title, subtitle, body)
	}
	_, subtitle, body = NotificationText(activity.Approval{SessionID: "s1", Session: "Deploy", ToolName: "Write"})
	if subtitle != "Deploy" || body != "Write" {
		t.Fatalf("%q %q", subtitle, body)
	}
}

func TestDecisionFor(t *testing.T) {
	for _, id := range []string{ActionAllowOnce, ActionAllowSession, ActionDeny} {
		if d, ok := DecisionFor(id); !ok || d != id {
			t.Errorf("DecisionFor(%q) = %q, %v", id, d, ok)
		}
	}
	if _, ok := DecisionFor("DEFAULT_ACTION"); ok {
		t.Fatal("a body click is not a decision")
	}
}

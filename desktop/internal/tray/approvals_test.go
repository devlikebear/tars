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

func TestQueuedApprovals(t *testing.T) {
	var q QueuedApprovals
	a1 := activity.QueuedApproval{ApprovalID: "apv_1", SessionID: "s1"}
	a2 := activity.QueuedApproval{ApprovalID: "apv_2", SessionID: "s2"}
	added, removed := q.Update([]activity.QueuedApproval{a1, {SessionID: "no id"}})
	if len(added) != 1 || added[0].ApprovalID != "apv_1" || len(removed) != 0 {
		t.Fatalf("first poll: +%v -%v", added, removed)
	}
	added, removed = q.Update([]activity.QueuedApproval{a1, a2})
	if len(added) != 1 || added[0].ApprovalID != "apv_2" || len(removed) != 0 {
		t.Fatalf("second poll notifies only the new one: +%v -%v", added, removed)
	}
	if got, ok := q.Lookup("apv_2"); !ok || got.SessionID != "s2" {
		t.Fatalf("lookup = %+v, %v", got, ok)
	}
	added, removed = q.Update([]activity.QueuedApproval{a2})
	if len(added) != 0 || len(removed) != 1 || removed[0].ApprovalID != "apv_1" {
		t.Fatalf("reviewed elsewhere: +%v -%v", added, removed)
	}
	if gone := q.Forget(); len(gone) != 1 || gone[0].ApprovalID != "apv_2" {
		t.Fatalf("forget = %v", gone)
	}
	if _, ok := q.Lookup("apv_2"); ok {
		t.Fatal("a forgotten approval must not be found")
	}
}

func TestQueuedNotificationText(t *testing.T) {
	title, subtitle, body := QueuedNotificationText(activity.QueuedApproval{SessionID: "s1", Session: "Nightly", Source: "cron", ToolName: "exec", Preview: "make", Reason: "runs a command"})
	if title != "Needs input: cron run" || subtitle != "Nightly" || body != "exec: make\nruns a command" {
		t.Fatalf("%q %q %q", title, subtitle, body)
	}
	title, subtitle, body = QueuedNotificationText(activity.QueuedApproval{SessionID: "s1", ToolName: "exec"})
	if title != "Needs input" || subtitle != "s1" || body != "exec" {
		t.Fatalf("%q %q %q", title, subtitle, body)
	}
}

func TestReviewFor(t *testing.T) {
	if d, ok := ReviewFor(ActionApprove); !ok || d != ReviewApprove {
		t.Fatalf("approve = %q, %v", d, ok)
	}
	if d, ok := ReviewFor(ActionReject); !ok || d != ReviewReject {
		t.Fatalf("reject = %q, %v", d, ok)
	}
	for _, id := range []string{"DEFAULT_ACTION", ActionAllowOnce, ActionDeny} {
		if _, ok := ReviewFor(id); ok {
			t.Fatalf("%q is not a review", id)
		}
	}
	// The two categories' actions never overlap, so a chat decision can
	// never be read as a review or the other way round.
	for _, id := range []string{ActionApprove, ActionReject} {
		if _, ok := DecisionFor(id); ok {
			t.Fatalf("%q must not be a chat decision", id)
		}
	}
}

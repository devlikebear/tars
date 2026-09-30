package tray

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/desktop/internal/activity"
)

func TestStatusOf(t *testing.T) {
	busy := activity.Snapshot{
		Running: []activity.Running{{SessionID: "s1"}, {SessionID: "s2"}},
		Pending: []activity.Approval{{RequestID: "r1", SessionID: "s1"}},
	}
	cases := []struct {
		name  string
		obs   Observation
		state State
		label string
		tip   string
	}{
		{"offline", Observation{Snapshot: busy}, Offline, "off", "server not running"},
		{"setup", Observation{Reachable: true, NeedsSetup: true, Unauthorized: true}, Setup, "", "finish setup"},
		{"locked", Observation{Reachable: true, Unauthorized: true}, Locked, "", "TARS_API_TOKEN"},
		{"needs input", Observation{Reachable: true, Snapshot: busy}, NeedsInput, "1?", "1 request waiting"},
		{"running", Observation{Reachable: true, Snapshot: activity.Snapshot{Running: busy.Running}}, Running, "2…", "2 chats running"},
		{"idle", Observation{Reachable: true}, Idle, "", "idle"},
	}
	for _, c := range cases {
		st := StatusOf(c.obs)
		if st.State != c.state || st.Label() != c.label || !strings.Contains(st.Tooltip(), c.tip) {
			t.Errorf("%s: state=%v label=%q tooltip=%q", c.name, st.State, st.Label(), st.Tooltip())
		}
		if st.State.String() == "unknown" {
			t.Errorf("%s: state has no name", c.name)
		}
	}
	if State(99).String() != "unknown" {
		t.Fatal("an out-of-range state must read unknown")
	}
	if got := StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Pending: make([]activity.Approval, 3)}}).Tooltip(); !strings.Contains(got, "3 requests") {
		t.Fatalf("plural tooltip = %q", got)
	}
	if got := StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Running: make([]activity.Running, 1)}}).Tooltip(); !strings.Contains(got, "1 chat running") {
		t.Fatalf("singular tooltip = %q", got)
	}
}

func find(items []Item, label string) *Item {
	for i := range items {
		if strings.Contains(items[i].Label, label) {
			return &items[i]
		}
		if hit := find(items[i].Children, label); hit != nil {
			return hit
		}
	}
	return nil
}

func TestOfflineMenuOffersStart(t *testing.T) {
	items := Menu(StatusOf(Observation{}), []activity.Session{{ID: "x", Title: "ignored"}})
	if find(items, "Start server") == nil || find(items, "Recent chats") != nil || find(items, "New chat") != nil {
		t.Fatalf("offline menu = %+v", items)
	}
	if items[0].Action.Kind != NoAction || !items[0].Disabled || !items[1].Separator {
		t.Fatal("the menu opens with the disabled status line")
	}
	if q := find(items, "Quit"); q == nil || q.Action.Kind != Quit {
		t.Fatal("offline menu must still quit")
	}
}

func TestBusyMenu(t *testing.T) {
	approval := activity.Approval{RequestID: "r1", SessionID: "s1", Session: "Build", ToolName: "Bash", Preview: "make release", Decisions: []string{"allow_once", "allow_session", "deny"}}
	st := StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{
		Running: []activity.Running{{SessionID: "s1", Session: "Build"}, {SessionID: "s 2"}},
		Pending: []activity.Approval{approval, {RequestID: "r2", SessionID: "s3", ToolName: "Write"}},
	}})
	items := Menu(st, []activity.Session{{ID: "a", Title: "Alpha"}, {ID: "b"}})

	entry := find(items, "Build — Bash: make release")
	if entry == nil || len(entry.Children) != 5 {
		t.Fatalf("approval entry = %+v", entry)
	}
	if open := entry.Children[0]; open.Action.Kind != ShowConsole || open.Action.Path != "/console/chat/s1" {
		t.Fatalf("open = %+v", open)
	}
	allow := entry.Children[3]
	if allow.Label != "Allow for this session" || allow.Action.Kind != Decide || allow.Action.Decision != "allow_session" || allow.Action.Approval.RequestID != "r1" {
		t.Fatalf("allow = %+v", allow)
	}
	// An approval without decisions still offers allow once and deny.
	if other := find(items, "s3 — Write"); other == nil || len(other.Children) != 4 || other.Children[3].Label != "Deny" {
		t.Fatalf("default decisions = %+v", other)
	}
	if r := find(items, "  s 2"); r == nil || r.Action.Path != "/console/chat/s%202" {
		t.Fatalf("running entry = %+v", r)
	}
	checkRecentChats(t, items)
	if n := find(items, "New chat in folder"); n == nil || n.Disabled {
		t.Fatal("new chat must be enabled on a running server")
	}
}

// checkRecentChats checks the Recent chats submenu of a menu listing the
// chats "Alpha" (a) and b.
func checkRecentChats(t *testing.T, items []Item) {
	t.Helper()
	recent := find(items, "Recent chats")
	if recent == nil || len(recent.Children) != 4 || recent.Children[1].Label != "b" || recent.Children[0].Action.Path != "/console/chat/a" || !recent.Children[2].Separator {
		t.Fatalf("recent = %+v", recent)
	}
	windows := recent.Children[3]
	if windows.Label != "Open in new window" || len(windows.Children) != 2 {
		t.Fatalf("new window submenu = %+v", windows)
	}
	if w := windows.Children[0]; w.Label != "Alpha" || w.Action.Kind != OpenSessionWindow || w.Action.SessionID != "a" || w.Action.Path != "/console/chat/a" {
		t.Fatalf("new window entry = %+v", w)
	}
}

func TestSetupMenuDisablesNewChat(t *testing.T) {
	items := Menu(StatusOf(Observation{Reachable: true, NeedsSetup: true}), nil)
	if n := find(items, "New chat in folder"); n == nil || !n.Disabled {
		t.Fatal("new chat needs a configured server")
	}
}

func TestHelpers(t *testing.T) {
	if got := truncate(strings.Repeat("가", 70), 60); len([]rune(got)) != 60 || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate = %q", got)
	}
	if DecisionLabel("allow_always") != "Always allow in this folder" {
		t.Fatalf("allow_always label = %q", DecisionLabel("allow_always"))
	}
	if DecisionLabel("custom") != "custom" || DecisionLabel("deny") != "Deny" || DecisionLabel("allow_once") != "Allow once" {
		t.Fatal("decision labels")
	}
	if describe("  ") != "" {
		t.Fatal("an empty preview adds nothing")
	}
}

func TestChatWindowTitle(t *testing.T) {
	sessions := []activity.Session{{ID: "a", Title: "Alpha"}, {ID: "b", Title: " "}}
	for id, want := range map[string]string{"a": "TARS — Alpha", "b": "TARS — b", "c": "TARS — c"} {
		if got := ChatWindowTitle(sessions, id); got != want {
			t.Errorf("ChatWindowTitle(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestKey(t *testing.T) {
	idle := Menu(StatusOf(Observation{Reachable: true}), []activity.Session{{ID: "a", Title: "Alpha"}})
	if Key(idle) != Key(Menu(StatusOf(Observation{Reachable: true}), []activity.Session{{ID: "a", Title: "Alpha"}})) {
		t.Fatal("the same menu must have the same key")
	}
	if Key(idle) == Key(Menu(StatusOf(Observation{Reachable: true}), []activity.Session{{ID: "a", Title: "Alpha 2"}})) {
		t.Fatal("a renamed chat in a submenu must change the key")
	}
	busy := Menu(StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Pending: []activity.Approval{{RequestID: "r1", SessionID: "s"}}}}), nil)
	other := Menu(StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Pending: []activity.Approval{{RequestID: "r2", SessionID: "s"}}}}), nil)
	if Key(busy) == Key(other) {
		t.Fatal("a different pending request must change the key")
	}
}

// #1033: unattended runs' questions wait in the ops queue. The tray counts
// them as needs input and lists them with approve and reject, which review
// through the ops approvals and never decide a chat permission.
func TestQueuedApprovalsNeedInput(t *testing.T) {
	queued := activity.QueuedApproval{ApprovalID: "apv_1", SessionID: "s2", Session: "Nightly", Source: "cron", ToolName: "write_file", Preview: "notes.md"}
	st := StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{
		Running: []activity.Running{{SessionID: "s1"}},
		Pending: []activity.Approval{{RequestID: "r1", SessionID: "s1", ToolName: "Bash"}},
		Queued:  []activity.QueuedApproval{queued, {ApprovalID: "apv_2", SessionID: "s3", ToolName: "exec"}},
	}})
	if st.State != NeedsInput || st.Pending != 1 || st.Queued != 2 || st.Waiting() != 3 {
		t.Fatalf("status = %+v", st)
	}
	if st.Label() != "3?" || !strings.Contains(st.Tooltip(), "3 requests waiting") {
		t.Fatalf("label %q tooltip %q", st.Label(), st.Tooltip())
	}
	onlyQueued := StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Queued: []activity.QueuedApproval{queued}}})
	if onlyQueued.State != NeedsInput || onlyQueued.Label() != "1?" {
		t.Fatalf("an unattended question alone needs input: %+v", onlyQueued)
	}

	checkQueuedMenu(t, Menu(onlyQueued, nil))
	checkQueuedMenuKeys(t, st)
}

// checkQueuedMenu checks the entry of apv_1 (Nightly, cron, write_file on
// notes.md) in a menu that lists it alone.
func checkQueuedMenu(t *testing.T, items []Item) {
	t.Helper()
	if find(items, "Waiting for approval") == nil {
		t.Fatal("queued approvals are listed under waiting for approval")
	}
	entry := find(items, "Nightly — write_file: notes.md (cron)")
	if entry == nil || len(entry.Children) != 4 {
		t.Fatalf("queued entry = %+v", entry)
	}
	if open := entry.Children[0]; open.Action.Kind != ShowConsole || open.Action.Path != "/console/chat/s2" {
		t.Fatalf("open = %+v", open)
	}
	approve, reject := entry.Children[2], entry.Children[3]
	if approve.Label != "Approve" || approve.Action.Kind != Review || approve.Action.Decision != ReviewApprove || approve.Action.Queued.ApprovalID != "apv_1" {
		t.Fatalf("approve = %+v", approve)
	}
	if reject.Label != "Reject" || reject.Action.Kind != Review || reject.Action.Decision != ReviewReject {
		t.Fatalf("reject = %+v", reject)
	}
}

// checkQueuedMenuKeys checks that the full menu of st never turns a queued
// approval into a chat decision, and that the menu key follows the queue.
func checkQueuedMenuKeys(t *testing.T, st Status) {
	t.Helper()
	assertNoQueuedDecision(t, Menu(st, nil))
	if other := find(Menu(st, nil), "s3 — exec"); other == nil || strings.Contains(other.Label, "(") {
		t.Fatalf("a queued approval without a source = %+v", other)
	}

	a := Menu(StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Queued: []activity.QueuedApproval{{ApprovalID: "apv_1", SessionID: "s"}}}}), nil)
	b := Menu(StatusOf(Observation{Reachable: true, Snapshot: activity.Snapshot{Queued: []activity.QueuedApproval{{ApprovalID: "apv_2", SessionID: "s"}}}}), nil)
	if Key(a) == Key(b) {
		t.Fatal("a different queued approval must change the key")
	}
}

// assertNoQueuedDecision fails if any item answers a queued approval as a
// chat permission.
func assertNoQueuedDecision(t *testing.T, items []Item) {
	t.Helper()
	for _, it := range items {
		if it.Action.Queued.ApprovalID != "" && it.Action.Kind == Decide {
			t.Fatalf("a queued approval must never be a chat decision: %+v", it)
		}
		assertNoQueuedDecision(t, it.Children)
	}
}

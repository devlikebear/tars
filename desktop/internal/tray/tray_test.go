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
	recent := find(items, "Recent chats")
	if recent == nil || len(recent.Children) != 2 || recent.Children[1].Label != "b" || recent.Children[0].Action.Path != "/console/chat/a" {
		t.Fatalf("recent = %+v", recent)
	}
	if n := find(items, "New chat in folder"); n == nil || n.Disabled {
		t.Fatal("new chat must be enabled on a running server")
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

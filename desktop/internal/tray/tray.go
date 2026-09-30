// Package tray decides what the tray icon says and what its menu holds. It
// has no GUI dependency: main turns the Items into native menu entries, so
// the whole model is testable without a display.
package tray

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/devlikebear/tars/desktop/internal/activity"
)

// State is the one thing the tray icon shows at a glance.
type State int

const (
	// Offline: nothing answers at the server address.
	Offline State = iota
	// Locked: the server answers but refuses the shell's token.
	Locked
	// Setup: the server runs in setup-only mode and wants onboarding.
	Setup
	// Idle: the server runs and no chat turn is in progress.
	Idle
	// Running: at least one chat turn is in progress.
	Running
	// NeedsInput: at least one turn waits for a tool approval, a chat
	// turn's or an unattended run's.
	NeedsInput
)

func (s State) String() string {
	switch s {
	case Offline:
		return "offline"
	case Locked:
		return "locked"
	case Setup:
		return "setup"
	case Idle:
		return "idle"
	case Running:
		return "running"
	case NeedsInput:
		return "needs-input"
	}
	return "unknown"
}

// Status is what the last poll of the server found.
type Status struct {
	State   State
	Running int
	// Pending counts chat turns' approvals, Queued unattended runs'
	// approvals waiting in the ops queue.
	Pending  int
	Queued   int
	Snapshot activity.Snapshot
}

// Waiting counts every question waiting for an answer.
func (s Status) Waiting() int {
	return s.Pending + s.Queued
}

// Observation is the raw result of one poll.
type Observation struct {
	// Reachable: /v1/healthz answered as TARS.
	Reachable bool
	// NeedsSetup: healthz reported setup-only mode.
	NeedsSetup bool
	// Unauthorized: the activity call was refused for the token.
	Unauthorized bool
	// Snapshot is the activity, when it could be read.
	Snapshot activity.Snapshot
}

// StatusOf turns one poll into the state the icon shows. Waiting for input
// outranks running: a person has to act.
func StatusOf(o Observation) Status {
	st := Status{Snapshot: o.Snapshot, Running: len(o.Snapshot.Running), Pending: len(o.Snapshot.Pending), Queued: len(o.Snapshot.Queued)}
	switch {
	case !o.Reachable:
		st = Status{State: Offline}
	case o.NeedsSetup:
		st = Status{State: Setup}
	case o.Unauthorized:
		st = Status{State: Locked}
	case st.Waiting() > 0:
		st.State = NeedsInput
	case st.Running > 0:
		st.State = Running
	default:
		st.State = Idle
	}
	return st
}

// Label is the short text beside the icon in the macOS menu bar; empty
// when nothing needs attention, so an idle tray takes no space.
func (s Status) Label() string {
	switch s.State {
	case NeedsInput:
		return fmt.Sprintf("%d?", s.Waiting())
	case Running:
		return fmt.Sprintf("%d…", s.Running)
	case Offline:
		return "off"
	}
	return ""
}

// Tooltip is the hover text on every platform.
func (s Status) Tooltip() string {
	switch s.State {
	case Offline:
		return "TARS — server not running"
	case Locked:
		return "TARS — server wants an API token (tars-desktop config.json or TARS_API_TOKEN)"
	case Setup:
		return "TARS — finish setup in the console"
	case NeedsInput:
		return fmt.Sprintf("TARS — %s waiting for approval", plural(s.Waiting(), "request", "requests"))
	case Running:
		return fmt.Sprintf("TARS — %s running", plural(s.Running, "chat", "chats"))
	}
	return "TARS — idle"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ActionKind is what clicking a menu entry does.
type ActionKind int

const (
	// NoAction: a heading or separator.
	NoAction ActionKind = iota
	// ShowConsole opens the window on Path.
	ShowConsole
	// OpenSessionWindow shows chat SessionID in a window of its own.
	OpenSessionWindow
	// Decide answers a chat approval with Decision.
	Decide
	// Review approves or rejects a queued unattended approval; Decision is
	// ReviewApprove or ReviewReject.
	Review
	// NewSessionInFolder asks for a folder and starts a session there.
	NewSessionInFolder
	// StartServer starts the server.
	StartServer
	// CheckUpdates asks the updater for a new release.
	CheckUpdates
	// Quit quits the shell; the server keeps running.
	Quit
)

// Action is a menu entry's click.
type Action struct {
	Kind      ActionKind
	Path      string
	SessionID string
	Approval  activity.Approval
	Queued    activity.QueuedApproval
	Decision  string
}

// The two answers a queued unattended approval takes.
const (
	ReviewApprove = "approve"
	ReviewReject  = "reject"
)

// Item is one menu entry. Separator entries have no label.
type Item struct {
	Label     string
	Separator bool
	Disabled  bool
	Action    Action
	Children  []Item
}

// Menu is the tray menu for status. sessions are the recent chats.
func Menu(status Status, sessions []activity.Session) []Item {
	items := []Item{{Label: status.Tooltip(), Disabled: true}, {Separator: true}}
	items = append(items, Item{Label: "Open TARS", Action: Action{Kind: ShowConsole, Path: "/console"}})

	if status.State == Offline {
		return append(items,
			Item{Label: "Start server", Action: Action{Kind: StartServer}},
			Item{Separator: true},
			Item{Label: "Check for updates…", Action: Action{Kind: CheckUpdates}},
			Item{Label: "Quit", Action: Action{Kind: Quit}},
		)
	}

	if len(status.Snapshot.Pending) > 0 || len(status.Snapshot.Queued) > 0 {
		items = append(items, Item{Separator: true}, Item{Label: "Waiting for approval", Disabled: true})
		for _, a := range status.Snapshot.Pending {
			items = append(items, approvalItem(a))
		}
		for _, q := range status.Snapshot.Queued {
			items = append(items, queuedItem(q))
		}
	}
	if len(status.Snapshot.Running) > 0 {
		items = append(items, Item{Separator: true}, Item{Label: "Running", Disabled: true})
		for _, r := range status.Snapshot.Running {
			items = append(items, Item{Label: "  " + titleOr(r.Session, r.SessionID), Action: chatAction(r.SessionID)})
		}
	}
	if len(sessions) > 0 {
		recent := Item{Label: "Recent chats"}
		windows := Item{Label: "Open in new window"}
		for _, s := range sessions {
			recent.Children = append(recent.Children, Item{Label: titleOr(s.Title, s.ID), Action: chatAction(s.ID)})
			windows.Children = append(windows.Children, Item{Label: titleOr(s.Title, s.ID), Action: windowAction(s.ID)})
		}
		recent.Children = append(recent.Children, Item{Separator: true}, windows)
		items = append(items, Item{Separator: true}, recent)
	}
	return append(items,
		Item{Separator: true},
		Item{Label: "New chat in folder…", Disabled: status.State == Setup || status.State == Locked, Action: Action{Kind: NewSessionInFolder}},
		Item{Label: "Check for updates…", Action: Action{Kind: CheckUpdates}},
		Item{Label: "Quit", Action: Action{Kind: Quit}},
	)
}

func approvalItem(a activity.Approval) Item {
	label := "  " + truncate(titleOr(a.Session, a.SessionID)+" — "+a.ToolName+describe(a.Preview), 60)
	item := Item{Label: label}
	item.Children = []Item{{Label: "Open chat", Action: chatAction(a.SessionID)}, {Separator: true}}
	decisions := a.Decisions
	if len(decisions) == 0 {
		decisions = []string{"allow_once", "deny"}
	}
	for _, d := range decisions {
		item.Children = append(item.Children, Item{Label: DecisionLabel(d), Action: Action{Kind: Decide, Approval: a, Decision: d}})
	}
	return item
}

// queuedItem is an unattended run's question. Its answers go to the ops
// approvals (Review), never to the chat permission endpoint.
func queuedItem(q activity.QueuedApproval) Item {
	label := "  " + truncate(titleOr(q.Session, q.SessionID)+" — "+q.ToolName+describe(q.Preview)+runOf(q.Source), 60)
	item := Item{Label: label}
	item.Children = []Item{{Label: "Open chat", Action: chatAction(q.SessionID)}, {Separator: true}}
	for _, d := range []string{ReviewApprove, ReviewReject} {
		item.Children = append(item.Children, Item{Label: ReviewLabel(d), Action: Action{Kind: Review, Queued: q, Decision: d}})
	}
	return item
}

// ReviewLabel is how a queued approval's answer reads on a button.
func ReviewLabel(decision string) string {
	if decision == ReviewApprove {
		return "Approve"
	}
	return "Reject"
}

// runOf names the kind of run that asked, e.g. " (cron)".
func runOf(source string) string {
	if source = strings.TrimSpace(source); source == "" {
		return ""
	}
	return " (" + source + ")"
}

// DecisionLabel is how a decision reads on a button.
func DecisionLabel(decision string) string {
	switch decision {
	case "allow_once":
		return "Allow once"
	case "allow_session":
		return "Allow for this session"
	case "allow_always":
		return "Always allow in this folder"
	case "deny":
		return "Deny"
	}
	return decision
}

func chatAction(sessionID string) Action {
	return Action{Kind: ShowConsole, Path: "/console/chat/" + url.PathEscape(sessionID)}
}

// ChatWindowTitle is the title of chat id's own window: its title when the
// recent chats know it, else its id.
func ChatWindowTitle(sessions []activity.Session, id string) string {
	for _, s := range sessions {
		if s.ID == id {
			return "TARS — " + titleOr(s.Title, id)
		}
	}
	return "TARS — " + id
}

func windowAction(sessionID string) Action {
	a := chatAction(sessionID)
	a.Kind, a.SessionID = OpenSessionWindow, sessionID
	return a
}

func describe(preview string) string {
	if preview = strings.TrimSpace(preview); preview == "" {
		return ""
	}
	return ": " + preview
}

func titleOr(title, id string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	return id
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// Key is a fingerprint of a menu, so the shell rebuilds the native menu only
// when something in it changed: rebuilding closes a menu the user has open.
func Key(items []Item) string {
	var b strings.Builder
	var walk func([]Item, int)
	walk = func(items []Item, depth int) {
		for _, it := range items {
			fmt.Fprintf(&b, "%d|%s|%v|%v|%d|%s|%s|%s|%s|%s\n", depth, it.Label, it.Separator, it.Disabled, it.Action.Kind, it.Action.Path, it.Action.SessionID, it.Action.Approval.RequestID, it.Action.Queued.ApprovalID, it.Action.Decision)
			walk(it.Children, depth+1)
		}
	}
	walk(items, 0)
	return b.String()
}

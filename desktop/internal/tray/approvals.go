package tray

import (
	"sync"

	"github.com/devlikebear/tars/desktop/internal/activity"
)

// Approvals remembers which approvals the shell has already notified about,
// so each question gets one notification, and one that was answered
// elsewhere (the console, another client) has its notification withdrawn.
type Approvals struct {
	mu    sync.Mutex
	known map[string]activity.Approval
}

// Update records the pending list from a poll and returns the approvals
// that are new since the last one and those that are gone.
func (a *Approvals) Update(pending []activity.Approval) (added, removed []activity.Approval) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.known == nil {
		a.known = map[string]activity.Approval{}
	}
	now := make(map[string]bool, len(pending))
	for _, p := range pending {
		if p.RequestID == "" {
			continue
		}
		now[p.RequestID] = true
		if _, seen := a.known[p.RequestID]; !seen {
			added = append(added, p)
		}
		a.known[p.RequestID] = p
	}
	for id, p := range a.known {
		if !now[id] {
			removed = append(removed, p)
			delete(a.known, id)
		}
	}
	return added, removed
}

// Forget clears everything, for when the server goes away: its turns and
// their questions went with it.
func (a *Approvals) Forget() []activity.Approval {
	a.mu.Lock()
	defer a.mu.Unlock()
	var gone []activity.Approval
	for _, p := range a.known {
		gone = append(gone, p)
	}
	a.known = nil
	return gone
}

// Lookup finds a pending approval by request ID.
func (a *Approvals) Lookup(requestID string) (activity.Approval, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, ok := a.known[requestID]
	return p, ok
}

// Notification action IDs. The category offers the three decisions; a click
// on the notification body opens the chat.
const (
	NotificationCategory = "tars.approval"
	ActionAllowOnce      = "allow_once"
	ActionAllowSession   = "allow_session"
	ActionDeny           = "deny"
)

// NotificationText is what a notification for a says.
func NotificationText(a activity.Approval) (title, subtitle, body string) {
	title = "Approval needed"
	subtitle = titleOr(a.Session, a.SessionID)
	body = a.ToolName + describe(a.Preview)
	if a.Reason != "" {
		body += "\n" + a.Reason
	}
	return title, subtitle, body
}

// DecisionFor maps a notification action to an approval decision; ok is
// false for a click on the notification itself, which opens the chat.
func DecisionFor(actionID string) (decision string, ok bool) {
	switch actionID {
	case ActionAllowOnce, ActionAllowSession, ActionDeny:
		return actionID, true
	}
	return "", false
}

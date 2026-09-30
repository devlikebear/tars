package tray

import (
	"strings"
	"sync"

	"github.com/devlikebear/tars/desktop/internal/activity"
)

// tracker remembers which questions the shell has already notified about,
// so each question gets one notification, and one that was answered
// elsewhere (the console, another client) has its notification withdrawn.
type tracker[T any] struct {
	mu    sync.Mutex
	known map[string]T
}

func (t *tracker[T]) update(list []T, id func(T) string) (added, removed []T) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.known == nil {
		t.known = map[string]T{}
	}
	now := make(map[string]bool, len(list))
	for _, p := range list {
		key := id(p)
		if key == "" {
			continue
		}
		now[key] = true
		if _, seen := t.known[key]; !seen {
			added = append(added, p)
		}
		t.known[key] = p
	}
	for key, p := range t.known {
		if !now[key] {
			removed = append(removed, p)
			delete(t.known, key)
		}
	}
	return added, removed
}

func (t *tracker[T]) forget() []T {
	t.mu.Lock()
	defer t.mu.Unlock()
	var gone []T
	for _, p := range t.known {
		gone = append(gone, p)
	}
	t.known = nil
	return gone
}

func (t *tracker[T]) lookup(key string) (T, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.known[key]
	return p, ok
}

// Approvals tracks the chat turns' approvals by request ID.
type Approvals struct{ t tracker[activity.Approval] }

// Update records the pending list from a poll and returns the approvals
// that are new since the last one and those that are gone.
func (a *Approvals) Update(pending []activity.Approval) (added, removed []activity.Approval) {
	return a.t.update(pending, func(p activity.Approval) string { return p.RequestID })
}

// Forget clears everything, for when the server goes away: its turns and
// their questions went with it.
func (a *Approvals) Forget() []activity.Approval { return a.t.forget() }

// Lookup finds a pending approval by request ID.
func (a *Approvals) Lookup(requestID string) (activity.Approval, bool) {
	return a.t.lookup(requestID)
}

// QueuedApprovals tracks unattended runs' approvals by approval ID.
type QueuedApprovals struct {
	t tracker[activity.QueuedApproval]
}

// Update records the queued list from a poll and returns the approvals
// that are new since the last one and those that are gone.
func (q *QueuedApprovals) Update(queued []activity.QueuedApproval) (added, removed []activity.QueuedApproval) {
	return q.t.update(queued, func(p activity.QueuedApproval) string { return p.ApprovalID })
}

// Forget clears everything, for when the server goes away. The server
// expires the queue at its next start.
func (q *QueuedApprovals) Forget() []activity.QueuedApproval { return q.t.forget() }

// Lookup finds a queued approval by approval ID.
func (q *QueuedApprovals) Lookup(approvalID string) (activity.QueuedApproval, bool) {
	return q.t.lookup(approvalID)
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

// Notification action IDs for an unattended run's question. They are
// answered through the ops approvals, so the category is its own and its
// actions never map to a chat decision.
const (
	QueuedNotificationCategory = "tars.unattended"
	ActionApprove              = "ops_approve"
	ActionReject               = "ops_reject"
)

// QueuedNotificationText is what a notification for q says.
func QueuedNotificationText(q activity.QueuedApproval) (title, subtitle, body string) {
	title = "Needs input"
	if source := strings.TrimSpace(q.Source); source != "" {
		title = "Needs input: " + source + " run"
	}
	subtitle = titleOr(q.Session, q.SessionID)
	body = q.ToolName + describe(q.Preview)
	if q.Reason != "" {
		body += "\n" + q.Reason
	}
	return title, subtitle, body
}

// ReviewFor maps a notification action on a queued approval to its answer;
// ok is false for a click on the notification itself, which opens the chat.
func ReviewFor(actionID string) (decision string, ok bool) {
	switch actionID {
	case ActionApprove:
		return ReviewApprove, true
	case ActionReject:
		return ReviewReject, true
	}
	return "", false
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

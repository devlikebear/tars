package ops

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Tool permission approvals (#970). A turn nobody is watching — a cron job,
// a Telegram message, a subagent — cannot put an approval card in front of
// anyone, so when its session's permission mode says to ask, the question
// becomes an approval here. The turn waits for the review and reads the
// outcome back with GetApproval; unlike cleanup and git mutations, nothing
// is applied on approve.

const (
	ApprovalTypeToolPermission = "tool_permission"

	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
	// ApprovalStatusExpired closes a question whose turn stopped waiting:
	// nobody answered in time, the turn was cancelled, or the server
	// restarted while it waited.
	ApprovalStatusExpired = "expired"
)

// ToolPermissionRequest is what an unattended turn asks to run.
type ToolPermissionRequest struct {
	SessionID string `json:"session_id"`
	// Source names the kind of turn that asked: cron, telegram, subagent.
	Source   string `json:"source"`
	RunLabel string `json:"run_label,omitempty"`
	ToolName string `json:"tool_name"`
	// Preview is the command, path or URL the call would act on.
	Preview string `json:"preview,omitempty"`
	Reason  string `json:"reason,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// CreateToolPermissionApproval queues a pending tool permission question.
func (m *Manager) CreateToolPermissionApproval(req ToolPermissionRequest) (Approval, error) {
	if m == nil {
		return Approval{}, fmt.Errorf("ops manager is nil")
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.ToolName = strings.TrimSpace(req.ToolName)
	if req.ToolName == "" {
		return Approval{}, fmt.Errorf("tool_name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.loadApprovalsLocked()
	if err != nil {
		return Approval{}, err
	}
	now := m.nowFn().UTC()
	id := newApprovalID(now)
	for {
		if _, err := approvalIndexByID(items, id); err != nil {
			break
		}
		now = now.Add(time.Nanosecond)
		id = newApprovalID(now)
	}
	item := Approval{
		ID:             id,
		Type:           ApprovalTypeToolPermission,
		Status:         ApprovalStatusPending,
		RequestedAt:    now,
		UpdatedAt:      now,
		Plan:           CleanupPlan{Candidates: []CleanupCandidate{}},
		ToolPermission: &req,
	}
	items = append(items, item)
	if err := m.saveApprovalsLocked(items); err != nil {
		return Approval{}, err
	}
	_ = m.appendEventLocked("tool_permission_requested", map[string]any{
		"approval_id": id,
		"session_id":  req.SessionID,
		"tool_name":   req.ToolName,
		"source":      req.Source,
	})
	return item, nil
}

// ReviewToolPermission approves or rejects a pending tool permission
// question. A question that is no longer pending cannot be reviewed: its
// turn has already moved on.
func (m *Manager) ReviewToolPermission(approvalID string, approve bool) error {
	next := ApprovalStatusRejected
	if approve {
		next = ApprovalStatusApproved
	}
	return m.closeToolPermission(approvalID, next)
}

// ExpireToolPermission closes a pending question its turn stopped waiting
// for. Closing one already reviewed is not an error.
func (m *Manager) ExpireToolPermission(approvalID string) error {
	err := m.closeToolPermission(approvalID, ApprovalStatusExpired)
	if errors.Is(err, errApprovalNotPending) {
		return nil
	}
	return err
}

// ExpirePendingToolPermissions closes every pending question. The server
// calls it at startup: the turns that asked them did not survive the
// restart. It returns how many it closed.
func (m *Manager) ExpirePendingToolPermissions() (int, error) {
	if m == nil {
		return 0, fmt.Errorf("ops manager is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.loadApprovalsLocked()
	if err != nil {
		return 0, err
	}
	now := m.nowFn().UTC()
	closed := 0
	for i := range items {
		if items[i].Type == ApprovalTypeToolPermission && items[i].Status == ApprovalStatusPending {
			markApprovalReviewed(&items[i], ApprovalStatusExpired, now)
			items[i].Note = "The server restarted before anyone answered."
			closed++
		}
	}
	if closed == 0 {
		return 0, nil
	}
	return closed, m.saveApprovalsLocked(items)
}

var errApprovalNotPending = errors.New("approval is no longer pending")

func (m *Manager) closeToolPermission(approvalID, next string) error {
	if m == nil {
		return fmt.Errorf("ops manager is nil")
	}
	id := strings.TrimSpace(approvalID)
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.loadApprovalsLocked()
	if err != nil {
		return err
	}
	index, err := approvalIndexByID(items, id)
	if err != nil {
		return err
	}
	if items[index].Type != ApprovalTypeToolPermission {
		return fmt.Errorf("approval %s is not a tool permission", id)
	}
	if items[index].Status != ApprovalStatusPending {
		return errApprovalNotPending
	}
	markApprovalReviewed(&items[index], next, m.nowFn().UTC())
	if err := m.saveApprovalsLocked(items); err != nil {
		return err
	}
	_ = m.appendEventLocked("approval_"+next, map[string]any{"approval_id": id})
	return nil
}

// IsApprovalNotPending reports whether err says the approval was already
// closed.
func IsApprovalNotPending(err error) bool {
	return errors.Is(err, errApprovalNotPending)
}

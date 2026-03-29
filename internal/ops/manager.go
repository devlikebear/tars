package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Options struct {
	HomeDir    string
	Now        func() time.Time
	MinFileAge time.Duration
}

type Status struct {
	Timestamp       time.Time `json:"timestamp"`
	DiskTotalBytes  uint64    `json:"disk_total_bytes"`
	DiskFreeBytes   uint64    `json:"disk_free_bytes"`
	DiskUsedPercent float64   `json:"disk_used_percent"`
	ProcessCount    int       `json:"process_count"`
}

type CleanupCandidate struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Reason    string `json:"reason,omitempty"`
}

type CleanupPlan struct {
	ApprovalID string             `json:"approval_id"`
	CreatedAt  time.Time          `json:"created_at"`
	TotalBytes int64              `json:"total_bytes"`
	Candidates []CleanupCandidate `json:"candidates"`
}

type Approval struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Status      string      `json:"status"`
	RequestedAt time.Time   `json:"requested_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	ReviewedAt  *time.Time  `json:"reviewed_at,omitempty"`
	Plan        CleanupPlan `json:"plan"`
	Note        string      `json:"note,omitempty"`
}

type CleanupApplyResult struct {
	ApprovalID   string   `json:"approval_id"`
	DeletedCount int      `json:"deleted_count"`
	DeletedBytes int64    `json:"deleted_bytes"`
	Errors       []string `json:"errors,omitempty"`
}

type Manager struct {
	mu            sync.Mutex
	workspaceDir  string
	homeDir       string
	approvalsPath string
	eventsDir     string
	nowFn         func() time.Time
	minFileAge    time.Duration
}

func NewManager(workspaceDir string, opts Options) *Manager {
	home := strings.TrimSpace(opts.HomeDir)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	root := strings.TrimSpace(workspaceDir)
	if root == "" {
		root = "./workspace"
	}
	return &Manager{
		workspaceDir:  root,
		homeDir:       home,
		approvalsPath: filepath.Join(root, "ops", "approvals.json"),
		eventsDir:     filepath.Join(root, "ops", "events"),
		nowFn:         nowFn,
		minFileAge:    opts.MinFileAge,
	}
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	_ = ctx
	if m == nil {
		return Status{}, fmt.Errorf("ops manager is nil")
	}
	now := m.nowFn().UTC()
	var fs syscall.Statfs_t
	if err := syscall.Statfs(strings.TrimSpace(m.homeDir), &fs); err != nil {
		return Status{}, err
	}
	total := fs.Blocks * uint64(fs.Bsize)
	free := fs.Bavail * uint64(fs.Bsize)
	usedPercent := float64(0)
	if total > 0 {
		usedPercent = (float64(total-free) / float64(total)) * 100
	}
	count, _ := processCount()
	return Status{
		Timestamp:       now,
		DiskTotalBytes:  total,
		DiskFreeBytes:   free,
		DiskUsedPercent: usedPercent,
		ProcessCount:    count,
	}, nil
}

func (m *Manager) CreateCleanupPlan(ctx context.Context) (CleanupPlan, error) {
	if m == nil {
		return CleanupPlan{}, fmt.Errorf("ops manager is nil")
	}
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	candidates, err := m.scanCandidates()
	if err != nil {
		return CleanupPlan{}, err
	}
	now := m.nowFn().UTC()
	plan := CleanupPlan{
		ApprovalID: newApprovalID(now),
		CreatedAt:  now,
		Candidates: candidates,
	}
	for _, item := range candidates {
		plan.TotalBytes += item.SizeBytes
	}
	approvals, err := m.loadApprovalsLocked()
	if err != nil {
		return CleanupPlan{}, err
	}
	approvals = append(approvals, Approval{
		ID:          plan.ApprovalID,
		Type:        "cleanup",
		Status:      "pending",
		RequestedAt: now,
		UpdatedAt:   now,
		Plan:        plan,
	})
	if err := m.saveApprovalsLocked(approvals); err != nil {
		return CleanupPlan{}, err
	}
	_ = m.appendEventLocked("cleanup_plan_created", map[string]any{
		"approval_id": plan.ApprovalID,
		"total_bytes": plan.TotalBytes,
		"candidates":  len(plan.Candidates),
	})
	return plan, nil
}

func (m *Manager) ApplyCleanup(ctx context.Context, approvalID string) (CleanupApplyResult, error) {
	if m == nil {
		return CleanupApplyResult{}, fmt.Errorf("ops manager is nil")
	}
	_ = ctx
	id := strings.TrimSpace(approvalID)
	if id == "" {
		return CleanupApplyResult{}, fmt.Errorf("approval_id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	approval, approvals, index, err := m.loadApprovedCleanupLocked(id)
	if err != nil {
		return CleanupApplyResult{}, err
	}
	result := m.applyCleanupPlanLocked(id, approval)

	now := m.nowFn().UTC()
	reviewedAt := now
	approval.Status = "applied"
	approval.UpdatedAt = now
	approval.ReviewedAt = &reviewedAt
	approvals[index] = approval
	if err := m.saveApprovalsLocked(approvals); err != nil {
		return CleanupApplyResult{}, err
	}
	_ = m.appendEventLocked("cleanup_applied", map[string]any{
		"approval_id":   id,
		"deleted_count": result.DeletedCount,
		"deleted_bytes": result.DeletedBytes,
		"errors":        len(result.Errors),
	})
	return result, nil
}

func (m *Manager) ListApprovals() ([]Approval, error) {
	if m == nil {
		return nil, fmt.Errorf("ops manager is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.loadApprovalsLocked()
	if err != nil {
		return nil, err
	}
	sortApprovalsNewestFirst(items)
	if items == nil {
		return []Approval{}, nil
	}
	return items, nil
}

func (m *Manager) Approve(approvalID string) error {
	return m.updateApprovalStatus(approvalID, "approved")
}

func (m *Manager) Reject(approvalID string) error {
	return m.updateApprovalStatus(approvalID, "rejected")
}

// SetNote updates the note field of an approval record.
func (m *Manager) SetNote(approvalID, note string) error {
	if m == nil {
		return fmt.Errorf("ops manager is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	approvals, err := m.loadApprovalsLocked()
	if err != nil {
		return err
	}
	for i, a := range approvals {
		if a.ID == approvalID {
			approvals[i].Note = note
			return m.saveApprovalsLocked(approvals)
		}
	}
	return fmt.Errorf("approval not found: %s", approvalID)
}

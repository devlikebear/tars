package ops

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	approvals, err := m.loadApprovalsLocked()
	if err != nil {
		return CleanupApplyResult{}, err
	}
	index := -1
	for i := range approvals {
		if strings.TrimSpace(approvals[i].ID) == id {
			index = i
			break
		}
	}
	if index < 0 {
		return CleanupApplyResult{}, fmt.Errorf("approval not found: %s", id)
	}
	approval := approvals[index]
	if approval.Status != "approved" {
		return CleanupApplyResult{}, fmt.Errorf("approval is not approved: %s", approval.Status)
	}

	result := CleanupApplyResult{ApprovalID: id}
	for _, candidate := range approval.Plan.Candidates {
		absPath := strings.TrimSpace(candidate.Path)
		if !m.isSafeCleanupPath(absPath) {
			result.Errors = append(result.Errors, "unsafe cleanup path rejected: "+absPath)
			continue
		}
		if err := os.Remove(absPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		result.DeletedCount++
		result.DeletedBytes += candidate.SizeBytes
	}
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
	sort.Slice(items, func(i, j int) bool {
		return items[i].RequestedAt.After(items[j].RequestedAt)
	})
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

func (m *Manager) updateApprovalStatus(approvalID string, next string) error {
	if m == nil {
		return fmt.Errorf("ops manager is nil")
	}
	id := strings.TrimSpace(approvalID)
	if id == "" {
		return fmt.Errorf("approval_id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.loadApprovalsLocked()
	if err != nil {
		return err
	}
	for i := range items {
		if strings.TrimSpace(items[i].ID) != id {
			continue
		}
		now := m.nowFn().UTC()
		items[i].Status = next
		items[i].UpdatedAt = now
		items[i].ReviewedAt = &now
		if err := m.saveApprovalsLocked(items); err != nil {
			return err
		}
		_ = m.appendEventLocked("approval_"+next, map[string]any{"approval_id": id})
		return nil
	}
	return fmt.Errorf("approval not found: %s", id)
}

func (m *Manager) scanCandidates() ([]CleanupCandidate, error) {
	roots := m.safeRoots()
	now := m.nowFn().UTC()
	out := make([]CleanupCandidate, 0)
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		reason := cleanupReason(root)
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			stat, err := d.Info()
			if err != nil {
				return nil
			}
			if !stat.Mode().IsRegular() {
				return nil
			}
			if m.minFileAge > 0 && now.Sub(stat.ModTime().UTC()) < m.minFileAge {
				return nil
			}
			out = append(out, CleanupCandidate{
				Path:      path,
				SizeBytes: stat.Size(),
				Reason:    reason,
			})
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SizeBytes == out[j].SizeBytes {
			return out[i].Path < out[j].Path
		}
		return out[i].SizeBytes > out[j].SizeBytes
	})
	if len(out) > 200 {
		out = out[:200]
	}
	return out, nil
}

func (m *Manager) isSafeCleanupPath(path string) bool {
	cleanPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return false
	}
	for _, root := range m.safeRoots() {
		cleanRoot, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			continue
		}
		prefix := cleanRoot + string(os.PathSeparator)
		if cleanPath == cleanRoot || strings.HasPrefix(cleanPath, prefix) {
			return true
		}
	}
	return false
}

func (m *Manager) safeRoots() []string {
	home := strings.TrimSpace(m.homeDir)
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Library", "Caches"),
		filepath.Join(home, ".Trash"),
	}
}

func (m *Manager) loadApprovalsLocked() ([]Approval, error) {
	if err := os.MkdirAll(filepath.Dir(m.approvalsPath), 0o755); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(m.approvalsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Approval{}, nil
		}
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return []Approval{}, nil
	}
	items := []Approval{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	if items == nil {
		return []Approval{}, nil
	}
	return items, nil
}

func (m *Manager) saveApprovalsLocked(items []Approval) error {
	if err := os.MkdirAll(filepath.Dir(m.approvalsPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.approvalsPath, payload, 0o644)
}

func (m *Manager) appendEventLocked(eventType string, payload map[string]any) error {
	if err := os.MkdirAll(m.eventsDir, 0o755); err != nil {
		return err
	}
	now := m.nowFn().UTC()
	path := filepath.Join(m.eventsDir, now.Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	record := map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"type":      strings.TrimSpace(eventType),
		"payload":   payload,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func processCount() (int, error) {
	out, err := exec.Command("ps", "-A", "-o", "pid=").Output()
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func cleanupReason(root string) string {
	v := strings.TrimSpace(root)
	switch {
	case strings.HasSuffix(v, string(os.PathSeparator)+"Downloads"):
		return "downloads cleanup"
	case strings.HasSuffix(v, string(os.PathSeparator)+"Desktop"):
		return "desktop cleanup"
	case strings.HasSuffix(v, string(os.PathSeparator)+"Caches"):
		return "cache cleanup"
	case strings.HasSuffix(v, string(os.PathSeparator)+".Trash"):
		return "trash cleanup"
	default:
		return "ops cleanup"
	}
}

func newApprovalID(now time.Time) string {
	return "apr_" + now.UTC().Format("20060102T150405.000000000")
}

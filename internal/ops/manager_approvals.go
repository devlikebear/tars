package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
	index, err := approvalIndexByID(items, id)
	if err != nil {
		return err
	}
	markApprovalReviewed(&items[index], strings.TrimSpace(next), m.nowFn().UTC())
	if err := m.saveApprovalsLocked(items); err != nil {
		return err
	}
	_ = m.appendEventLocked("approval_"+next, map[string]any{"approval_id": id})
	return nil
}

func approvalIndexByID(items []Approval, id string) (int, error) {
	for i := range items {
		if strings.TrimSpace(items[i].ID) == strings.TrimSpace(id) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("approval not found: %s", strings.TrimSpace(id))
}

func markApprovalReviewed(item *Approval, status string, now time.Time) {
	if item == nil {
		return
	}
	item.Status = strings.TrimSpace(status)
	item.UpdatedAt = now
	item.ReviewedAt = &now
}

func sortApprovalsNewestFirst(items []Approval) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].RequestedAt.After(items[j].RequestedAt)
	})
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

func newApprovalID(now time.Time) string {
	return "apr_" + now.UTC().Format("20060102T150405.000000000")
}

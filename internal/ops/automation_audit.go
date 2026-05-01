package ops

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AutomationAuditEntry struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	Reason    string         `json:"reason,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	CWD       string         `json:"cwd,omitempty"`
	Result    string         `json:"result"`
	Details   map[string]any `json:"details,omitempty"`
}

type AutomationAuditListOptions struct {
	Limit     int
	SessionID string
}

func (m *Manager) RecordAutomationAudit(entry AutomationAuditEntry) (AutomationAuditEntry, error) {
	if m == nil {
		return AutomationAuditEntry{}, fmt.Errorf("ops manager is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	entry = m.normalizeAutomationAuditEntry(entry)
	if entry.Actor == "" {
		return AutomationAuditEntry{}, fmt.Errorf("actor is required")
	}
	if entry.Action == "" {
		return AutomationAuditEntry{}, fmt.Errorf("action is required")
	}
	if entry.Result == "" {
		return AutomationAuditEntry{}, fmt.Errorf("result is required")
	}
	if err := os.MkdirAll(filepath.Dir(m.auditPath), 0o755); err != nil {
		return AutomationAuditEntry{}, err
	}
	file, err := os.OpenFile(m.auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return AutomationAuditEntry{}, err
	}
	defer file.Close()
	encoded, err := json.Marshal(entry)
	if err != nil {
		return AutomationAuditEntry{}, err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return AutomationAuditEntry{}, err
	}
	return entry, nil
}

func (m *Manager) ListAutomationAudit(opts AutomationAuditListOptions) ([]AutomationAuditEntry, error) {
	if m == nil {
		return nil, fmt.Errorf("ops manager is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	file, err := os.Open(m.auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []AutomationAuditEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	filterSessionID := strings.TrimSpace(opts.SessionID)
	items := []AutomationAuditEntry{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry AutomationAuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entry = m.normalizeAutomationAuditEntry(entry)
		if filterSessionID != "" && entry.SessionID != filterSessionID {
			continue
		}
		items = append(items, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Timestamp.Equal(items[j].Timestamp) {
			return items[i].ID > items[j].ID
		}
		return items[i].Timestamp.After(items[j].Timestamp)
	})
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (m *Manager) normalizeAutomationAuditEntry(entry AutomationAuditEntry) AutomationAuditEntry {
	entry.Actor = strings.TrimSpace(entry.Actor)
	entry.Action = strings.TrimSpace(entry.Action)
	entry.Reason = strings.TrimSpace(entry.Reason)
	entry.SessionID = strings.TrimSpace(entry.SessionID)
	entry.CWD = strings.TrimSpace(entry.CWD)
	entry.Result = strings.TrimSpace(entry.Result)
	if entry.Timestamp.IsZero() {
		entry.Timestamp = m.nowFn().UTC()
	} else {
		entry.Timestamp = entry.Timestamp.UTC()
	}
	if entry.ID == "" {
		entry.ID = automationAuditID(entry)
	} else {
		entry.ID = strings.TrimSpace(entry.ID)
	}
	return entry
}

func automationAuditID(entry AutomationAuditEntry) string {
	parts := []string{
		entry.Timestamp.UTC().Format(time.RFC3339Nano),
		entry.Actor,
		entry.Action,
		entry.SessionID,
		entry.CWD,
		entry.Result,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "auto_" + hex.EncodeToString(sum[:])[:16]
}

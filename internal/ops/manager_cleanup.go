package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (m *Manager) loadApprovedCleanupLocked(id string) (Approval, []Approval, int, error) {
	approvals, err := m.loadApprovalsLocked()
	if err != nil {
		return Approval{}, nil, -1, err
	}
	index, err := approvalIndexByID(approvals, id)
	if err != nil {
		return Approval{}, nil, -1, err
	}
	approval := approvals[index]
	if approval.Status != "approved" {
		return Approval{}, nil, -1, fmt.Errorf("approval is not approved: %s", approval.Status)
	}
	return approval, approvals, index, nil
}

func (m *Manager) applyCleanupPlanLocked(id string, approval Approval) CleanupApplyResult {
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
	return result
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
		items, err := m.scanCleanupRoot(root, now)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sortCleanupCandidates(out)
	if len(out) > 200 {
		out = out[:200]
	}
	return out, nil
}

func (m *Manager) scanCleanupRoot(root string, now time.Time) ([]CleanupCandidate, error) {
	out := make([]CleanupCandidate, 0)
	reason := cleanupReason(root)
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		stat, err := d.Info()
		if err != nil || !stat.Mode().IsRegular() {
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
	return out, nil
}

func sortCleanupCandidates(items []CleanupCandidate) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SizeBytes == items[j].SizeBytes {
			return items[i].Path < items[j].Path
		}
		return items[i].SizeBytes > items[j].SizeBytes
	})
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

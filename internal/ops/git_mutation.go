package ops

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	gitrepo "github.com/devlikebear/tars/internal/git"
)

const (
	GitMutationStage        = "stage"
	GitMutationUnstage      = "unstage"
	GitMutationDiscard      = "discard"
	GitMutationCommit       = "commit"
	GitMutationSwitchBranch = "switch_branch"
)

type GitMutationPlan struct {
	ApprovalID  string    `json:"approval_id"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	SessionID   string    `json:"session_id,omitempty"`
	Root        string    `json:"root"`
	Action      string    `json:"action"`
	Path        string    `json:"path,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Message     string    `json:"message,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Command     string    `json:"command"`
	Destructive bool      `json:"destructive,omitempty"`
}

type GitMutationApplyResult struct {
	ApprovalID  string `json:"approval_id"`
	SessionID   string `json:"session_id,omitempty"`
	Root        string `json:"root"`
	Action      string `json:"action"`
	Path        string `json:"path,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Result      string `json:"result"`
	Output      string `json:"output,omitempty"`
	Destructive bool   `json:"destructive,omitempty"`
}

func (m *Manager) CreateGitMutationApproval(ctx context.Context, plan GitMutationPlan) (GitMutationPlan, error) {
	if m == nil {
		return GitMutationPlan{}, fmt.Errorf("ops manager is nil")
	}
	normalized, err := normalizeGitMutationPlan(ctx, plan, m.nowFn().UTC())
	if err != nil {
		return GitMutationPlan{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	approvals, err := m.loadApprovalsLocked()
	if err != nil {
		return GitMutationPlan{}, err
	}
	approvals = append(approvals, Approval{
		ID:          normalized.ApprovalID,
		Type:        "git_mutation",
		Status:      "pending",
		RequestedAt: normalized.CreatedAt,
		UpdatedAt:   normalized.CreatedAt,
		Plan:        CleanupPlan{Candidates: []CleanupCandidate{}},
		GitMutation: &normalized,
	})
	if err := m.saveApprovalsLocked(approvals); err != nil {
		return GitMutationPlan{}, err
	}
	_ = m.appendEventLocked("git_mutation_approval_created", map[string]any{
		"approval_id": normalized.ApprovalID,
		"session_id":  normalized.SessionID,
		"root":        normalized.Root,
		"action":      normalized.Action,
		"path":        normalized.Path,
		"branch":      normalized.Branch,
		"destructive": normalized.Destructive,
	})
	return normalized, nil
}

func (m *Manager) ApplyGitMutation(ctx context.Context, approvalID string) (GitMutationApplyResult, error) {
	if m == nil {
		return GitMutationApplyResult{}, fmt.Errorf("ops manager is nil")
	}
	id := strings.TrimSpace(approvalID)
	if id == "" {
		return GitMutationApplyResult{}, fmt.Errorf("approval_id is required")
	}

	m.mu.Lock()
	approval, err := m.loadApprovedGitMutationLocked(id)
	m.mu.Unlock()
	if err != nil {
		return GitMutationApplyResult{}, err
	}
	plan := *approval.GitMutation
	result := GitMutationApplyResult{
		ApprovalID:  id,
		SessionID:   plan.SessionID,
		Root:        plan.Root,
		Action:      plan.Action,
		Path:        plan.Path,
		Branch:      plan.Branch,
		Destructive: plan.Destructive,
	}
	mutation, err := gitrepo.NewClient().Mutate(ctx, gitrepo.MutationOptions{
		StartDir: plan.Root,
		Action:   gitrepo.MutationAction(plan.Action),
		Path:     plan.Path,
		Branch:   plan.Branch,
		Message:  plan.Message,
	})
	if err != nil {
		result.Result = "failure"
		_, _ = m.RecordAutomationAudit(automationAuditForGitMutation(plan, result, err.Error()))
		return result, err
	}
	result.Result = "success"
	result.Output = mutation.Output
	result.Destructive = mutation.Destructive

	now := m.nowFn().UTC()
	reviewedAt := now
	approval.Status = "applied"
	approval.UpdatedAt = now
	approval.ReviewedAt = &reviewedAt
	approval.Note = gitMutationNote(result)

	m.mu.Lock()
	approvals, saveErr := m.loadApprovalsLocked()
	if saveErr == nil {
		index, indexErr := approvalIndexByID(approvals, id)
		if indexErr != nil {
			saveErr = indexErr
		} else {
			approvals[index] = approval
			saveErr = m.saveApprovalsLocked(approvals)
		}
	}
	if saveErr == nil {
		_ = m.appendEventLocked("git_mutation_applied", map[string]any{
			"approval_id": id,
			"session_id":  plan.SessionID,
			"root":        plan.Root,
			"action":      plan.Action,
			"path":        plan.Path,
			"branch":      plan.Branch,
		})
	}
	m.mu.Unlock()
	if saveErr != nil {
		return result, saveErr
	}
	_, _ = m.RecordAutomationAudit(automationAuditForGitMutation(plan, result, ""))
	return result, nil
}

func (m *Manager) loadApprovedGitMutationLocked(id string) (Approval, error) {
	approvals, err := m.loadApprovalsLocked()
	if err != nil {
		return Approval{}, err
	}
	index, err := approvalIndexByID(approvals, id)
	if err != nil {
		return Approval{}, err
	}
	approval := approvals[index]
	if approval.Type != "git_mutation" || approval.GitMutation == nil {
		return Approval{}, fmt.Errorf("approval is not a git mutation")
	}
	if approval.Status != "approved" {
		return Approval{}, fmt.Errorf("approval is not approved: %s", approval.Status)
	}
	return approval, nil
}

func normalizeGitMutationPlan(ctx context.Context, plan GitMutationPlan, now time.Time) (GitMutationPlan, error) {
	action := strings.TrimSpace(plan.Action)
	if action == "" {
		return GitMutationPlan{}, fmt.Errorf("action is required")
	}
	root, err := gitrepo.NewClient().RepositoryRoot(ctx, strings.TrimSpace(plan.Root))
	if err != nil {
		return GitMutationPlan{}, err
	}
	normalized := GitMutationPlan{
		ApprovalID: strings.TrimSpace(plan.ApprovalID),
		Type:       "git_mutation",
		CreatedAt:  now,
		SessionID:  strings.TrimSpace(plan.SessionID),
		Root:       filepath.Clean(root),
		Action:     action,
		Path:       strings.TrimSpace(plan.Path),
		Branch:     strings.TrimSpace(plan.Branch),
		Message:    strings.TrimSpace(plan.Message),
		Reason:     strings.TrimSpace(plan.Reason),
	}
	if normalized.ApprovalID == "" {
		normalized.ApprovalID = newApprovalID(now)
	}
	switch normalized.Action {
	case GitMutationStage:
		if normalized.Path, err = normalizeGitMutationPath(normalized.Path); err != nil {
			return GitMutationPlan{}, err
		}
		normalized.Command = fmt.Sprintf("git add -- %s", normalized.Path)
	case GitMutationUnstage:
		if normalized.Path, err = normalizeGitMutationPath(normalized.Path); err != nil {
			return GitMutationPlan{}, err
		}
		normalized.Command = fmt.Sprintf("git restore --staged -- %s", normalized.Path)
	case GitMutationDiscard:
		if normalized.Path, err = normalizeGitMutationPath(normalized.Path); err != nil {
			return GitMutationPlan{}, err
		}
		normalized.Command = fmt.Sprintf("git restore --worktree -- %s", normalized.Path)
		normalized.Destructive = true
	case GitMutationCommit:
		if normalized.Message == "" {
			return GitMutationPlan{}, fmt.Errorf("commit message is required")
		}
		normalized.Command = "git commit -m <message>"
	case GitMutationSwitchBranch:
		if normalized.Branch == "" {
			return GitMutationPlan{}, fmt.Errorf("branch is required")
		}
		normalized.Command = fmt.Sprintf("git switch -- %s", normalized.Branch)
	default:
		return GitMutationPlan{}, fmt.Errorf("unsupported git mutation action: %s", normalized.Action)
	}
	return normalized, nil
}

func normalizeGitMutationPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path must stay inside the repository")
	}
	return clean, nil
}

func automationAuditForGitMutation(plan GitMutationPlan, result GitMutationApplyResult, errText string) AutomationAuditEntry {
	details := map[string]any{
		"approval_id": result.ApprovalID,
		"command":     plan.Command,
	}
	if result.Path != "" {
		details["path"] = result.Path
	}
	if result.Branch != "" {
		details["branch"] = result.Branch
	}
	if errText != "" {
		details["error"] = errText
	}
	return AutomationAuditEntry{
		Actor:     "git",
		Action:    "git." + plan.Action,
		Reason:    plan.Reason,
		SessionID: plan.SessionID,
		CWD:       plan.Root,
		Result:    result.Result,
		Details:   details,
	}
}

func gitMutationNote(result GitMutationApplyResult) string {
	switch result.Action {
	case GitMutationStage:
		return "Staged " + result.Path
	case GitMutationUnstage:
		return "Unstaged " + result.Path
	case GitMutationDiscard:
		return "Discarded " + result.Path
	case GitMutationCommit:
		if strings.TrimSpace(result.Output) != "" {
			return result.Output
		}
		return "Created commit"
	case GitMutationSwitchBranch:
		return "Switched to " + result.Branch
	default:
		return strings.TrimSpace(result.Output)
	}
}

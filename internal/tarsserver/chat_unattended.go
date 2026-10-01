package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/pkg/agentloop"
)

// Unattended approvals (#970). A cron job, a Telegram message or a subagent
// runs a turn nobody watches in the console, so there is no card to answer.
// Before session permission modes, such turns ran every tool (claude-code-cli
// in "auto", native tools without a gate), and a session that never picks a
// mode still does.
//
// Once a session picks Ask, Accept edits or Plan, its unattended turns follow
// the same mode: what the mode lets through runs, plan mode refuses edits,
// and what it would ask about becomes a tool_permission approval in the ops
// queue. The turn waits for the review (or until unattendedApprovalTimeout),
// and a notification tells whoever is around that the session needs input.

const unattendedApprovalTimeout = 30 * time.Minute

const unattendedPollInterval = 2 * time.Second

const unattendedPlanMessage = "Plan mode: nobody is watching this run to approve a plan. Give the plan as your answer instead of starting the work."

type unattendedPermissions struct {
	ops      *ops.Manager
	sessions *session.Store
	always   *chatAlwaysRuleStore
	notify   func(context.Context, notificationEvent)
	timeout  time.Duration
	poll     time.Duration
	now      func() time.Time
}

func newUnattendedPermissions(manager *ops.Manager, sessions *session.Store, always *chatAlwaysRuleStore, notify func(context.Context, notificationEvent)) *unattendedPermissions {
	if manager == nil || sessions == nil {
		return nil
	}
	return &unattendedPermissions{
		ops: manager, sessions: sessions, always: always, notify: notify,
		timeout: unattendedApprovalTimeout, poll: unattendedPollInterval, now: time.Now,
	}
}

// unattendedRun is one unattended turn: which session's mode applies, where
// it works, and what started it.
type unattendedRun struct {
	sessionID string
	cwd       string
	// source is cron, telegram or subagent.
	source string
	label  string
	mode   string
}

// unattendedRunOptions is what a gated turn adds to its agentloop options.
type unattendedRunOptions struct {
	authorizer     agentloop.ToolAuthorizer
	handler        llm.ClaudeCodePermissionHandler
	permissionMode string
	// permissionAllow are the Claude Code rules the person always allows in
	// the turn's folder.
	permissionAllow []string
}

func (o unattendedRunOptions) apply(opts *agentloop.RunOptions) {
	if o.authorizer == nil {
		return
	}
	opts.ToolAuthorizer = o.authorizer
	if o.permissionMode == "" {
		// Native gate only: the claude-code-cli fields stay as configured.
		return
	}
	opts.ClaudeCodePermissionHandler = o.handler
	opts.ClaudeCodePermissionMode = o.permissionMode
	opts.ClaudeCodePermissionAllow = o.permissionAllow
}

// options returns the gate for an unattended turn in sessionID, or zero
// options when the session has not picked a mode that asks: then the turn
// runs as it always has.
func (u *unattendedPermissions) options(sessionID, cwd, source, label string) unattendedRunOptions {
	if u == nil || strings.TrimSpace(sessionID) == "" {
		return unattendedRunOptions{}
	}
	sess, err := u.sessions.Get(sessionID)
	if err != nil {
		return unattendedRunOptions{}
	}
	mode := strings.TrimSpace(sess.PermissionMode)
	if !validChatPermissionMode(mode) || mode == chatPermissionModeAuto {
		return unattendedRunOptions{}
	}
	run := unattendedRun{sessionID: sess.ID, cwd: strings.TrimSpace(cwd), source: source, label: strings.TrimSpace(label), mode: mode}
	return unattendedRunOptions{
		authorizer:      &unattendedToolGate{perms: u, run: run},
		handler:         u.claudeCodeHandler(run),
		permissionMode:  claudeCodePermissionFlag(mode),
		permissionAllow: claudeCodeAlwaysRules(u.always, run.cwd),
	}
}

// focusOptions are the options of a server-driven focus turn: the session's
// mode as for any unattended turn, but a session that never picked one asks
// before high-risk native tools (manual, through the ops queue) — the same
// default a console turn there gets. claude-code-cli keeps its configured
// permission mode then: only the native gate is added.
func (u *unattendedPermissions) focusOptions(sessionID, cwd, source string) unattendedRunOptions {
	if opts := u.options(sessionID, cwd, source, source); opts.authorizer != nil || u == nil {
		return opts
	}
	sess, err := u.sessions.Get(sessionID)
	if err != nil || strings.TrimSpace(sess.PermissionMode) != "" {
		return unattendedRunOptions{}
	}
	run := unattendedRun{sessionID: sess.ID, cwd: strings.TrimSpace(cwd), source: source, label: source, mode: chatPermissionModeManual}
	return unattendedRunOptions{authorizer: &unattendedToolGate{perms: u, run: run}}
}

// unattendedToolGate is the native-provider gate of an unattended turn.
type unattendedToolGate struct {
	perms *unattendedPermissions
	run   unattendedRun
}

func (g *unattendedToolGate) Authorize(ctx context.Context, call agentloop.ToolCallRequest) (agentloop.ToolDecision, error) {
	name := tool.CanonicalToolName(call.ToolName)
	switch chatGateVerdictFor(g.run.mode, name) {
	case chatGateAllow:
		return agentloop.ToolDecision{Allow: true}, nil
	case chatGateRefuse:
		g.perms.record(g.run, call.ToolName, "", "refused_plan_mode")
		return agentloop.ToolDecision{Message: chatPlanModeMessage}, nil
	}
	if g.perms.alwaysAllows(g.run.cwd, name, call.ToolArgs) {
		return agentloop.ToolDecision{Allow: true}, nil
	}
	allowed, message, err := g.perms.ask(ctx, g.run, call.ToolName, []byte(call.ToolArgs), "")
	if err != nil {
		return agentloop.ToolDecision{}, err
	}
	return agentloop.ToolDecision{Allow: allowed, Message: message}, nil
}

func (u *unattendedPermissions) alwaysAllows(cwd, name, args string) bool {
	return (&chatPermissionBroker{always: u.always}).alwaysAllows(cwd, name, args)
}

// claudeCodeHandler answers Claude Code's permission prompts for an
// unattended turn. Rules the person chose to always allow reach the CLI
// through --settings, so what arrives here is what they have not allowed.
func (u *unattendedPermissions) claudeCodeHandler(run unattendedRun) llm.ClaudeCodePermissionHandler {
	return func(ctx context.Context, req llm.ClaudeCodePermissionRequest) (llm.ClaudeCodePermissionDecision, error) {
		if req.ToolName == chatExitPlanModeTool {
			u.record(run, req.ToolName, "", "refused_plan_approval")
			return llm.ClaudeCodePermissionDecision{Message: unattendedPlanMessage}, nil
		}
		allowed, message, err := u.ask(ctx, run, req.ToolName, req.Input, req.DecisionReason)
		if err != nil {
			return llm.ClaudeCodePermissionDecision{}, err
		}
		return llm.ClaudeCodePermissionDecision{Allow: allowed, Message: message}, nil
	}
}

// ask queues the call for review and waits for the answer. A call nobody
// answers in time is not run; a cancelled turn closes its question.
func (u *unattendedPermissions) ask(ctx context.Context, run unattendedRun, toolName string, input []byte, reason string) (bool, string, error) {
	approval, err := u.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{
		SessionID: run.sessionID,
		Source:    run.source,
		RunLabel:  run.label,
		ToolName:  toolName,
		Preview:   permissionInputPreview(input),
		Reason:    strings.TrimSpace(reason),
		CWD:       run.cwd,
		Mode:      run.mode,
	})
	if err != nil {
		return false, "", fmt.Errorf("queue tool approval: %w", err)
	}
	u.announce(ctx, run, approval)

	deadline := time.NewTimer(u.timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(u.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = u.ops.ExpireToolPermission(approval.ID)
			u.record(run, toolName, approval.ID, "withdrawn")
			return false, "", ctx.Err()
		case <-deadline.C:
			if err := u.ops.ExpireToolPermission(approval.ID); err == nil {
				if status := u.status(approval.ID); status != ops.ApprovalStatusExpired {
					// Reviewed between the last poll and the deadline.
					return u.outcome(run, toolName, approval.ID, status)
				}
			}
			u.record(run, toolName, approval.ID, "expired")
			return false, fmt.Sprintf("Nobody approved %s within %s, so it was not run. Finish without it or say what approval you need.", toolName, u.timeout), nil
		case <-ticker.C:
			status := u.status(approval.ID)
			if status == ops.ApprovalStatusPending || status == "" {
				continue
			}
			return u.outcome(run, toolName, approval.ID, status)
		}
	}
}

// queuedBySession counts the tool approvals still waiting in the ops queue
// for each session, for the session board.
func (u *unattendedPermissions) queuedBySession() (map[string]int, error) {
	approvals, err := u.pendingToolPermissions()
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, a := range approvals {
		out[a.ToolPermission.SessionID]++
	}
	return out, nil
}

// queuedApprovals lists the tool approvals still waiting in the ops queue,
// for GET /v1/chat/activity. They are answered through the ops approvals,
// never through the chat permission endpoint.
func (u *unattendedPermissions) queuedApprovals() ([]chatQueuedApproval, error) {
	approvals, err := u.pendingToolPermissions()
	if err != nil {
		return nil, err
	}
	out := make([]chatQueuedApproval, 0, len(approvals))
	for _, a := range approvals {
		req := a.ToolPermission
		out = append(out, chatQueuedApproval{
			ApprovalID:  a.ID,
			SessionID:   req.SessionID,
			Source:      req.Source,
			RunLabel:    req.RunLabel,
			ToolName:    req.ToolName,
			Preview:     capPreview(req.Preview),
			Reason:      req.Reason,
			RequestedAt: a.RequestedAt,
		})
	}
	return out, nil
}

// pendingToolPermissions are the tool_permission approvals nobody has
// answered yet.
func (u *unattendedPermissions) pendingToolPermissions() ([]ops.Approval, error) {
	approvals, err := u.ops.ListApprovals()
	if err != nil {
		return nil, err
	}
	out := approvals[:0]
	for _, a := range approvals {
		if a.Type == ops.ApprovalTypeToolPermission && a.Status == ops.ApprovalStatusPending && a.ToolPermission != nil {
			out = append(out, a)
		}
	}
	return out, nil
}

func (u *unattendedPermissions) status(id string) string {
	approval, err := u.ops.GetApproval(id)
	if err != nil {
		return ""
	}
	return approval.Status
}

func (u *unattendedPermissions) outcome(run unattendedRun, toolName, id, status string) (bool, string, error) {
	if status == ops.ApprovalStatusApproved {
		u.record(run, toolName, id, "allowed")
		return true, "", nil
	}
	u.record(run, toolName, id, "denied")
	return false, chatPermissionDenyMessage, nil
}

func (u *unattendedPermissions) announce(ctx context.Context, run unattendedRun, approval ops.Approval) {
	if u.notify == nil {
		return
	}
	title := "Needs input: approve " + approval.ToolPermission.ToolName
	if sess, err := u.sessions.Get(run.sessionID); err == nil && strings.TrimSpace(sess.Title) != "" {
		title += " in " + strings.TrimSpace(sess.Title)
	}
	message := run.source + " run waits for approval " + approval.ID
	if preview := approval.ToolPermission.Preview; preview != "" {
		message += ": " + preview
	}
	u.notify(context.WithoutCancel(ctx), newNotificationEvent("ops", "warning", title, message))
}

func (u *unattendedPermissions) record(run unattendedRun, toolName, id, outcome string) {
	broker := &chatPermissionBroker{audit: auditTo(u.ops)}
	broker.record(id, outcome, chatPermissionAudit{sessionID: run.sessionID, cwd: run.cwd, tool: toolName, mode: run.mode, source: run.source})
}

// permissionInputPreview picks the command, path or URL a call acts on, as
// the approval card does, or the whole input when none is set.
func permissionInputPreview(input []byte) string {
	var fields map[string]any
	if err := json.Unmarshal(input, &fields); err != nil {
		return truncatePreview(strings.TrimSpace(string(input)))
	}
	for _, key := range []string{"command", "file_path", "notebook_path", "path", "url"} {
		if value, ok := fields[key].(string); ok && strings.TrimSpace(value) != "" {
			return truncatePreview(strings.TrimSpace(value))
		}
	}
	return truncatePreview(strings.TrimSpace(string(input)))
}

func truncatePreview(value string) string {
	const limit = 400
	if len(value) <= limit {
		return value
	}
	cut := value[:limit]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…"
}

type unattendedPermissionsContextKey struct{}

// withSubagentPermissions wraps the agent runtime's prompt runner so a
// subagent's tools follow the permission mode of the session that spawned
// it (see newAgentPromptRunnerWithToolsAndMemory).
func withSubagentPermissions(runner agentRuntimePromptRunner, perms *unattendedPermissions) agentRuntimePromptRunner {
	if runner == nil || perms == nil {
		return runner
	}
	return func(ctx context.Context, runLabel string, promptText string, allowedTools []string, tier string, providerOverride *agentruntime.ProviderOverride) (string, error) {
		ctx = context.WithValue(ctx, unattendedPermissionsContextKey{}, perms)
		return runner(ctx, runLabel, promptText, allowedTools, tier, providerOverride)
	}
}

// subagentRunOptions returns the gate for a subagent run in dir, or zero
// options when the run has no parent session or the runner was not wrapped.
func subagentRunOptions(ctx context.Context, dir, label string) unattendedRunOptions {
	perms, _ := ctx.Value(unattendedPermissionsContextKey{}).(*unattendedPermissions)
	return perms.options(agentruntime.ParentSessionFromContext(ctx), dir, "subagent", label)
}

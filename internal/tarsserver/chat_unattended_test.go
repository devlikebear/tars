package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/rs/zerolog"
)

type unattendedFixture struct {
	perms   *unattendedPermissions
	ops     *ops.Manager
	store   *session.Store
	session string
	dir     string
	root    string

	mu     sync.Mutex
	events []notificationEvent
}

func newUnattendedFixture(t *testing.T, mode string) *unattendedFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("nightly")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "" {
		if err := store.SetPermissionMode(sess.ID, mode); err != nil {
			t.Fatal(err)
		}
	}
	f := &unattendedFixture{ops: ops.NewManager(root, ops.Options{HomeDir: t.TempDir()}), store: store, session: sess.ID, dir: t.TempDir(), root: root}
	f.perms = newUnattendedPermissions(f.ops, store, newChatAlwaysRuleStore(root), func(_ context.Context, evt notificationEvent) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.events = append(f.events, evt)
	})
	f.perms.poll = 5 * time.Millisecond
	f.perms.timeout = 2 * time.Second
	return f
}

// answerNext reviews the next pending tool approval once it appears.
func (f *unattendedFixture) answerNext(t *testing.T, approve bool) {
	t.Helper()
	go func() {
		for i := 0; i < 400; i++ {
			items, _ := f.ops.ListApprovals()
			for _, item := range items {
				if item.Type == ops.ApprovalTypeToolPermission && item.Status == ops.ApprovalStatusPending {
					_ = f.ops.ReviewToolPermission(item.ID, approve)
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

func (f *unattendedFixture) auditResults(t *testing.T) []string {
	t.Helper()
	entries, err := f.ops.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: f.session, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Actor+":"+e.Result)
	}
	return out
}

func TestUnattendedOptionsFollowTheSessionMode(t *testing.T) {
	var nilPerms *unattendedPermissions
	if nilPerms.options("s", "", "cron", "").authorizer != nil {
		t.Fatal("nil permissions must not gate")
	}
	if newUnattendedPermissions(nil, nil, nil, nil) != nil {
		t.Fatal("missing ops manager should disable the gate")
	}
	for _, mode := range []string{"", chatPermissionModeAuto} {
		f := newUnattendedFixture(t, mode)
		opts := f.perms.options(f.session, f.dir, "cron", "cron:job")
		if opts.authorizer != nil || opts.handler != nil || opts.permissionMode != "" {
			t.Fatalf("mode %q should run as before, got %+v", mode, opts)
		}
		var run agentloop.RunOptions
		opts.apply(&run)
		if run.ToolAuthorizer != nil || run.ClaudeCodePermissionMode != "" {
			t.Fatalf("zero options changed run options: %+v", run)
		}
	}
	f := newUnattendedFixture(t, chatPermissionModeManual)
	if f.perms.options("missing", f.dir, "cron", "").authorizer != nil {
		t.Fatal("unknown session should not gate")
	}
	opts := f.perms.options(f.session, f.dir, "cron", "cron:job")
	var run agentloop.RunOptions
	opts.apply(&run)
	if run.ToolAuthorizer == nil || run.ClaudeCodePermissionHandler == nil || run.ClaudeCodePermissionMode != "default" {
		t.Fatalf("manual session options: %+v", run)
	}
}

func TestUnattendedGateVerdicts(t *testing.T) {
	ctx := context.Background()
	plan := newUnattendedFixture(t, chatPermissionModePlan)
	gate := plan.perms.options(plan.session, plan.dir, "cron", "").authorizer
	if d, err := gate.Authorize(ctx, agentloop.ToolCallRequest{ToolName: "read_file", ToolArgs: `{"path":"a"}`}); err != nil || !d.Allow {
		t.Fatalf("read tool: %+v %v", d, err)
	}
	d, err := gate.Authorize(ctx, agentloop.ToolCallRequest{ToolName: "write_file", ToolArgs: `{"path":"a"}`})
	if err != nil || d.Allow || d.Message != chatPlanModeMessage {
		t.Fatalf("plan write: %+v %v", d, err)
	}
	if got := plan.auditResults(t); len(got) != 1 || got[0] != "tars:refused_plan_mode" {
		t.Fatalf("plan audit = %v", got)
	}

	edits := newUnattendedFixture(t, chatPermissionModeAcceptEdits)
	gate = edits.perms.options(edits.session, edits.dir, "cron", "").authorizer
	if d, err := gate.Authorize(ctx, agentloop.ToolCallRequest{ToolName: "write_file", ToolArgs: `{"path":"a"}`}); err != nil || !d.Allow {
		t.Fatalf("accept edits write: %+v %v", d, err)
	}
}

func TestUnattendedGateQueuesAndWaits(t *testing.T) {
	ctx := context.Background()
	f := newUnattendedFixture(t, chatPermissionModeManual)
	gate := f.perms.options(f.session, f.dir, "cron", "cron:nightly").authorizer
	call := agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"go test ./..."}`}

	f.answerNext(t, true)
	d, err := gate.Authorize(ctx, call)
	if err != nil || !d.Allow {
		t.Fatalf("approved: %+v %v", d, err)
	}
	f.answerNext(t, false)
	d, err = gate.Authorize(ctx, call)
	if err != nil || d.Allow || d.Message != chatPermissionDenyMessage {
		t.Fatalf("rejected: %+v %v", d, err)
	}

	items, _ := f.ops.ListApprovals()
	if len(items) != 2 || items[0].ToolPermission.Preview != "go test ./..." || items[0].ToolPermission.Source != "cron" || items[0].ToolPermission.Mode != chatPermissionModeManual {
		t.Fatalf("queued approvals: %+v", items)
	}
	f.mu.Lock()
	events := append([]notificationEvent(nil), f.events...)
	f.mu.Unlock()
	if len(events) != 2 || events[0].Severity != "warning" || !strings.Contains(events[0].Title, "Needs input: approve exec in nightly") || !strings.Contains(events[0].Message, "go test ./...") {
		t.Fatalf("notifications: %+v", events)
	}
	got := f.auditResults(t)
	if len(got) != 2 || got[0] != "ops:denied" || got[1] != "ops:allowed" {
		t.Fatalf("audit = %v", got)
	}
}

func TestUnattendedGateAlwaysRuleSkipsTheQueue(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	rule := chatToolRule{Tool: "exec", Prefix: "go test"}.always()
	if err := f.perms.always.add(f.dir, rule); err != nil {
		t.Fatal(err)
	}
	gate := f.perms.options(f.session, f.dir, "cron", "").authorizer
	d, err := gate.Authorize(context.Background(), agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"go test ./..."}`})
	if err != nil || !d.Allow {
		t.Fatalf("always rule: %+v %v", d, err)
	}
	if items, _ := f.ops.ListApprovals(); len(items) != 0 {
		t.Fatalf("always-allowed call was queued: %+v", items)
	}
}

func TestUnattendedGateExpiresWithoutAnAnswer(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	f.perms.timeout = 30 * time.Millisecond
	gate := f.perms.options(f.session, f.dir, "telegram", "").authorizer
	d, err := gate.Authorize(context.Background(), agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"make"}`})
	if err != nil || d.Allow || !strings.Contains(d.Message, "Nobody approved exec") {
		t.Fatalf("timeout: %+v %v", d, err)
	}
	items, _ := f.ops.ListApprovals()
	if len(items) != 1 || items[0].Status != ops.ApprovalStatusExpired {
		t.Fatalf("approval after timeout: %+v", items)
	}
	if got := f.auditResults(t); len(got) != 1 || got[0] != "tars:expired" {
		t.Fatalf("audit = %v", got)
	}
}

func TestUnattendedGateCancelledTurnWithdraws(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	gate := f.perms.options(f.session, f.dir, "subagent", "").authorizer
	if _, err := gate.Authorize(ctx, agentloop.ToolCallRequest{ToolName: "exec", ToolArgs: `{"command":"make"}`}); err == nil {
		t.Fatal("expected the cancelled turn's error")
	}
	items, _ := f.ops.ListApprovals()
	if len(items) != 1 || items[0].Status != ops.ApprovalStatusExpired {
		t.Fatalf("approval after cancel: %+v", items)
	}
	if got := f.auditResults(t); len(got) != 1 || got[0] != "tars:withdrawn" {
		t.Fatalf("audit = %v", got)
	}
}

func TestUnattendedClaudeCodeHandler(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	handler := f.perms.options(f.session, f.dir, "cron", "").handler

	d, err := handler(context.Background(), llm.ClaudeCodePermissionRequest{ToolName: chatExitPlanModeTool, Input: json.RawMessage(`{"plan":"x"}`)})
	if err != nil || d.Allow || d.Message != unattendedPlanMessage {
		t.Fatalf("plan approval: %+v %v", d, err)
	}

	f.answerNext(t, true)
	d, err = handler(context.Background(), llm.ClaudeCodePermissionRequest{ToolName: "Bash", Input: json.RawMessage(`{"command":"npm test"}`), DecisionReason: "not in allow list"})
	if err != nil || !d.Allow {
		t.Fatalf("bash approved: %+v %v", d, err)
	}
	items, _ := f.ops.ListApprovals()
	if len(items) != 1 || items[0].ToolPermission.Preview != "npm test" || items[0].ToolPermission.Reason != "not in allow list" {
		t.Fatalf("queued: %+v", items[0].ToolPermission)
	}
}

func TestSubagentRunOptionsUseTheParentSession(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	var seen unattendedRunOptions
	runner := withSubagentPermissions(func(ctx context.Context, label, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
		seen = subagentRunOptions(ctx, f.dir, label)
		return "ok", nil
	}, f.perms)
	ctx := agentruntime.WithParentSession(context.Background(), f.session)
	if _, err := runner(ctx, "subagent:1", "go", nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if seen.authorizer == nil {
		t.Fatal("a subagent of a manual session should be gated")
	}
	if _, err := runner(context.Background(), "cron:1", "go", nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if seen.authorizer != nil {
		t.Fatal("a run without a parent session should not be gated")
	}
	if withSubagentPermissions(nil, f.perms) != nil {
		t.Fatal("nil runner should stay nil")
	}
	if got := subagentRunOptions(context.Background(), f.dir, "x"); got.authorizer != nil {
		t.Fatal("unwrapped runner should not be gated")
	}
}

func TestPermissionInputPreview(t *testing.T) {
	cases := map[string]string{
		`{"command":" ls -la "}`:      "ls -la",
		`{"file_path":"/tmp/a.go"}`:   "/tmp/a.go",
		`{"url":"https://x.test"}`:    "https://x.test",
		`{"other":1}`:                 `{"other":1}`,
		`not json`:                    "not json",
		`{"command":"","path":"p"}`:   "p",
		`{"notebook_path":"n.ipynb"}`: "n.ipynb",
	}
	for input, want := range cases {
		if got := permissionInputPreview([]byte(input)); got != want {
			t.Errorf("preview(%s) = %q, want %q", input, got, want)
		}
	}
	long := strings.Repeat("é", 300)
	got := permissionInputPreview([]byte(long))
	if !strings.HasSuffix(got, "…") || len(got) > 404 {
		t.Fatalf("long preview len %d", len(got))
	}
}

func TestOpsHandlerReviewsToolPermissions(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	var emitted []notificationEvent
	handler := newOpsAPIHandler(f.ops, zerolog.Nop(), func(_ context.Context, evt notificationEvent) { emitted = append(emitted, evt) }, f.store)
	approval, err := f.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: f.session, ToolName: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	post := func(path string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		return rec.Code
	}
	if code := post("/v1/ops/approvals/" + approval.ID + "/apply"); code != http.StatusNotFound {
		t.Fatalf("unknown action = %d", code)
	}
	if code := post("/v1/ops/approvals/" + approval.ID + "/approve"); code != http.StatusOK {
		t.Fatalf("approve = %d", code)
	}
	if code := post("/v1/ops/approvals/" + approval.ID + "/reject"); code != http.StatusConflict {
		t.Fatalf("second review = %d", code)
	}
	if got, _ := f.ops.GetApproval(approval.ID); got.Status != ops.ApprovalStatusApproved {
		t.Fatalf("status = %s", got.Status)
	}
	if len(emitted) != 1 || emitted[0].Title != "Tool call approved" {
		t.Fatalf("emitted = %+v", emitted)
	}
	rejected, _ := f.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: f.session, ToolName: "exec"})
	if code := post("/v1/ops/approvals/" + rejected.ID + "/reject"); code != http.StatusOK {
		t.Fatalf("reject = %d", code)
	}
	if len(emitted) != 2 || emitted[1].Title != "Tool call rejected" {
		t.Fatalf("emitted = %+v", emitted)
	}
}

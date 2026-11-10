package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

// The harness proposal's server side (tars#1291, docs/decisions/focus-mode.md
// §5.2). A pipeline created with harness_proposal: true remembers its
// repository (HarnessOptIn.Repo). When it finishes with its work merged
// (focusDriver.pipelineFinished), the driver — in the background, never
// holding the session — collects the pipeline's facts
// (focuspipeline.CollectHarnessEvidence). With none, nothing else happens.
// Otherwise one decision-only call on the focus_harness role reads them, the
// repository's harness (focusprobe.ReadHarnessFiles) and its harness library,
// and proposes edits; the server keeps only those that cite the evidence
// (focuspipeline.ValidateHarnessLessons) and opens them as a new development
// pipeline on the repository, through the same path POST /v1/focus/pipelines
// takes. That pipeline's own gates — a person approving its plan and merging
// its pull request — decide whether the harness changes; the server never
// writes a harness file. Its lessons go to the library as proposed, and to
// EXPERIENCE.md as adopted when its pull request merges.
//
// The model call is tagged with its own usage source (focus_harness) and no
// session, and the proposal pipeline starts without goal mode: none of it is
// drawn from the finished pipeline's goal mode budget or counted as its
// session's cost.

// focusHarnessSource names the proposal's model call in usage and the audit.
const focusHarnessSource = "focus_harness"

// focusHarnessTimeout bounds one proposal: the model call, the re-ask and
// opening the pipeline (a variable for tests).
var focusHarnessTimeout = 10 * time.Minute

// focusHarnessLLM asks the focus_harness role one decision-only question and
// returns its reply.
type focusHarnessLLM func(ctx context.Context, prompt string) (string, error)

// focusHarnessReask asks once more for the JSON a reply lacked.
const focusHarnessReask = "\n\nYour previous reply was not the JSON object asked for (%s). Reply again with the JSON object only."

// newFocusHarnessLLM resolves the focus_harness role on every call (the
// configuration may change) and refuses a provider that cannot keep the
// call free of tools. nil without a router.
func newFocusHarnessLLM(router llm.Router) focusHarnessLLM {
	if router == nil {
		return nil
	}
	return func(ctx context.Context, prompt string) (string, error) {
		client, resolution, err := router.ClientFor(llm.RoleFocusHarness)
		if err != nil {
			return "", err
		}
		if !llm.SupportsDecisionOnly(resolution.Provider) {
			return "", fmt.Errorf("the focus_harness role runs on %s, which cannot keep a call free of tools", resolution.Provider)
		}
		ctx = usage.WithCallMeta(ctx, usage.CallMeta{Source: focusHarnessSource})
		resp, err := client.Chat(ctx, []llm.ChatMessage{{Role: "user", Content: prompt}}, llm.DecisionOnlyChatOptions())
		if err != nil {
			return "", err
		}
		if llm.AttemptedTools(resp) {
			return "", errors.New("the reply tried to use a tool")
		}
		return resp.Message.Content, nil
	}
}

// focusHarnessCost is the session's recorded cost this month (0 when
// unknown), for the evidence's context.
func focusHarnessCost(tracker *usage.Tracker) func(sessionID string) float64 {
	if tracker == nil {
		return nil
	}
	return func(sessionID string) float64 {
		summary, err := tracker.SummaryFiltered("month", "provider", usage.SummaryFilter{SessionID: sessionID})
		if err != nil {
			return 0
		}
		return summary.TotalCostUSD
	}
}

// checkHarness validates a create request's harness_proposal and returns the
// setting to store (nil when not asked for). A status of 0 is valid.
func (d *focusDriver) checkHarness(ctx context.Context, req focusCreateRequest, kind string) (*focuspipeline.HarnessOptIn, int, error) {
	if !req.HarnessProposal {
		return nil, 0, nil
	}
	if kind == focuspipeline.KindRelease {
		return nil, http.StatusBadRequest, errors.New("harness_proposal: a release pipeline proposes no harness changes")
	}
	if d == nil || d.harnessLLM == nil || d.openPipeline == nil {
		return nil, http.StatusServiceUnavailable, errors.New("harness proposals are unavailable")
	}
	repo := ""
	if dir := expandCwdHome(strings.TrimSpace(req.Cwd)); dir != "" {
		repo = focusprobe.MainCheckout(ctx, dir)
	}
	if repo == "" {
		return nil, http.StatusBadRequest, errors.New("harness_proposal: the folder is not in a git repository")
	}
	return &focuspipeline.HarnessOptIn{Repo: repo}, 0, nil
}

// harnessAfterFinish runs when a pipeline has just finished: a pipeline that
// opted in gets its proposal (in the background), and a proposal pipeline
// whose pull request merged has its lessons adopted in the library.
func (d *focusDriver) harnessAfterFinish(sessionID string, p focuspipeline.Pipeline) {
	if focuspipeline.HarnessDue(p) {
		d.goHarness(sessionID)
	}
	if from := p.HarnessFrom; from != nil && p.PR != nil && p.PR.State == focuspipeline.PRStateMerged {
		lib, err := focuspipeline.OpenHarnessLibrary(d.sharedDir(), from.Repo)
		if err != nil {
			return
		}
		n, err := lib.Adopt(sessionID, p.PR.URL, d.now())
		if err != nil {
			d.logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: adopt harness lessons failed")
			return
		}
		if n > 0 {
			d.auditHarness(from.SessionID, "adopted", map[string]any{"proposal": sessionID, "lessons": n, "pr_url": p.PR.URL})
		}
	}
}

// sharedDir is <workspace>/_shared, where the harness library lives.
func (d *focusDriver) sharedDir() string {
	if d.sessions == nil {
		return ""
	}
	return filepath.Join(d.sessions.WorkspaceDir(), "_shared")
}

// goHarness runs the session's harness proposal in the background, once at
// a time per session.
func (d *focusDriver) goHarness(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ctx.Err() != nil || d.harnessBusy[sessionID] {
		return
	}
	if d.harnessBusy == nil {
		d.harnessBusy = map[string]bool{}
	}
	d.harnessBusy[sessionID] = true
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.harnessBusy, sessionID)
			d.mu.Unlock()
		}()
		d.runHarness(sessionID)
	}()
}

// resumeHarness starts the proposals a restart cut off: pipelines finished
// merged, opted in, with no outcome yet.
func (d *focusDriver) resumeHarness() int {
	store := focusStoreFor(d.sessions)
	if store == nil {
		return 0
	}
	list, err := store.List()
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range list {
		if focuspipeline.HarnessDue(p) {
			d.goHarness(p.SessionID)
			n++
		}
	}
	return n
}

// runHarness collects the pipeline's evidence and, when there is some, asks
// for lessons and opens the proposal pipeline. Every way it ends is recorded
// on the pipeline once (focuspipeline.RecordHarnessOutcome).
func (d *focusDriver) runHarness(sessionID string) {
	log := d.logger.With().Str("session_id", sessionID).Logger()
	p, found, err := focusStoreFor(d.sessions).Get(sessionID)
	if err != nil || !found || !focuspipeline.HarnessDue(p) {
		return
	}
	ctx, cancel := context.WithTimeout(d.ctx, focusHarnessTimeout)
	defer cancel()
	outcome, proposal, details := d.proposeHarness(ctx, p, log)
	if d.ctx.Err() != nil {
		return // shutting down: resumeHarness runs it again
	}
	updated, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.RecordHarnessOutcome(p, outcome, proposal, details, d.now()), nil
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: record harness proposal failed")
		return
	}
	log.Info().Str("outcome", outcome).Str("proposal", proposal).Msg("focus: harness proposal done")
	d.auditHarness(sessionID, outcome, details)
	if outcome == focuspipeline.HarnessProposed && d.notify != nil {
		evt := newNotificationEvent(focusNotifyCategory, "info", "Harness improvement proposed", updated.Goal)
		evt.SessionID = proposal
		evt.OpenPath = "/console/focus/" + proposal
		d.notify(d.ctx, evt)
	}
}

// proposeHarness is runHarness's work outside the pipeline store: the
// outcome, the proposal pipeline's session, and the outcome's details
// (counts and reason codes only — no model text, no error text).
func (d *focusDriver) proposeHarness(ctx context.Context, p focuspipeline.Pipeline, log zerolog.Logger) (string, string, map[string]any) {
	ev := focuspipeline.CollectHarnessEvidence(p)
	if ev.Empty() {
		return focuspipeline.HarnessNoEvidence, "", nil
	}
	details := map[string]any{"evidence": len(ev.Items)}
	if d.harnessLLM == nil || d.openPipeline == nil {
		details["reason"] = "unavailable"
		return focuspipeline.HarnessUnavailable, "", details
	}
	if d.harnessCost != nil {
		ev.CostUSD = d.harnessCost(p.SessionID)
	}
	lib, err := focuspipeline.OpenHarnessLibrary(d.sharedDir(), p.Harness.Repo)
	if err != nil {
		details["reason"] = "no_repository"
		return focuspipeline.HarnessUnavailable, "", details
	}
	known, lines, err := lib.Known()
	if err != nil {
		log.Warn().Err(err).Msg("focus: read harness library failed")
		details["reason"] = "library"
		return focuspipeline.HarnessFailed, "", details
	}
	prompt := focuspipeline.HarnessLessonPrompt(ev, focusprobe.ReadHarnessFiles(p.Harness.Repo), lines)
	edits, err := d.harnessEdits(ctx, prompt)
	if err != nil {
		log.Info().Err(err).Msg("focus: harness lessons not received")
		details["reason"] = "model"
		if errors.Is(err, focuspipeline.ErrInvalidHarnessReply) {
			details["reason"] = "invalid_reply"
		}
		return focuspipeline.HarnessFailed, "", details
	}
	kept, dropped := focuspipeline.ValidateHarnessLessons(edits, ev, known)
	details["proposed"], details["dropped"] = len(edits), len(dropped)
	if len(kept) == 0 {
		return focuspipeline.HarnessNoEdits, "", details
	}
	proposal, err := d.openPipeline(ctx, focusCreateRequest{
		Goal: focuspipeline.HarnessProposalGoal(ev), Kickoff: focuspipeline.HarnessKickoff(ev, kept),
		Cwd: p.Harness.Repo, Isolate: true, Start: true,
		harnessFrom: &focuspipeline.HarnessSource{SessionID: p.SessionID, Repo: p.Harness.Repo},
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: open harness proposal pipeline failed")
		details["reason"] = "open"
		return focuspipeline.HarnessFailed, "", details
	}
	if _, err := lib.RecordProposed(ev, proposal, kept, d.now()); err != nil {
		log.Warn().Err(err).Msg("focus: record proposed harness lessons failed")
	}
	details["lessons"] = len(kept)
	return focuspipeline.HarnessProposed, proposal, details
}

// harnessEdits asks for the edits, and once more when the reply is not the
// JSON asked for.
func (d *focusDriver) harnessEdits(ctx context.Context, prompt string) ([]focuspipeline.HarnessEdit, error) {
	reply, err := d.harnessLLM(ctx, prompt)
	if err != nil {
		return nil, err
	}
	edits, _, err := focuspipeline.ParseHarnessReply(reply)
	if err == nil {
		return edits, nil
	}
	if reply, err = d.harnessLLM(ctx, prompt+fmt.Sprintf(focusHarnessReask, err)); err != nil {
		return nil, err
	}
	edits, _, err = focuspipeline.ParseHarnessReply(reply)
	return edits, err
}

// auditHarness records a proposal step in the ops automation audit.
func (d *focusDriver) auditHarness(sessionID, result string, details map[string]any) {
	if d.audit == nil {
		return
	}
	d.audit(ops.AutomationAuditEntry{
		Actor: focusHarnessSource, Action: "focus_harness_proposal", SessionID: sessionID, Result: result, Details: details,
	})
}

// focusOpenPipeline binds the driver's way of opening a pipeline to the
// focus API's create path, run as the admin (the server's own request).
func focusOpenPipeline(api *focusAPI) func(ctx context.Context, req focusCreateRequest) (string, error) {
	return func(ctx context.Context, req focusCreateRequest) (string, error) {
		status, resp := api.open(ctx, req, serverauth.RoleAdmin)
		if status != http.StatusCreated {
			msg, _ := resp["error"].(string)
			return "", fmt.Errorf("open pipeline: %d %s", status, msg)
		}
		id, _ := resp["session_id"].(string)
		return id, nil
	}
}

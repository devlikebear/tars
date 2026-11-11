package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/execsandbox"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/rs/zerolog"
)

// Verify promotion's server side (tars#1292, docs/decisions/focus-mode.md
// §5.4). A pipeline created with verify_promotion: true remembers its
// repository (PromotionOptIn.Repo). When it finishes with its work merged
// (focusDriver.pipelineFinished), the driver — in the background, never
// holding the session — records the recurrence keys of its facts in the
// repository's ledger, next to the harness library. Only when a key now
// recurs in focuspipeline.PromotionThreshold of the last
// focuspipeline.PromotionWindow recorded pipelines does anything else
// happen: a failed verify command is its own check; for any other key one
// decision-only call on the focus_harness role proposes a check, and the
// server runs it on the work tree before and after the turn that fixed the
// defect in this pipeline (its checkpoints), sandboxed in a temporary clone
// (focusprobe.Replay). A check that does not fail before and pass after is
// dropped with a reason code. Survivors open as a new development pipeline
// on the repository — the same path POST /v1/focus/pipelines takes — that
// adds the checks and lists their commands in the repository's
// .tars/verify.json; a person merges it or not. The checks go to the
// harness library as proposed, and enforced when that pull request merges
// (harnessAfterFinish's adoption: the pipeline is marked harness_from).

// focusPromotionSource names the promotion in the audit.
const focusPromotionSource = "focus_verify_promotion"

// focusPromotionTimeout bounds one promotion: the model call and its
// re-ask, the replays (two sandboxed runs per check) and opening the
// pipeline. A variable for tests.
var focusPromotionTimeout = 30 * time.Minute

// focusPromotionReask asks once more for the JSON a reply lacked.
const focusPromotionReask = "\n\nYour previous reply was not the JSON object asked for (%s). Reply again with the JSON object only."

// focusPromotion is what a verify promotion needs beyond the harness
// proposal's model and pipeline opener: the session's checkpoints (to find
// the fix turn and export its two sides) and the sandboxed runner. Without
// a runner — no OS sandbox on this host — pipelines cannot opt in: a check
// a model wrote never runs unsandboxed.
type focusPromotion struct {
	run    focusprobe.ReplayRunner
	turns  func(sessionID string) ([]checkpoint.Entry, error)
	export func(ctx context.Context, sessionID, turnID, side, dest string) error
	// busy holds the sessions whose promotion runs now (under the
	// driver's mu).
	busy map[string]bool
}

// focusPromotionProbeTimeout bounds the sandbox probe at start-up.
const focusPromotionProbeTimeout = 10 * time.Second

// newFocusPromotion binds verify promotion to the checkpoint store and the
// host's OS sandbox; nil when either is missing.
func newFocusPromotion(store *checkpoint.Store, logger zerolog.Logger) *focusPromotion {
	if store == nil {
		return nil
	}
	backend, err := execsandbox.Detect()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), focusPromotionProbeTimeout)
		err = backend.Probe(ctx)
		cancel()
	}
	if err != nil {
		logger.Debug().Err(err).Msg("focus: verify promotion unavailable without the OS sandbox")
		return nil
	}
	return &focusPromotion{run: focusprobe.SandboxRunner(backend), turns: store.List, export: store.ExportTurn}
}

// checkPromotion validates a create request's verify_promotion and returns
// the setting to store (nil when not asked for). A status of 0 is valid.
func (d *focusDriver) checkPromotion(ctx context.Context, req focusCreateRequest, kind string) (*focuspipeline.PromotionOptIn, int, error) {
	if !req.VerifyPromotion {
		return nil, 0, nil
	}
	if kind == focuspipeline.KindRelease {
		return nil, http.StatusBadRequest, errors.New("verify_promotion: a release pipeline promotes no checks")
	}
	if d == nil || d.harnessLLM == nil || d.openPipeline == nil || d.promotion == nil || d.promotion.run == nil {
		return nil, http.StatusServiceUnavailable, errors.New("verify promotion is unavailable (it needs the focus_harness role, checkpoints and the OS sandbox)")
	}
	repo := ""
	if dir := expandCwdHome(strings.TrimSpace(req.Cwd)); dir != "" {
		repo = focusprobe.MainCheckout(ctx, dir)
	}
	if repo == "" {
		return nil, http.StatusBadRequest, errors.New("verify_promotion: the folder is not in a git repository")
	}
	return &focuspipeline.PromotionOptIn{Repo: repo}, 0, nil
}

// promotionAfterFinish starts the verify promotion of a pipeline that just
// finished merged and opted in.
func (d *focusDriver) promotionAfterFinish(sessionID string, p focuspipeline.Pipeline) {
	if focuspipeline.PromotionDue(p) {
		d.goPromotion(sessionID)
	}
}

// goPromotion runs the session's promotion in the background, once at a
// time per session.
func (d *focusDriver) goPromotion(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.promotion == nil || d.ctx.Err() != nil || d.promotion.busy[sessionID] {
		return
	}
	if d.promotion.busy == nil {
		d.promotion.busy = map[string]bool{}
	}
	d.promotion.busy[sessionID] = true
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.promotion.busy, sessionID)
			d.mu.Unlock()
		}()
		d.runPromotion(sessionID)
	}()
}

// resumePromotions starts the promotions a restart cut off.
func (d *focusDriver) resumePromotions() int {
	store := focusStoreFor(d.sessions)
	if store == nil || d.promotion == nil {
		return 0
	}
	list, err := store.List()
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range list {
		if focuspipeline.PromotionDue(p) {
			d.goPromotion(p.SessionID)
			n++
		}
	}
	return n
}

// runPromotion runs the promotion and records how it ended on the
// pipeline, once.
func (d *focusDriver) runPromotion(sessionID string) {
	log := d.logger.With().Str("session_id", sessionID).Logger()
	p, found, err := focusStoreFor(d.sessions).Get(sessionID)
	if err != nil || !found || !focuspipeline.PromotionDue(p) {
		return
	}
	ctx, cancel := context.WithTimeout(d.ctx, focusPromotionTimeout)
	defer cancel()
	outcome, proposal, details := d.proposePromotion(ctx, p, log)
	if d.ctx.Err() != nil {
		return // shutting down: resumePromotions runs it again
	}
	updated, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.RecordPromotionOutcome(p, outcome, proposal, details, d.now()), nil
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: record verify promotion failed")
		return
	}
	log.Info().Str("outcome", outcome).Str("proposal", proposal).Msg("focus: verify promotion done")
	d.auditPromotion(sessionID, outcome, details)
	if outcome == focuspipeline.PromotionProposed && d.notify != nil {
		evt := newNotificationEvent(focusNotifyCategory, "info", "Verify checks proposed", updated.Goal)
		evt.SessionID = proposal
		evt.OpenPath = "/console/focus/" + proposal
		d.notify(d.ctx, evt)
	}
}

// proposePromotion is runPromotion's work outside the pipeline store: the
// outcome, the promotion pipeline's session, and the details (counts and
// reason codes — never model text or error text).
func (d *focusDriver) proposePromotion(ctx context.Context, p focuspipeline.Pipeline, log zerolog.Logger) (string, string, map[string]any) {
	ev := focuspipeline.CollectHarnessEvidence(p)
	hits := focuspipeline.RecurrenceHits(ev)
	details := map[string]any{"hits": len(hits)}
	lib, err := focuspipeline.OpenHarnessLibrary(d.sharedDir(), p.Promotion.Repo)
	if err != nil {
		details["reason"] = "no_repository"
		return focuspipeline.PromotionUnavailable, "", details
	}
	source := focuspipeline.RecurrencePipeline{
		SessionID: p.SessionID, Goal: ev.Goal, PRURL: ev.PRURL, RecordedAt: d.now().UTC(), Hits: hits,
	}
	ledger, err := lib.RecordRecurrence(source)
	if err != nil {
		log.Warn().Err(err).Msg("focus: record recurrence failed")
		details["reason"] = "ledger"
		return focuspipeline.PromotionFailed, "", details
	}
	exclude, err := lib.CheckRecurrences(false)
	if err != nil {
		log.Warn().Err(err).Msg("focus: read harness library failed")
		details["reason"] = "library"
		return focuspipeline.PromotionFailed, "", details
	}
	cands := focuspipeline.RecurrenceCandidates(ledger, hits, exclude)
	if len(cands) == 0 {
		return focuspipeline.PromotionNoCandidates, "", details
	}
	details["candidates"] = len(cands)
	if d.promotion == nil || d.promotion.run == nil || d.openPipeline == nil {
		details["reason"] = "unavailable"
		return focuspipeline.PromotionUnavailable, "", details
	}
	checks, reasons := d.promotionChecks(ctx, p, cands, hits, log)
	if len(reasons) > 0 {
		details["reasons"] = reasons
		if err := lib.RecordPromotionAttempts(cands, reasons, d.now()); err != nil {
			log.Warn().Err(err).Msg("focus: record promotion attempts failed")
		}
	}
	if len(checks) == 0 {
		return focuspipeline.PromotionNoChecks, "", details
	}
	proposal, err := d.openPipeline(ctx, focusCreateRequest{
		Goal: focuspipeline.PromotionGoal(checks), Kickoff: focuspipeline.PromotionKickoff(checks),
		Cwd: p.Promotion.Repo, Isolate: true, Start: true,
		// Marked as a harness proposal: when its pull request merges,
		// harnessAfterFinish adopts its checks as enforced.
		harnessFrom: &focuspipeline.HarnessSource{SessionID: p.SessionID, Repo: p.Promotion.Repo},
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: open verify promotion pipeline failed")
		details["reason"] = "open"
		return focuspipeline.PromotionFailed, "", details
	}
	if _, err := lib.RecordChecks(source, proposal, checks, d.now()); err != nil {
		log.Warn().Err(err).Msg("focus: record promoted checks failed")
	}
	details["checks"] = len(checks)
	return focuspipeline.PromotionProposed, proposal, details
}

// promotionChecks turns the candidates into confirmed checks: a failed
// verify command is its own check; for the others the model proposes one
// and the server replays it. reasons holds, per candidate key that ended
// without a check, why (a reason code).
func (d *focusDriver) promotionChecks(ctx context.Context, p focuspipeline.Pipeline, cands []focuspipeline.RecurrenceCandidate, hits []focuspipeline.RecurrenceHit, log zerolog.Logger) ([]focuspipeline.VerifiedCheck, map[string]string) {
	reasons := map[string]string{}
	var checks []focuspipeline.VerifiedCheck
	var asked []focuspipeline.RecurrenceCandidate
	for _, c := range cands {
		if c.Kind == focuspipeline.RecurrenceVerify {
			checks = append(checks, focuspipeline.RecordCheck(c))
			continue
		}
		asked = append(asked, c)
	}
	if len(asked) == 0 {
		return checks, reasons
	}
	fail := func(reason string) {
		for _, c := range asked {
			reasons[c.Key] = reason
		}
	}
	if d.harnessLLM == nil {
		fail("unavailable")
		return checks, reasons
	}
	prompt := focuspipeline.PromotionPrompt(asked, p.RepoVerify, focusprobe.ReadPromotionContext(p.Promotion.Repo))
	proposed, err := d.askPromotionChecks(ctx, prompt)
	if err != nil {
		log.Info().Err(err).Msg("focus: promotion checks not received")
		reason := "model"
		if errors.Is(err, focuspipeline.ErrInvalidPromotionReply) {
			reason = "invalid_reply"
		}
		fail(reason)
		return checks, reasons
	}
	kept, dropped := focuspipeline.ValidatePromotionChecks(proposed, asked)
	for _, drop := range dropped {
		if _, ok := reasons[drop.Key]; !ok && drop.Key != "" {
			reasons[drop.Key] = drop.Reason
		}
	}
	byKey := map[string]focuspipeline.PromotionCheck{}
	for _, c := range kept {
		byKey[c.Key] = c
	}
	for _, c := range asked {
		check, ok := byKey[c.Key]
		if !ok {
			if _, has := reasons[c.Key]; !has {
				reasons[c.Key] = focuspipeline.PromotionNoCheck
			}
			continue
		}
		delete(reasons, c.Key) // an invalid duplicate does not outweigh a valid check
		verified, reason := d.replayPromotionCheck(ctx, p, c, check, hits)
		if reason != focusprobe.ReplayVerified {
			reasons[c.Key] = reason
			continue
		}
		checks = append(checks, verified)
	}
	return checks, reasons
}

// askPromotionChecks asks for the checks, and once more when the reply is
// not the JSON asked for.
func (d *focusDriver) askPromotionChecks(ctx context.Context, prompt string) ([]focuspipeline.PromotionCheck, error) {
	reply, err := d.harnessLLM(ctx, prompt)
	if err != nil {
		return nil, err
	}
	checks, err := focuspipeline.ParsePromotionReply(reply)
	if err == nil {
		return checks, nil
	}
	if reply, err = d.harnessLLM(ctx, prompt+fmt.Sprintf(focusPromotionReask, err)); err != nil {
		return nil, err
	}
	return focuspipeline.ParsePromotionReply(reply)
}

// replayPromotionCheck runs a proposed check on the two sides of the turn
// that fixed the candidate's defect in this pipeline: the first turn with
// recorded changes that started after the defect's card. It returns the
// confirmed check, or why it was not confirmed.
func (d *focusDriver) replayPromotionCheck(ctx context.Context, p focuspipeline.Pipeline, c focuspipeline.RecurrenceCandidate, check focuspipeline.PromotionCheck, hits []focuspipeline.RecurrenceHit) (focuspipeline.VerifiedCheck, string) {
	var cardAt time.Time
	for _, h := range hits {
		if h.Key != c.Key {
			continue
		}
		for _, card := range p.Cards {
			if card.ID == h.CardID {
				cardAt = card.CreatedAt
			}
		}
	}
	if cardAt.IsZero() || d.promotion.turns == nil || d.promotion.export == nil {
		return focuspipeline.VerifiedCheck{}, focuspipeline.PromotionNoReplay
	}
	entries, err := d.promotion.turns(p.SessionID)
	if err != nil {
		return focuspipeline.VerifiedCheck{}, focuspipeline.PromotionNoReplay
	}
	turn, ok := fixTurnAfter(entries, cardAt)
	if !ok {
		return focuspipeline.VerifiedCheck{}, focuspipeline.PromotionNoReplay
	}
	side := func(name string) focusprobe.ReplayState {
		return func(ctx context.Context, dest string) error {
			return d.promotion.export(ctx, p.SessionID, turn.TurnID, name, dest)
		}
	}
	files := make([]focusprobe.ReplayFile, 0, len(check.Files))
	for _, f := range check.Files {
		files = append(files, focusprobe.ReplayFile{Path: f.Path, Content: f.Content})
	}
	res := focusprobe.Replay(ctx, focusprobe.ReplayRequest{
		Repo: p.Promotion.Repo, Base: p.BaseCommit,
		Defect: side(checkpoint.SideStart), Fix: side(checkpoint.SideEnd),
		Files: files, Command: check.Command, Run: d.promotion.run,
	})
	if res.Reason != focusprobe.ReplayVerified {
		return focuspipeline.VerifiedCheck{}, res.Reason
	}
	return focuspipeline.VerifiedCheck{
		PromotionCheck: check, Count: c.Count, Sessions: c.Sessions, Hits: c.Hits,
		VerifiedBy: focuspipeline.VerifiedByReplay, DefectExit: res.Defect.ExitCode, FixExit: res.Fix.ExitCode,
	}, focusprobe.ReplayVerified
}

// fixTurnAfter is the turn that fixed a defect recorded at at: the first
// checkpointed turn that started after it and changed something.
func fixTurnAfter(entries []checkpoint.Entry, at time.Time) (checkpoint.Entry, bool) {
	for _, e := range entries {
		if e.StartedAt.Before(at) || e.Skipped != "" || e.Start == "" || e.End == "" || e.Files == 0 {
			continue
		}
		return e, true
	}
	return checkpoint.Entry{}, false
}

// auditPromotion records a promotion step in the ops automation audit.
func (d *focusDriver) auditPromotion(sessionID, result string, details map[string]any) {
	if d.audit == nil {
		return
	}
	d.audit(ops.AutomationAuditEntry{
		Actor: focusPromotionSource, Action: focusPromotionSource, SessionID: sessionID, Result: result, Details: details,
	})
}

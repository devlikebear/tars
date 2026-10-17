package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

// The second opinion review's server side (docs/decisions/focus-mode.md
// §5.1). A pipeline created with second_opinion: {tier} has the first round
// of each review-kind stage reviewed again on that tier: when the stage's
// review turn has reported, the pipeline asks for ActionSecondOpinion, and
// the driver runs one read-only turn in the pipeline's hidden worker session
// — plan permission mode, isolation off, the tier pinned — with the diff the
// server read from git, never the first review's findings. Its
// <focus-findings> go back as EventSecondOpinion; a reply without the block
// is asked for it once more. A failure (timeout, error, no block, a tier
// that no longer resolves) is a result too: one notice card, and the round
// goes on without it.
//
// The step runs inside the driver's run and claims the pipeline's session
// like verification does: the session shows as running, a person's turn
// preempts it (human input wins), and POST /v1/chat/cancel stops it. The
// turn itself runs on the worker session, so it never holds the pipeline
// session's own chat turn.

// focusSecondOpinionSource names second opinion turns in the ops approval
// queue and the audit.
const focusSecondOpinionSource = "focus_second_opinion"

// focusSecondOpinionTimeout bounds one second opinion review, the request
// for a missing block included (a variable for tests).
var focusSecondOpinionTimeout = 10 * time.Minute

// focusSecondOpinionPhase is the focus_progress phase of the step.
const focusSecondOpinionPhase = "second_opinion"

// focusSecondOpinionSameModel is the warning code of a second opinion tier
// that resolves to the chat's own provider and model.
const focusSecondOpinionSameModel = "same_model_as_chat"

// focusSecondOpinionTurnRunner runs one second opinion turn on the worker
// session and returns the reply.
type focusSecondOpinionTurnRunner func(ctx context.Context, sessionID, prompt string) (string, error)

// focusSecondOpinionTiers checks a second opinion tier against the
// configuration: an error when it cannot be used, and whether it resolves
// to the chat's own provider and model.
type focusSecondOpinionTiers func(tier string) (sameAsChat bool, err error)

// newFocusSecondOpinionTiers checks tiers against cfg: the tier must be one
// a session can pin (llm.ParseTier), resolve, and run on a provider that can
// keep a turn read-only (antigravity-cli cannot, as for computer use and
// initiative). sameAsChat compares the provider alias and model with the
// chat's tier (chat_main, else the default tier).
func newFocusSecondOpinionTiers(cfg config.Config) focusSecondOpinionTiers {
	return func(tier string) (bool, error) {
		parsed, err := llm.ParseTier(tier)
		if err != nil {
			return false, err
		}
		resolved, err := config.ResolveLLMTier(&cfg, parsed.String())
		if err != nil {
			return false, fmt.Errorf("tier %s: %w", parsed, err)
		}
		if !llm.SupportsDecisionOnly(resolved.Kind) {
			return false, fmt.Errorf("tier %s runs on %s, which cannot keep a review read-only", parsed, resolved.Kind)
		}
		chatTier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleChatMain)])
		if chatTier == "" {
			chatTier = strings.TrimSpace(cfg.LLMDefaultTier)
		}
		if chatTier == "" {
			return false, nil
		}
		chat, err := config.ResolveLLMTier(&cfg, chatTier)
		if err != nil {
			return false, nil
		}
		return chat.ProviderAlias == resolved.ProviderAlias && chat.Model == resolved.Model, nil
	}
}

// focusSecondOpinionRequest is the create request's second_opinion field.
type focusSecondOpinionRequest struct {
	Tier string `json:"tier"`
}

// checkSecondOpinion validates a create request's second opinion and
// returns the setting to store and the warning code ("" for none). A status
// of 0 is valid.
func (d *focusDriver) checkSecondOpinion(req *focusSecondOpinionRequest) (*focuspipeline.SecondOpinion, string, int, error) {
	if req == nil {
		return nil, "", 0, nil
	}
	if d == nil || d.secondOpinionTurn == nil || d.secondOpinionTiers == nil {
		return nil, "", http.StatusServiceUnavailable, errors.New("the second opinion review is unavailable")
	}
	tier, err := llm.ParseTier(req.Tier)
	if err != nil {
		return nil, "", http.StatusBadRequest, fmt.Errorf("second_opinion: %w", err)
	}
	same, err := d.secondOpinionTiers(tier.String())
	if err != nil {
		return nil, "", http.StatusBadRequest, fmt.Errorf("second_opinion: %w", err)
	}
	warning := ""
	if same {
		warning = focusSecondOpinionSameModel
	}
	return &focuspipeline.SecondOpinion{Tier: tier.String()}, warning, 0, nil
}

// runSecondOpinion runs the second opinion review the pipeline owes and
// applies its result. claimed is false when another turn held the session.
func (d *focusDriver) runSecondOpinion(run *focusRun, log zerolog.Logger) (focuspipeline.Action, bool) {
	none := focuspipeline.Action{Kind: focuspipeline.ActionNone}
	claim, release, ok := d.cancels.Claim(run.sessionID)
	if !ok {
		return none, false
	}
	defer release()
	claim.setCancel(run.cancel)
	store := focusStoreFor(d.sessions)
	p, found, err := store.Get(run.sessionID)
	if err != nil || !found || !p.Review.SecondOpinionOwed || p.SecondOpinion == nil {
		return none, true
	}

	feed, endFeed := d.feeds.begin(run.sessionID)
	defer endFeed()
	defer d.activity.begin(run.sessionID)()
	stream := newChatStreamWriter(discardResponseWriter{header: http.Header{}}, run.sessionID, log)
	stream.feed = feed
	stream.status("stream_open", "stream connected", "", "", "", "")
	stream.focusProgress(focusSecondOpinionPhase, p.SecondOpinion.Tier, 1, 1, nil)

	ctx, cancel := context.WithTimeout(run.ctx, focusSecondOpinionTimeout)
	result := d.secondOpinionResult(ctx, run.sessionID, p, log)
	cancel()
	if run.ctx.Err() != nil {
		// Stopped, preempted by a person's turn, or shut down: the round
		// still owes its second opinion (a restart interrupts it).
		stream.cancelled()
		return none, true
	}
	act := none
	updated, _, err := store.Update(run.sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, a, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventSecondOpinion, SecondOpinion: &result}, d.now())
		act = a
		return next, err
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: apply second opinion failed")
		return none, true
	}
	log.Info().Str("outcome", result.Outcome).Int("findings", len(result.Findings)).Msg("focus: second opinion review done")
	stream.pipeline(updated, focusNextPrompt(act))
	stream.focusDone()
	return act, true
}

// secondOpinionResult prepares the worker session, reads the diff, runs the
// turn (and the request for a missing block) and reads its findings. Git
// runs here, outside the pipeline store's lock.
func (d *focusDriver) secondOpinionResult(ctx context.Context, sessionID string, p focuspipeline.Pipeline, log zerolog.Logger) focuspipeline.SecondOpinionResult {
	unavailable := focuspipeline.SecondOpinionResult{Outcome: focuspipeline.SecondOpinionUnavailable}
	tier := p.SecondOpinion.Tier
	if d.secondOpinionTurn == nil {
		return unavailable
	}
	if d.secondOpinionTiers != nil {
		// The configuration may have changed since the pipeline started.
		if _, err := d.secondOpinionTiers(tier); err != nil {
			log.Info().Err(err).Msg("focus: second opinion tier unavailable")
			return unavailable
		}
	}
	parent, err := d.sessions.Get(sessionID)
	if err != nil {
		return unavailable
	}
	soID, err := d.secondOpinionSession(sessionID, parent, tier)
	if err != nil {
		log.Warn().Err(err).Msg("focus: prepare second opinion session failed")
		return unavailable
	}
	dir := strings.TrimSpace(parent.CurrentDir)
	diff, _ := focusprobe.ReviewDiffOf(ctx, dir, p.BaseCommit)
	reply, err := d.secondOpinionTurn(ctx, soID, focuspipeline.SecondOpinionPrompt(p, focusSecondOpinionChange(diff)))
	if err != nil {
		return focusSecondOpinionFailure(ctx, err, log)
	}
	blocks := focuspipeline.ParseBlocks(reply)
	if blocks.Findings == nil {
		if reply, err = d.secondOpinionTurn(ctx, soID, focuspipeline.SecondOpinionReask); err != nil {
			return focusSecondOpinionFailure(ctx, err, log)
		}
		if blocks = focuspipeline.ParseBlocks(reply); blocks.Findings == nil {
			return focuspipeline.SecondOpinionResult{Outcome: focuspipeline.SecondOpinionNoBlock}
		}
	}
	return focuspipeline.SecondOpinionResult{
		Outcome:  focuspipeline.SecondOpinionOK,
		Findings: focusprobe.FindingExcerpts(context.WithoutCancel(ctx), dir, p.BaseCommit, blocks.Findings),
	}
}

func focusSecondOpinionFailure(ctx context.Context, err error, log zerolog.Logger) focuspipeline.SecondOpinionResult {
	log.Info().Err(err).Msg("focus: second opinion turn ended without a review")
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return focuspipeline.SecondOpinionResult{Outcome: focuspipeline.SecondOpinionTimeout}
	}
	return focuspipeline.SecondOpinionResult{Outcome: focuspipeline.SecondOpinionError}
}

// focusSecondOpinionChange is the diff as the prompt shows it.
func focusSecondOpinionChange(d focusprobe.ReviewDiff) string {
	var b strings.Builder
	if d.Base == "" {
		b.WriteString("The server could not read the change from git: review the working tree of this folder against its last commit (`git diff HEAD`).\n")
		return b.String()
	}
	if d.Empty() {
		fmt.Fprintf(&b, "`git diff %s` shows no change.\n", d.Base)
		return b.String()
	}
	fmt.Fprintf(&b, "The change (`git diff %s`, committed and uncommitted):\n", d.Base)
	if d.Truncated {
		fmt.Fprintf(&b, "The diff is longer than %d KB and is cut below; every changed file:\n```\n%s\n```\n", focusprobe.ReviewDiffMaxBytes>>10, fenceSafe(d.Stat))
	}
	if strings.TrimSpace(d.Diff) != "" {
		fmt.Fprintf(&b, "```diff\n%s\n```\n", fenceSafe(d.Diff))
	}
	if d.Truncated {
		b.WriteString("(diff cut here — read the remaining files yourself)\n")
	}
	if len(d.Untracked) > 0 {
		b.WriteString("New files git does not track yet (not in the diff — read them yourself):\n")
		for _, f := range d.Untracked {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	return b.String()
}

// fenceSafe keeps text from closing the code fence it is quoted in.
func fenceSafe(s string) string {
	return strings.ReplaceAll(s, "```", "ˋˋˋ")
}

// secondOpinionSession returns the pipeline's second opinion session,
// creating it on first use, and points it at the folder, permission mode
// and tier the review needs now. Only the session's own driver run calls
// it, one at a time.
func (d *focusDriver) secondOpinionSession(pipelineID string, parent session.Session, tier string) (string, error) {
	store := focusStoreFor(d.sessions)
	p, _, err := store.Get(pipelineID)
	if err != nil {
		return "", err
	}
	soID := ""
	if p.SecondOpinion != nil {
		soID = strings.TrimSpace(p.SecondOpinion.SessionID)
	}
	if soID != "" {
		if _, err := d.sessions.Get(soID); err != nil {
			soID = ""
		}
	}
	if soID == "" {
		created, err := d.sessions.CreateWithOptions(focusSecondOpinionTitle(parent.Title), "worker", true)
		if err != nil {
			return "", err
		}
		soID = created.ID
		// Never isolated and never holding the repository: a read-only
		// review must not push the pipeline's own turns into a worktree.
		if err := d.sessions.SetIsolation(soID, session.IsolationOff); err != nil {
			return "", err
		}
		// Recorded at once (no session store call inside the update).
		if _, _, err := store.Update(pipelineID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
			if p.SecondOpinion != nil {
				p.SecondOpinion.SessionID = soID
			}
			return p, nil
		}); err != nil {
			_ = d.sessions.Delete(soID)
			return "", err
		}
	}
	if err := d.sessions.SetPermissionMode(soID, chatPermissionModePlan); err != nil {
		return "", err
	}
	if err := d.sessions.SetTierPin(soID, tier); err != nil {
		return "", err
	}
	if cwd := strings.TrimSpace(parent.CurrentDir); cwd != "" {
		dirs := append([]string(nil), parent.WorkDirs...)
		if !slices.Contains(dirs, cwd) {
			dirs = append(dirs, cwd)
		}
		if err := d.sessions.SetWorkDirs(soID, dirs, cwd); err != nil {
			return "", err
		}
	}
	return soID, nil
}

func focusSecondOpinionTitle(parent string) string {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return "Focus second opinion"
	}
	return "Second opinion · " + parent
}

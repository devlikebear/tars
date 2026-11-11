package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

// The execution tier's server side (docs/decisions/focus-mode.md §5.3,
// #1293). A pipeline created with exec_tier: heavy|standard|light sends
// its execution turns — build-kind stages and the fix turns of review and
// pr_review (focuspipeline.Pipeline.ExecTurn) — on that tier, for that turn
// only: the request carries the tier like CASE's question does, with no
// pin, so the session's tier_pin is never touched. Every other turn, and
// any turn a person types into the session, stays on the session's tier.
//
// A tier switch must not break a resumed CLI provider session: when the
// session's provider and the execution tier's are different kinds and one
// of them resumes an upstream session (claude-code-cli, antigravity-cli),
// the turn stays on the session's tier and the pipeline gets one notice.

// focusExecTierSource marks a turn the execution tier chose, in the turn's
// context info and the tier signal.
const focusExecTierSource = "focus_exec_tier"

// focusExecTierResumeWarning is the create response's warning code when
// the execution tier cannot take turns from the chat's tier as configured.
const focusExecTierResumeWarning = "resume_provider"

// focusExecReportDays is the report's default window, and the longest one.
const (
	focusExecReportDays    = 90
	focusExecReportMaxDays = 365
)

// focusTierKind resolves a tier to its provider kind; "" is the chat's own
// tier (chat_main, else the default tier).
type focusTierKind func(tier string) (string, error)

// newFocusTierKind resolves tiers against cfg.
func newFocusTierKind(cfg config.Config) focusTierKind {
	return func(tier string) (string, error) {
		tier = strings.TrimSpace(tier)
		if tier == "" {
			tier = strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleChatMain)])
		}
		if tier == "" {
			tier = strings.TrimSpace(cfg.LLMDefaultTier)
		}
		resolved, err := config.ResolveLLMTier(&cfg, tier)
		if err != nil {
			return "", fmt.Errorf("tier %s: %w", tier, err)
		}
		return resolved.Kind, nil
	}
}

// providerResumesUpstream reports whether a provider kind resumes an
// upstream session instead of replaying the transcript.
func providerResumesUpstream(kind string) bool { return initiativeResumeProviderKinds[kind] }

// checkExecTier validates a create request's exec_tier and returns the
// setting to store (nil when not asked for) and the warning code ("" for
// none). A status of 0 is valid.
func (d *focusDriver) checkExecTier(ctx context.Context, req focusCreateRequest, kind string) (*focuspipeline.ExecTier, string, int, error) {
	raw := strings.TrimSpace(req.ExecTier)
	if raw == "" {
		return nil, "", 0, nil
	}
	if kind == focuspipeline.KindRelease {
		return nil, "", http.StatusBadRequest, errors.New("exec_tier: a release pipeline runs on the session's tier")
	}
	if d == nil || d.tierKind == nil {
		return nil, "", http.StatusServiceUnavailable, errors.New("the execution tier is unavailable")
	}
	tier, err := llm.ParseTier(raw)
	if err != nil {
		return nil, "", http.StatusBadRequest, fmt.Errorf("exec_tier: %w", err)
	}
	execKind, err := d.tierKind(tier.String())
	if err != nil {
		return nil, "", http.StatusBadRequest, fmt.Errorf("exec_tier: %w", err)
	}
	warning := ""
	if chatKind, err := d.tierKind(""); err == nil && !focuspipeline.ExecTierSwitchSafe(chatKind, execKind, providerResumesUpstream) {
		warning = focusExecTierResumeWarning
	}
	et := &focuspipeline.ExecTier{Tier: tier.String(), At: d.now().UTC()}
	if dir := expandCwdHome(strings.TrimSpace(req.Cwd)); dir != "" {
		et.Repo = focusprobe.MainCheckout(ctx, dir)
	}
	et.Lessons = d.adoptedHarnessLessons(et.Repo)
	return et, warning, 0, nil
}

// adoptedHarnessLessons counts the adopted lessons in repo's harness
// library (0 when there is none or it cannot be read).
func (d *focusDriver) adoptedHarnessLessons(repo string) int {
	if repo == "" || d.sharedDir() == "" {
		return 0
	}
	lib, err := focuspipeline.OpenHarnessLibrary(d.sharedDir(), repo)
	if err != nil {
		return 0
	}
	lessons, err := lib.Lessons()
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range lessons {
		if l.Status == focuspipeline.HarnessLessonAdopted {
			n++
		}
	}
	return n
}

// execTurnTier is the tier the session's next focus turn runs on: the
// pipeline's execution tier for an execution turn it can safely switch to,
// else "" (the session's own tier). A switch it refuses leaves the
// pipeline's fallback notice once.
func (d *focusDriver) execTurnTier(sessionID string, log zerolog.Logger) string {
	if d == nil || d.tierKind == nil || d.sessions == nil {
		return ""
	}
	store := focusStoreFor(d.sessions)
	p, ok, err := store.Get(sessionID)
	if err != nil || !ok {
		return ""
	}
	tier := p.ExecTurnTier()
	if tier == "" {
		return ""
	}
	pin := ""
	if sess, err := d.sessions.Get(sessionID); err == nil {
		pin = sessionTierPin(sess, nil).String()
	}
	execKind, err := d.tierKind(tier)
	if err != nil {
		d.execTierFallback(sessionID, focuspipeline.ExecTierFallbackUnresolved, "", "", log)
		return ""
	}
	sessionKind, err := d.tierKind(pin)
	if err == nil && !focuspipeline.ExecTierSwitchSafe(sessionKind, execKind, providerResumesUpstream) {
		d.execTierFallback(sessionID, focuspipeline.ExecTierFallbackResume, sessionKind, execKind, log)
		return ""
	}
	return tier
}

func (d *focusDriver) execTierFallback(sessionID, reason, sessionKind, execKind string, log zerolog.Logger) {
	if _, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, _ := focuspipeline.AddExecTierFallback(p, reason, sessionKind, execKind, d.now())
		return next, nil
	}); err != nil {
		log.Warn().Err(err).Msg("focus: record execution tier fallback")
	}
}

// focusExecTierRequest is the tier a server turn carries: the execution
// tier for that turn only — no pin, so the session's tier stays.
func focusExecTierRequest(tier string) *chatTierRecommendationPayload {
	if tier == "" {
		return nil
	}
	return &chatTierRecommendationPayload{
		TaskType: "focus_exec", RecommendedTier: tier, ChosenTier: tier, Accepted: true,
		Source: focusExecTierSource, Reason: "Focus pipeline execution tier.",
	}
}

// focusExecCosts reads each pipeline session's cost since its start.
func focusExecCosts(tracker *usage.Tracker) func(starts map[string]time.Time) map[string]usage.SessionCost {
	if tracker == nil {
		return nil
	}
	return tracker.SessionCostsSince
}

// focusExecFacts reads the report's facts from the pipelines: feature work
// only (no release, no harness proposal), started within the window (since)
// and, with repo set, labeled with that repository. costs adds each one's
// cost from its start (since for a pipeline whose start is unknown).
func focusExecFacts(pipelines []focuspipeline.Pipeline, repo string, since time.Time, costs func(map[string]time.Time) map[string]usage.SessionCost) []focuspipeline.ExecFacts {
	facts := make([]focuspipeline.ExecFacts, 0, len(pipelines))
	starts := map[string]time.Time{}
	for _, p := range pipelines {
		if p.Kind == focuspipeline.KindRelease || p.HarnessFrom != nil {
			continue
		}
		f := focuspipeline.ExecFactsOf(p)
		if repo != "" && f.Repo != repo {
			continue
		}
		start := f.Start
		if start.IsZero() {
			if p.UpdatedAt.Before(since) {
				continue
			}
			start = since
		} else if start.Before(since) {
			continue
		}
		starts[p.SessionID] = start
		facts = append(facts, f)
	}
	if costs != nil && len(starts) > 0 {
		spent := costs(starts)
		for i := range facts {
			c := spent[facts[i].SessionID]
			facts[i].CostUSD, facts[i].UnpricedCalls = c.CostUSD, c.UnpricedCalls
		}
	}
	return facts
}

// execReport answers GET /v1/focus/exec-report?days=&repo=: the execution
// tier measurement, read-only — every number is a fact the server holds.
func (a *focusAPI) execReport(w http.ResponseWriter, r *http.Request) {
	days := focusExecReportDays
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > focusExecReportMaxDays {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("days must be 1 to %d", focusExecReportMaxDays)})
			return
		}
		days = n
	}
	repo := ""
	if raw := expandCwdHome(strings.TrimSpace(r.URL.Query().Get("repo"))); raw != "" {
		if repo = focusprobe.MainCheckout(r.Context(), raw); repo == "" {
			repo = filepath.Clean(raw)
		}
	}
	pipelines, err := a.store().List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	since := a.now().UTC().AddDate(0, 0, -days)
	var costs func(map[string]time.Time) map[string]usage.SessionCost
	if a.driver != nil {
		costs = a.driver.execCosts
	}
	facts := focusExecFacts(pipelines, repo, since, costs)
	report := focuspipeline.ExecReport(facts)
	writeJSON(w, http.StatusOK, map[string]any{
		"since":         since,
		"days":          days,
		"repo":          repo,
		"cost_recorded": costs != nil,
		"rows":          report.Rows,
		"comparisons":   report.Comparisons,
		"min_samples":   focuspipeline.ExecMinSamples,
		"pipelines":     facts,
	})
}

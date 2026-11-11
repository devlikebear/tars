package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

// execTierKinds is a tierKind over a fixed table; "" is the chat's tier.
func execTierKinds(kinds map[string]string) focusTierKind {
	return func(tier string) (string, error) {
		if tier == "" {
			tier = "chat"
		}
		kind, ok := kinds[tier]
		if !ok {
			return "", errors.New("tier " + tier + " is not configured")
		}
		return kind, nil
	}
}

func TestFocusCreateExecTier(t *testing.T) {
	f := newWorktreeFixture(t)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.sessions = f.store
	d.tierKind = execTierKinds(map[string]string{"chat": "anthropic", "heavy": "anthropic", "standard": "openai", "light": "claude-code-cli"})
	lib, err := focuspipeline.OpenHarnessLibrary(d.sharedDir(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lib.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib.Dir(), "lessons.json"), []byte(`[{"id":"a","status":"adopted"},{"id":"b","status":"proposed"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	create := func(extra string) (int, map[string]any) {
		rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+extra+`}`, true)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	for _, bad := range []string{`,"exec_tier":"huge"`, `,"exec_tier":"light","kind":"release"`} {
		if code, out := create(bad); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "exec_tier") {
			t.Fatalf("%s: %d %v", bad, code, out)
		}
	}
	d.tierKind = execTierKinds(map[string]string{"chat": "anthropic"})
	if code, out := create(`,"exec_tier":"light"`); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "not configured") {
		t.Fatalf("unconfigured tier: %d %v", code, out)
	}
	if list, _ := f.store.List(); len(list) != 0 {
		t.Fatalf("a refused execution tier left a session: %d", len(list))
	}
	d.tierKind = execTierKinds(map[string]string{"chat": "anthropic", "heavy": "anthropic", "standard": "openai", "light": "claude-code-cli"})

	code, out := create(`,"exec_tier":" Standard "`)
	if code != http.StatusCreated || out["exec_tier_warning"] != nil {
		t.Fatalf("standard: %d %v", code, out)
	}
	p := pipelineOf(t, f.store, out["session_id"].(string))
	if et := p.ExecTier; et == nil || et.Tier != "standard" || et.Repo != f.repo || et.Lessons != 1 || et.At.IsZero() {
		t.Fatalf("stored = %+v (repo %s)", p.ExecTier, f.repo)
	}
	if code, out = create(`,"exec_tier":"light"`); code != http.StatusCreated || out["exec_tier_warning"] != focusExecTierResumeWarning {
		t.Fatalf("light on a resuming provider: %d %v", code, out)
	}
	code, out = create(``)
	if code != http.StatusCreated {
		t.Fatalf("plain: %d", code)
	}
	raw, _ := os.ReadFile(filepath.Join(f.store.WorkspaceDir(), "sessions", out["session_id"].(string)+".pipeline.json"))
	if strings.Contains(string(raw), "exec_tier") {
		t.Fatalf("a pipeline that did not opt in records nothing:\n%s", raw)
	}

	plain := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	rec := focusRequest(t, plain, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+`,"exec_tier":"heavy"}`, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no driver: %d", rec.Code)
	}
}

// execTierSession is a session whose pipeline is building, opted in to
// tier ("" = not), with the session pinned to pin.
func execTierSession(t *testing.T, store *session.Store, tier, pin string) session.Session {
	t.Helper()
	sess := buildingFocusSession(t, store)
	if tier != "" {
		if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
			p.ExecTier = &focuspipeline.ExecTier{Tier: tier, At: time.Now()}
			return p, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if pin != "" {
		if err := store.SetTierPin(sess.ID, pin); err != nil {
			t.Fatal(err)
		}
	}
	return sess
}

func TestFocusExecTurnTier(t *testing.T) {
	kinds := execTierKinds(map[string]string{"chat": "anthropic", "heavy": "claude-code-cli", "standard": "openai", "light": "anthropic"})
	cases := []struct {
		name, tier, pin string
		kinds           focusTierKind
		review          bool
		want            string
		notice          string
	}{
		{name: "no opt-in", want: ""},
		{name: "build on another native provider", tier: "standard", want: "standard"},
		{name: "build on the same kind", tier: "light", want: "light"},
		{name: "review turn stays", tier: "standard", review: true, want: ""},
		{name: "resumed CLI session refuses a native tier", tier: "standard", pin: "heavy", want: "", notice: focuspipeline.ExecTierFallbackResume},
		{name: "native session refuses a CLI tier", tier: "heavy", want: "", notice: focuspipeline.ExecTierFallbackResume},
		{name: "unresolved tier", tier: "standard", kinds: execTierKinds(map[string]string{"chat": "anthropic"}), want: "", notice: focuspipeline.ExecTierFallbackUnresolved},
		{name: "no tier resolver", tier: "standard", kinds: func(string) (string, error) { return "", nil }, want: "standard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			sess := execTierSession(t, store, tc.tier, tc.pin)
			if tc.review {
				if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
					next, _, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 2, Blocks: focuspipeline.Blocks{Report: &focuspipeline.Report{Summary: "built", TasksDone: true}}}, time.Now())
					if err != nil {
						return p, err
					}
					next, _, err = focuspipeline.Apply(next, focuspipeline.Event{Kind: focuspipeline.EventVerification, Verification: &focuspipeline.Verification{Passed: true}}, time.Now())
					return next, err
				}); err != nil {
					t.Fatal(err)
				}
			}
			d := newFocusDriver(zerolog.Nop())
			d.sessions = store
			d.tierKind = kinds
			if tc.kinds != nil {
				d.tierKind = tc.kinds
			}
			for range 2 {
				if got := d.execTurnTier(sess.ID, zerolog.Nop()); got != tc.want {
					t.Fatalf("tier %q, want %q", got, tc.want)
				}
			}
			notices := noticesOf(pipelineOf(t, store, sess.ID), focuspipeline.NoticeExecTierFallback)
			switch {
			case tc.notice == "" && len(notices) != 0:
				t.Fatalf("unexpected notice %v", notices)
			case tc.notice != "" && (len(notices) != 1 || notices[0]["reason"] != tc.notice):
				t.Fatalf("notices %v, want one %s", notices, tc.notice)
			}
			if got, _ := store.Get(sess.ID); got.TierPin != tc.pin {
				t.Fatalf("pin changed to %q", got.TierPin)
			}
		})
	}
	var nilDriver *focusDriver
	if nilDriver.execTurnTier("x", zerolog.Nop()) != "" {
		t.Fatal("a nil driver picks no tier")
	}
	d := newFocusDriver(zerolog.Nop())
	d.sessions, d.tierKind = session.NewStore(t.TempDir()), kinds
	if d.execTurnTier("missing", zerolog.Nop()) != "" {
		t.Fatal("a session without a pipeline picks no tier")
	}
}

// tierRouter records the tiers a turn asked for.
type tierRouter struct {
	client llm.Client
	mu     sync.Mutex
	tiers  []llm.Tier
}

func (r *tierRouter) ClientFor(llm.Role) (llm.Client, llm.TierResolution, error) {
	return r.ClientForTier(llm.TierStandard)
}
func (r *tierRouter) ClientForTier(tier llm.Tier) (llm.Client, llm.TierResolution, error) {
	r.mu.Lock()
	r.tiers = append(r.tiers, tier)
	r.mu.Unlock()
	return r.client, llm.TierResolution{Tier: tier, Provider: "mock", Model: "m"}, nil
}
func (r *tierRouter) TierForRole(llm.Role) llm.Tier { return llm.TierStandard }
func (r *tierRouter) DefaultTier() llm.Tier         { return llm.TierStandard }

func (r *tierRouter) seen() []llm.Tier {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]llm.Tier(nil), r.tiers...)
}

// The execution tier rides on the turn's request for that turn only: the
// session's pin is untouched and the next turn is back on it.
func TestRunServerChatTurnOnTier(t *testing.T) {
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	deps, store, _ := testChatDeps(t, client)
	router := &tierRouter{client: client}
	deps.router = router
	sess, err := store.Create("s")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTierPin(sess.ID, "heavy"); err != nil {
		t.Fatal(err)
	}
	if _, err := runServerChatTurnOnTier(context.Background(), deps, sess.ID, "build", "light"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(sess.ID); got.TierPin != "heavy" {
		t.Fatalf("pin = %q", got.TierPin)
	}
	tiers := router.seen()
	if len(tiers) == 0 || tiers[len(tiers)-1] != llm.TierLight {
		t.Fatalf("execution turn tiers %v", tiers)
	}
	before := len(tiers)
	if _, err := runServerChatTurnOnTier(context.Background(), deps, sess.ID, "review", ""); err != nil {
		t.Fatal(err)
	}
	tiers = router.seen()
	if len(tiers) == before || tiers[len(tiers)-1] != llm.TierHeavy {
		t.Fatalf("session tier turn tiers %v", tiers[before:])
	}
}

func TestNewFocusTierKind(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{
			"anth": {Kind: "anthropic", AuthMode: "api-key", APIKey: "k", BaseURL: "https://api.anthropic.com"},
			"cc":   {Kind: "claude-code-cli"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"heavy": {Provider: "cc", Model: "opus"},
			"light": {Provider: "anth", Model: "claude-haiku-5"},
		},
		LLMDefaultTier:  "light",
		LLMRoleDefaults: map[string]string{"chat_main": "heavy"},
	}}
	kind := newFocusTierKind(cfg)
	if k, err := kind(""); err != nil || k != "claude-code-cli" {
		t.Fatalf("chat = %q %v", k, err)
	}
	if k, err := kind("light"); err != nil || k != "anthropic" {
		t.Fatalf("light = %q %v", k, err)
	}
	if _, err := kind("standard"); err == nil {
		t.Fatal("an unconfigured tier resolves")
	}
	cfg.LLMRoleDefaults = nil
	if k, err := newFocusTierKind(cfg)(""); err != nil || k != "anthropic" {
		t.Fatalf("default tier = %q %v", k, err)
	}
	if focusExecTierRequest("") != nil {
		t.Fatal("no tier, no request")
	}
	if req := focusExecTierRequest("light"); req.ChosenTier != "light" || req.Pin || req.Source != focusExecTierSource {
		t.Fatalf("request %+v", req)
	}
	if focusExecCosts(nil) != nil {
		t.Fatal("no tracker, no costs")
	}
}

func TestFocusExecFacts(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	since := now.AddDate(0, 0, -30)
	labeled := func(id, tier, repo string, at time.Time) focuspipeline.Pipeline {
		p := focuspipeline.New(id, "g", at)
		p.ExecTier = &focuspipeline.ExecTier{Tier: tier, Repo: repo, At: at}
		return p
	}
	unlabeled := func(id string, updated time.Time) focuspipeline.Pipeline {
		p := focuspipeline.New(id, "g", updated)
		p.UpdatedAt = updated
		return p
	}
	release := unlabeled("rel", now)
	release.Kind = focuspipeline.KindRelease
	proposal := labeled("prop", "light", "/r", now)
	proposal.HarnessFrom = &focuspipeline.HarnessSource{}
	pipelines := []focuspipeline.Pipeline{
		labeled("a", "heavy", "/r", now.Add(-time.Hour)),
		labeled("b", "light", "/other", now.Add(-time.Hour)),
		labeled("old", "light", "/r", now.AddDate(0, 0, -40)),
		unlabeled("u", now.Add(-2*time.Hour)),
		unlabeled("u-old", now.AddDate(0, 0, -40)),
		release, proposal,
	}
	var asked map[string]time.Time
	costs := func(starts map[string]time.Time) map[string]usage.SessionCost {
		asked = starts
		return map[string]usage.SessionCost{"a": {CostUSD: 2, UnpricedCalls: 1}}
	}
	facts := focusExecFacts(pipelines, "", since, costs)
	ids := []string{}
	for _, f := range facts {
		ids = append(ids, f.SessionID)
	}
	if strings.Join(ids, ",") != "a,b,u" {
		t.Fatalf("facts %v", ids)
	}
	if facts[0].CostUSD != 2 || facts[0].UnpricedCalls != 1 || facts[1].CostUSD != 0 {
		t.Fatalf("costs %+v", facts)
	}
	if !asked["a"].Equal(now.Add(-time.Hour)) || !asked["u"].Equal(since) {
		t.Fatalf("cost starts %v", asked)
	}
	facts = focusExecFacts(pipelines, "/r", since, nil)
	if len(facts) != 1 || facts[0].SessionID != "a" {
		t.Fatalf("repo filter %+v", facts)
	}
}

func TestFocusExecReportEndpoint(t *testing.T) {
	store := session.NewStore(t.TempDir())
	now := time.Now().UTC()
	for _, p := range []focuspipeline.Pipeline{
		func() focuspipeline.Pipeline {
			p := focuspipeline.New("a", "g", now)
			p.ExecTier = &focuspipeline.ExecTier{Tier: "heavy", At: now.Add(-time.Hour)}
			return p
		}(),
		func() focuspipeline.Pipeline {
			p := focuspipeline.New("b", "g", now)
			p.ExecTier = &focuspipeline.ExecTier{Tier: "light", Lessons: 3, At: now.Add(-time.Hour)}
			return p
		}(),
	} {
		if err := focusStoreFor(store).Save(p); err != nil {
			t.Fatal(err)
		}
	}
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.execCosts = func(starts map[string]time.Time) map[string]usage.SessionCost {
		return map[string]usage.SessionCost{"a": {CostUSD: 4}, "b": {CostUSD: 1}}
	}
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodGet, "/v1/focus/exec-report?days=7", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Days         int                            `json:"days"`
		CostRecorded bool                           `json:"cost_recorded"`
		Rows         []focuspipeline.ExecReportRow  `json:"rows"`
		Comparisons  []focuspipeline.ExecComparison `json:"comparisons"`
		Pipelines    []focuspipeline.ExecFacts      `json:"pipelines"`
		MinSamples   int                            `json:"min_samples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Days != 7 || !out.CostRecorded || out.MinSamples != focuspipeline.ExecMinSamples || len(out.Rows) != 2 || len(out.Pipelines) != 2 {
		t.Fatalf("report %+v", out)
	}
	if out.Rows[0].Tier != "heavy" || out.Rows[1].Tier != "light" || out.Rows[1].Library != focuspipeline.ExecLibraryApplied || out.Rows[0].Running != 1 {
		t.Fatalf("rows %+v", out.Rows)
	}
	if len(out.Comparisons) != 1 || out.Comparisons[0].Recommend {
		t.Fatalf("comparisons %+v", out.Comparisons)
	}
	for _, bad := range []string{"0", "366", "x"} {
		if rec := focusRequest(t, h, http.MethodGet, "/v1/focus/exec-report?days="+bad, "", false); rec.Code != http.StatusBadRequest {
			t.Fatalf("days=%s: %d", bad, rec.Code)
		}
	}
	rec = focusRequest(t, newFocusPipelineHandler(store, nil, nil, zerolog.Nop()), http.MethodGet, "/v1/focus/exec-report?repo="+t.TempDir(), "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cost_recorded":false`) || !strings.Contains(rec.Body.String(), `"pipelines":[]`) {
		t.Fatalf("no driver, repo filter: %d %s", rec.Code, rec.Body.String())
	}
}

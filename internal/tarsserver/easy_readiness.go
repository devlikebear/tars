package tarsserver

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/pkg/llm"
)

// GET /v1/easy/readiness (vibe-coder-identity.md phase 4): the Start
// screen's answer — whether this computer has what an easy-mode project
// needs, and the one next thing to do (easyproject.ReadinessOf). The Start
// screen reads it again every few seconds while the person installs or signs
// in, so the answer is kept briefly; Claude Code's sign-in is read with
// `claude auth status`, which spends no usage.

// easyReadinessTTL is how long one answer is kept.
const easyReadinessTTL = 5 * time.Second

type easyReadiness struct {
	// kind is the chat tier's provider kind, "" when none resolves.
	kind func() string
	// findCLI finds a CLI engine's program.
	findCLI func(kind string) (string, error)
	// runCLI runs it (the provider probe's runner).
	runCLI providerCLIRunner
	// toolsReady is the scaffolder's Ready: Node and npm, or the e2e stand-in.
	toolsReady func() error
	now        func() time.Time

	mu     sync.Mutex
	cached *easyproject.Readiness
	at     time.Time
}

func newEasyReadinessHandler(cfg config.Config) http.Handler {
	scaffolder := easyproject.ScaffolderFromEnv()
	h := &easyReadiness{
		kind:       func() string { return chatProviderKind(cfg) },
		findCLI:    findEngineCLI,
		runCLI:     runProviderCLI,
		toolsReady: scaffolder.Ready,
		now:        time.Now,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/easy/readiness", h.serve)
	return mux
}

// findEngineCLI finds the program a CLI engine kind runs, the way the
// provider itself does.
func findEngineCLI(kind string) (string, error) {
	if kind == "antigravity-cli" {
		return llm.FindAntigravityCLIPath()
	}
	return llm.FindClaudeCodeCLIPath()
}

func (h *easyReadiness) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if h.cached == nil || now.Sub(h.at) >= easyReadinessTTL {
		answer := h.read(r.Context())
		h.cached, h.at = &answer, now
	}
	writeJSON(w, http.StatusOK, map[string]any{"readiness": h.cached})
}

func (h *easyReadiness) read(ctx context.Context) easyproject.Readiness {
	facts := easyproject.EngineFacts{Kind: strings.ToLower(strings.TrimSpace(h.kind()))}
	if easyproject.IsCLIEngine(facts.Kind) {
		if path, err := h.findCLI(facts.Kind); err == nil {
			facts.Found = true
			if facts.Kind == "claude-code-cli" {
				out, _ := h.runCLI(ctx, path, claudeAuthStatusArgs...)
				facts.SignedIn, _ = parseClaudeAuthStatus(out)
			}
		}
	}
	return easyproject.ReadinessOf(facts, h.toolsReady())
}

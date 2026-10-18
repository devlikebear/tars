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
	"time"

	"github.com/devlikebear/tars/internal/casebridge"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

// CASE's Bridge (case-companion phase 2, docs/decisions/case-bridge.md):
//
//	POST /v1/case/ask     {question, scope, view} → 202 {session_id}
//	GET  /v1/case/answer  → {session_id, busy, question, answer, cards}
//
// A question runs as a turn in CASE's own hidden worker session, read-only
// (plan mode, no repository lease) on the light tier. The server takes a
// snapshot of TARS (casebridge.Snapshot) when the question arrives and
// hands it to that one turn as a <tars-status> turn context — not saved in
// the transcript, never offered as a tool, so it works on CLI providers
// too. The console streams the answer from GET /v1/chat/stream and then
// reads GET /v1/case/answer for the text without its <case-cards> block
// and the cards that point at something in the same snapshot.

// caseBridgeSource names Bridge turns in the ops queue and the audit; it is
// one of unattendedReadOnlySources, so a turn that would need a permission
// answer is refused at once instead of queued.
const caseBridgeSource = "case_bridge"

// caseBridgeTier is the tier CASE's session is pinned to.
const caseBridgeTier = "light"

const caseBridgeTitle = "CASE Bridge"

var errCaseBridgeBusy = errors.New("CASE is still answering the last question")

// caseBridgeSources are the stores a snapshot reads. Each may be nil.
type caseBridgeSources struct {
	cronJobs func() ([]cron.Job, error)
	usage    func(period string) (float64, bool)
	pulse    func() pulse.Snapshot
}

type caseBridge struct {
	workspaceDir string
	logger       zerolog.Logger
	now          func() time.Time
	sources      caseBridgeSources

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	sessions *session.Store
	activity *chatActivity
	cancels  *chatCancelRegistry
	runTurn  func(ctx context.Context, sessionID, question, consoleContext string) error
	busy     bool
	// The question being (or last) answered and the snapshot it was asked
	// against: the turn context reads it, and cards are checked against it.
	sessionID string
	question  string
	snapshot  casebridge.Snapshot
	turnErr   string
}

func newCaseBridge(workspaceDir string, sources caseBridgeSources, logger zerolog.Logger) *caseBridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &caseBridge{
		workspaceDir: workspaceDir,
		logger:       logger,
		now:          time.Now,
		sources:      sources,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// bind connects the Bridge to the chat handler's turn path.
func (c *caseBridge) bind(deps chatHandlerDeps) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions = deps.store
	c.activity = deps.chatActivity
	c.cancels = deps.cancelRegistry
	c.runTurn = func(ctx context.Context, sessionID, question, consoleContext string) error {
		_, err := runServerChatTurnAs(ctx, deps, sessionID, question, consoleContext, chatTurnOrigin{unattended: caseBridgeSource, readOnly: true})
		return err
	}
}

// Close stops taking questions and waits for a running one to end.
func (c *caseBridge) Close() {
	if c == nil {
		return
	}
	c.cancel()
	c.wg.Wait()
}

// turnContext is the <tars-status> block for a turn of CASE's session
// while a question is being answered; empty for every other session.
func (c *caseBridge) turnContext(sessionID string) string {
	if c == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.busy || sessionID != c.sessionID {
		return ""
	}
	return casebridge.TurnContext(casebridge.Render(c.snapshot))
}

type caseAskRequest struct {
	Question string          `json:"question"`
	Scope    string          `json:"scope"`
	View     casebridge.View `json:"view"`
}

func newCaseBridgeHandler(c *caseBridge) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/case/ask", c.handleAsk)
	mux.HandleFunc("/v1/case/answer", c.handleAnswer)
	return mux
}

func (c *caseBridge) handleAsk(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req caseAskRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question is required"})
		return
	}
	sessionID, status, err := c.ask(question, req.Scope, req.View, serverauth.RoleFromRequest(r))
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error(), "session_id": sessionID})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"session_id": sessionID})
}

// ask reserves CASE's session, takes the snapshot and starts the turn. It
// returns the HTTP status for an error.
func (c *caseBridge) ask(question, scope string, view casebridge.View, role string) (string, int, error) {
	c.mu.Lock()
	if c.runTurn == nil || c.sessions == nil {
		c.mu.Unlock()
		return "", http.StatusServiceUnavailable, errors.New("CASE Bridge is unavailable")
	}
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return "", http.StatusServiceUnavailable, errors.New("the server is shutting down")
	}
	sessionID, err := c.sessionLocked()
	if err != nil {
		c.mu.Unlock()
		c.logger.Error().Err(err).Msg("case bridge: prepare session failed")
		return "", http.StatusInternalServerError, errors.New("prepare CASE's session failed")
	}
	if c.busy || c.cancels.Running(sessionID) {
		c.mu.Unlock()
		return sessionID, http.StatusConflict, errCaseBridgeBusy
	}
	c.busy = true
	c.sessionID = sessionID
	c.question = question
	c.turnErr = ""
	c.snapshot = c.snapshotLocked(sessionID)
	run := c.runTurn
	c.wg.Add(1)
	c.mu.Unlock()

	consoleContext := casebridge.QuestionContext(view, scope)
	go func() {
		defer c.wg.Done()
		ctx := serverauth.WithRoleContext(c.ctx, role)
		err := run(ctx, sessionID, question, consoleContext)
		c.mu.Lock()
		c.busy = false
		if err != nil {
			c.turnErr = "CASE couldn't answer"
		}
		c.mu.Unlock()
		if err != nil {
			c.logger.Info().Err(err).Str("session_id", sessionID).Msg("case bridge: turn ended without an answer")
		}
	}()
	return sessionID, 0, nil
}

type caseAnswer struct {
	SessionID string            `json:"session_id"`
	Busy      bool              `json:"busy"`
	Question  string            `json:"question"`
	Answer    string            `json:"answer"`
	Cards     []casebridge.Card `json:"cards"`
	Error     string            `json:"error,omitempty"`
}

func (c *caseBridge) handleAnswer(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, c.answer())
}

// answer is the last question's reply without its card block, and the
// cards that point at something in the snapshot it was asked against.
// While the turn runs the text is empty; the console streams it instead.
func (c *caseBridge) answer() caseAnswer {
	out := caseAnswer{Cards: []casebridge.Card{}}
	if c == nil {
		return out
	}
	c.mu.Lock()
	out.SessionID = c.sessionID
	out.Busy = c.busy
	out.Question = c.question
	out.Error = c.turnErr
	snapshot := c.snapshot
	sessions := c.sessions
	c.mu.Unlock()
	if out.Busy || out.SessionID == "" || sessions == nil {
		return out
	}
	text, proposed := casebridge.SplitCards(lastAssistantReply(sessions.TranscriptPath(out.SessionID)))
	out.Answer = text
	out.Cards = casebridge.ValidCards(proposed, snapshot)
	return out
}

// lastAssistantReply is the last assistant text after the last user
// message of a transcript.
func lastAssistantReply(transcriptPath string) string {
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return ""
	}
	reply := ""
	for _, m := range messages {
		switch {
		case m.Role == "user":
			reply = ""
		case m.Role == "assistant" && strings.TrimSpace(m.Content) != "":
			reply = m.Content
		}
	}
	return strings.TrimSpace(reply)
}

// caseBridgeState is where CASE's session id is kept, with the local day
// it was made: a new day starts a new session, so the session the light
// tier resumes never grows past a day of questions.
type caseBridgeState struct {
	SessionID string `json:"session_id"`
	Day       string `json:"day"`
}

func (c *caseBridge) statePath() string {
	return filepath.Join(c.workspaceDir, "_shared", "case", "bridge.json")
}

// sessionLocked returns today's CASE session, making it on first use, and
// sets it up for a read-only question. Called with c.mu held.
func (c *caseBridge) sessionLocked() (string, error) {
	day := c.now().Format("2006-01-02")
	var state caseBridgeState
	if raw, err := os.ReadFile(c.statePath()); err == nil {
		_ = json.Unmarshal(raw, &state)
	}
	id := strings.TrimSpace(state.SessionID)
	if id != "" {
		if _, err := c.sessions.Get(id); err != nil || state.Day != day {
			id = ""
		}
	}
	if id == "" {
		created, err := c.sessions.CreateWithOptions(caseBridgeTitle, "worker", true)
		if err != nil {
			return "", err
		}
		id = created.ID
		// Never isolated and never holding a repository: a question must not
		// push anyone's turns into a worktree.
		if err := c.sessions.SetIsolation(id, session.IsolationOff); err != nil {
			return "", err
		}
		if err := writeCaseBridgeState(c.statePath(), caseBridgeState{SessionID: id, Day: day}); err != nil {
			_ = c.sessions.Delete(id)
			return "", err
		}
	}
	if err := c.sessions.SetPermissionMode(id, chatPermissionModePlan); err != nil {
		return "", err
	}
	if err := c.sessions.SetTierPin(id, caseBridgeTier); err != nil {
		return "", err
	}
	return id, nil
}

func writeCaseBridgeState(path string, state caseBridgeState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// caseBridgeUsage reads the total spend of a usage period ("today",
// "week"); nil without a tracker.
func caseBridgeUsage(tracker *usage.Tracker) func(period string) (float64, bool) {
	if tracker == nil {
		return nil
	}
	return func(period string) (float64, bool) {
		summary, err := tracker.Summary(period, "provider")
		if err != nil {
			return 0, false
		}
		return summary.TotalCostUSD, true
	}
}

// caseBridgeCronWindow is how far back a failed cron run still counts.
const caseBridgeCronWindow = 24 * time.Hour

// snapshotLocked reads TARS's state for one question. CASE's own session
// is left out (its turn is the one about to run). Called with c.mu held;
// none of the stores it reads call back into the Bridge.
func (c *caseBridge) snapshotLocked(caseSessionID string) casebridge.Snapshot {
	now := c.now()
	s := casebridge.Snapshot{At: now}

	if store := focusStoreFor(c.sessions); store != nil {
		if pipelines, err := store.List(); err == nil {
			for _, p := range pipelines {
				sess, err := c.sessions.Get(p.SessionID)
				if err != nil {
					continue
				}
				s.Pipelines = append(s.Pipelines, casebridge.Pipeline{
					SessionID:  p.SessionID,
					Title:      sess.Title,
					Stage:      focusStageName(p),
					Gate:       p.OpenGate,
					NeedsInput: p.OpenGate != focuspipeline.GateNone || p.NeedsInput() > 0,
					GoalMode:   p.GoalActive(),
					Finished:   focuspipeline.Finished(p),
					UpdatedAt:  p.UpdatedAt,
				})
			}
		}
	}

	if c.activity != nil {
		snap := c.activity.snapshot()
		for _, t := range snap.Running {
			if t.SessionID == caseSessionID {
				continue
			}
			s.Running = append(s.Running, casebridge.Turn{SessionID: t.SessionID, Title: t.Session, StartedAt: t.StartedAt})
		}
		for _, p := range snap.Pending {
			s.Approvals = append(s.Approvals, casebridge.Approval{SessionID: p.SessionID, Title: p.Session, Tool: p.ToolName, At: p.AskedAt})
		}
		for _, q := range c.activity.queuedApprovals() {
			s.Approvals = append(s.Approvals, casebridge.Approval{SessionID: q.SessionID, Title: q.Session, Tool: q.ToolName, Queued: true, Source: q.Source, At: q.RequestedAt})
		}
	}

	if c.sources.cronJobs != nil {
		if jobs, err := c.sources.cronJobs(); err == nil {
			for _, j := range jobs {
				if strings.TrimSpace(j.LastRunError) == "" || j.LastRunAt == nil || now.Sub(*j.LastRunAt) > caseBridgeCronWindow {
					continue
				}
				s.Cron = append(s.Cron, casebridge.CronFailure{JobID: j.ID, Name: j.Name, Error: j.LastRunError, At: *j.LastRunAt})
			}
		}
	}

	if c.sources.usage != nil {
		today, okToday := c.sources.usage("today")
		week, okWeek := c.sources.usage("week")
		s.Usage = casebridge.Usage{Known: okToday && okWeek, TodayUSD: today, WeekUSD: week}
	}

	if c.sources.pulse != nil {
		s.Pulse = latestPulseSignals(c.sources.pulse())
	}
	return s
}

// focusStageName is the current stage's label, or its id.
func focusStageName(p focuspipeline.Pipeline) string {
	if label := strings.TrimSpace(focusCurrentLabel(p)); label != "" {
		return label
	}
	return string(p.Current)
}

// latestPulseSignals are the warnings and worse of pulse's latest tick
// that looked at anything.
func latestPulseSignals(snap pulse.Snapshot) []casebridge.Signal {
	for i := len(snap.Recent) - 1; i >= 0; i-- {
		tick := snap.Recent[i]
		if tick.Skipped {
			continue
		}
		out := []casebridge.Signal{}
		for _, sig := range tick.Signals {
			if sig.Severity < pulse.SeverityWarn {
				continue
			}
			out = append(out, casebridge.Signal{Kind: string(sig.Kind), Severity: sig.Severity.String(), Summary: sig.Summary})
		}
		return out
	}
	return nil
}

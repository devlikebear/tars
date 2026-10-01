package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// focusDriver carries out the actions a focus pipeline asks for after a
// turn or a gate action (docs/decisions/focus-mode.md §4, §9): it runs the
// plan's verification commands and feeds the result back, and it starts
// the next turn itself, so the build loop continues with no console open.
//
// One run per session at a time, in a goroutine bound to the server's
// lifetime, working through a queue of actions: an action that arrives
// while the run is busy (a person's turn ended while a decision answer
// waited) joins the queue instead of replacing what is owed. Every step
// claims the session like any chat turn (chatCancelRegistry.Claim), so a
// driver step and a person's turn never run together, and re-reads the
// pipeline first: a stopped, finished or gated pipeline gets nothing more.
// A run ends when the queue is empty, on POST /v1/chat/cancel for the
// session, at shutdown, and when a person starts a turn on the session —
// human input wins; that turn's post-turn hook starts a new run. A server
// turn that fails raises the resumable blocked gate (FailTurn), and a
// restart that cut a step off raises it at startup (Interrupt).
//
// The driver never asks the model whether a stage is done: it only moves
// facts (verification exit codes, the turn's blocks) into the pipeline.
type focusDriver struct {
	ctx    context.Context
	stop   context.CancelFunc
	logger zerolog.Logger
	now    func() time.Time
	// idlePoll is how often a run checks whether another turn on its
	// session has ended before it starts the next step.
	idlePoll time.Duration

	// Bound by the chat handler (bind): how a turn runs and a command is
	// verified, and where the verification step reports progress.
	sessions *session.Store
	runTurn  focusTurnRunner
	verify   focusVerifier
	feeds    *chatTurnFeeds
	activity *chatActivity
	cancels  *chatCancelRegistry
	qaTurn   focusQATurnRunner
	// The PR stages (focus_pr_poll.go): the gh probe, how often it runs,
	// how a finished pipeline's worktree ends, and where attention-worthy
	// changes are announced.
	probe          focusPRProber
	prPoll         time.Duration
	finishWorktree func(ctx context.Context, sessionID, action string) error
	notify         func(context.Context, notificationEvent)
	// localBranch is the branch checked out in a folder; discardCheck says
	// why a merged pipeline's worktree must be kept ("" = safe to discard).
	localBranch  func(ctx context.Context, dir string) string
	discardCheck func(ctx context.Context, dir, head string) string

	mu   sync.Mutex
	runs map[string]*focusRun
	// qaBusy holds the Q&A sessions answering a question now.
	qaBusy map[string]bool
	// pollers holds each session's running PR poller.
	pollers map[string]*focusPoller
	wg      sync.WaitGroup
}

// focusTurnRunner runs one turn on a session (runServerChatTurn in the
// server). ctx carries the run and the role the turn runs as.
type focusTurnRunner func(ctx context.Context, sessionID, message string) error

// focusVerifier runs one verification command for a session.
type focusVerifier func(ctx context.Context, sessionID, command string) (focuspipeline.VerificationResult, error)

// focusVerifyTimeout bounds one verification command: longer than a full
// `make test` of this repository.
const focusVerifyTimeout = 15 * time.Minute

// focusPreemptWait bounds how long a person's turn waits for a cancelled
// run to wind down.
const focusPreemptWait = 30 * time.Second

// focusBusyRetries bounds how often a step that found its session claimed
// (a person's turn starting in the same instant) waits and tries again.
const focusBusyRetries = 50

type focusRun struct {
	sessionID string
	role      string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	mu        sync.Mutex
	queue     []focuspipeline.Action
	cancelled bool
}

// push queues act after what the run already owes. Decision answers join
// answers already waiting in the queue, so answers given while a turn runs
// (or before the run reaches them) go out as one turn.
func (r *focusRun) push(act focuspipeline.Action) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(act.Answers) > 0 {
		for i := range r.queue {
			if len(r.queue[i].Answers) > 0 {
				r.queue[i] = mergeAnswers(r.queue[i], act)
				return
			}
		}
	}
	r.queue = append(r.queue, act)
}

// mergeAnswers is one answer turn carrying a's answers and then b's (a
// card answered twice keeps its latest answer).
func mergeAnswers(a, b focuspipeline.Action) focuspipeline.Action {
	answers := append([]focuspipeline.Answer(nil), a.Answers...)
	for _, ans := range b.Answers {
		if i := slices.IndexFunc(answers, func(x focuspipeline.Answer) bool { return x.CardID == ans.CardID }); i >= 0 {
			answers[i] = ans
			continue
		}
		answers = append(answers, ans)
	}
	return focuspipeline.Action{Kind: focuspipeline.ActionSendTurn, Prompt: focuspipeline.AnswersPrompt(answers), Answers: answers}
}

// foldLate appends late answers as context to the next turn the run owes;
// false when none is queued.
func (r *focusRun) foldLate(late []focuspipeline.Answer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.queue {
		if r.queue[i].Kind == focuspipeline.ActionSendTurn && len(r.queue[i].Answers) == 0 {
			r.queue[i].Prompt = strings.TrimRight(r.queue[i].Prompt, "\n") + "\n\n" + focuspipeline.LateAnswersContext(late)
			return true
		}
	}
	return false
}

// pushFront makes act the run's next step: what its own step asked for
// comes before what was queued meanwhile.
func (r *focusRun) pushFront(act focuspipeline.Action) {
	r.mu.Lock()
	r.queue = append([]focuspipeline.Action{act}, r.queue...)
	r.mu.Unlock()
}

func (r *focusRun) pop() (focuspipeline.Action, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return focuspipeline.Action{}, false
	}
	act := r.queue[0]
	r.queue = r.queue[1:]
	return act, true
}

func (r *focusRun) stop() {
	r.mu.Lock()
	r.cancelled = true
	r.mu.Unlock()
	r.cancel()
}

func (r *focusRun) isCancelled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelled
}

type focusRunKey struct{}

func focusRunFrom(ctx context.Context) *focusRun {
	if ctx == nil {
		return nil
	}
	run, _ := ctx.Value(focusRunKey{}).(*focusRun)
	return run
}

func newFocusDriver(logger zerolog.Logger) *focusDriver {
	ctx, stop := context.WithCancel(context.Background())
	return &focusDriver{
		ctx:      ctx,
		stop:     stop,
		logger:   logger,
		now:      time.Now,
		idlePoll: 100 * time.Millisecond,
		prPoll:   focusPRPollInterval,
		runs:     map[string]*focusRun{},
		qaBusy:   map[string]bool{},
		pollers:  map[string]*focusPoller{},
	}
}

// bind connects the driver to the chat handler's turn path.
func (d *focusDriver) bind(deps chatHandlerDeps) {
	if d == nil {
		return
	}
	d.sessions = deps.store
	d.feeds = deps.turnFeeds
	d.activity = deps.chatActivity
	d.cancels = deps.cancelRegistry
	d.bindPR(deps.tooling)
	d.runTurn = func(ctx context.Context, sessionID, message string) error {
		_, err := runServerChatTurn(ctx, deps, sessionID, message, "")
		return err
	}
	d.qaTurn = func(ctx context.Context, qaSessionID, question, consoleContext string) error {
		_, err := runServerChatTurnAs(ctx, deps, qaSessionID, question, consoleContext, chatTurnOrigin{unattended: focusQASource, readOnly: true})
		return err
	}
	d.verify = func(ctx context.Context, sessionID, command string) (focuspipeline.VerificationResult, error) {
		results, _, err := runTaskVerificationCommands(ctx, deps.store, sessionID, "", []string{command}, focusVerifyTimeout, focusFailureExcerpt)
		if err != nil || len(results) == 0 {
			return focuspipeline.VerificationResult{Command: command}, err
		}
		r := results[0]
		return focuspipeline.VerificationResult{
			Command: command, ExitCode: r.ExitCode, TimedOut: r.TimedOut,
			Passed: r.Status == "passed", Excerpt: r.Output,
		}, nil
	}
	// Pipelines a restart left waiting on PR facts poll again.
	if n := d.resumePRPolls(); n > 0 {
		d.logger.Info().Int("count", n).Msg("focus: resumed PR polls")
	}
}

// Close stops every run and waits for them, at most until ctx ends:
// server shutdown must not hang on a turn that ignores its cancel.
func (d *focusDriver) Close(ctx context.Context) {
	if d == nil {
		return
	}
	// Under mu: start and ask add to wg only while the driver is open.
	d.mu.Lock()
	d.stop()
	d.mu.Unlock()
	finished := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
		d.logger.Warn().Msg("focus: driver runs still going at shutdown")
	}
}

// start carries out act for sessionID: queued on the session's run when
// one is going, else in a new run. role is the role of the person whose
// action led here; the run's turns run as that role.
func (d *focusDriver) start(sessionID string, act focuspipeline.Action, role string) {
	d.watchPR(sessionID)
	if d == nil || act.Kind == focuspipeline.ActionNone || strings.TrimSpace(sessionID) == "" || d.runTurn == nil {
		return
	}
	d.mu.Lock()
	if d.ctx.Err() != nil {
		d.mu.Unlock()
		return
	}
	previous := d.runs[sessionID]
	if previous != nil && !previous.isCancelled() {
		previous.push(act)
		d.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(d.ctx)
	run := &focusRun{sessionID: sessionID, role: role, cancel: cancel, done: make(chan struct{}), queue: []focuspipeline.Action{act}}
	run.ctx = context.WithValue(serverauth.WithRoleContext(ctx, role), focusRunKey{}, run)
	d.runs[sessionID] = run
	d.wg.Add(1)
	d.mu.Unlock()
	var after <-chan struct{}
	if previous != nil {
		after = previous.done
	}
	go d.loop(run, after)
}

// afterTurn is the post-turn hook: a turn the run itself started hands its
// next action back to the front of the run's queue; any other turn (a
// person's) starts or joins a run.
func (d *focusDriver) afterTurn(ctx context.Context, sessionID string, act focuspipeline.Action, role string) {
	d.watchPR(sessionID)
	if d == nil || act.Kind == focuspipeline.ActionNone {
		return
	}
	if run := focusRunFrom(ctx); run != nil && run.sessionID == sessionID {
		run.pushFront(act)
		return
	}
	d.start(sessionID, act, role)
}

// cancel stops the session's run, if any, and waits (bounded) for it to
// end. The run stays the session's run until it has ended, so a new run
// never starts beside it. It reports whether there was a run.
func (d *focusDriver) cancel(sessionID string) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	run := d.runs[sessionID]
	d.mu.Unlock()
	if run == nil {
		return false
	}
	run.stop()
	select {
	case <-run.done:
	case <-time.After(focusPreemptWait):
		d.logger.Warn().Str("session_id", sessionID).Msg("focus: driver run did not stop in time")
	}
	return true
}

// running reports whether the session has a run.
func (d *focusDriver) running(sessionID string) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runs[sessionID] != nil
}

func (d *focusDriver) loop(run *focusRun, after <-chan struct{}) {
	defer d.wg.Done()
	defer close(run.done)
	defer func() {
		d.mu.Lock()
		if d.runs[run.sessionID] == run {
			delete(d.runs, run.sessionID)
		}
		d.mu.Unlock()
	}()
	if after != nil {
		// The run this one follows winds down first: one run per session.
		select {
		case <-after:
		case <-run.ctx.Done():
			return
		}
	}
	log := d.logger.With().Str("session_id", run.sessionID).Logger()
	busy := 0
	for {
		act, ok := run.pop()
		if !ok {
			return
		}
		if !d.waitIdle(run) {
			return
		}
		if len(act.Answers) > 0 {
			var ok bool
			if act, ok = d.currentAnswers(run, act, log); !ok {
				continue
			}
		}
		if !d.stepAllowed(run.sessionID, act) {
			log.Debug().Str("action", act.Kind).Msg("focus: step skipped, the pipeline moved on")
			continue
		}
		switch act.Kind {
		case focuspipeline.ActionSendTurn:
			err := d.runTurn(run.ctx, run.sessionID, act.Prompt)
			if errors.Is(err, errChatTurnBusy) && busy < focusBusyRetries {
				busy++
				run.pushFront(act)
				continue
			}
			busy = 0
			if err != nil {
				if run.ctx.Err() == nil {
					d.turnFailed(run.sessionID, err, log)
				}
				return
			}
		case focuspipeline.ActionRunVerification:
			next, claimed := d.runVerification(run, log)
			if !claimed && busy < focusBusyRetries {
				busy++
				run.pushFront(act)
				continue
			}
			busy = 0
			if next.Kind != focuspipeline.ActionNone {
				run.pushFront(next)
			}
		default:
			log.Warn().Str("action", act.Kind).Msg("focus: unknown action")
		}
	}
}

// stepAllowed re-reads the pipeline before a step: only an active pipeline
// with no gate open gets a turn or a verification, and verification only
// while one is awaited — a stopped pipeline never gets a fix turn.
func (d *focusDriver) stepAllowed(sessionID string, act focuspipeline.Action) bool {
	p, ok, err := focusStoreFor(d.sessions).Get(sessionID)
	if err != nil || !ok || !p.Active() || p.OpenGate != focuspipeline.GateNone {
		return false
	}
	return act.Kind != focuspipeline.ActionRunVerification || p.AwaitingVerification
}

// currentAnswers keeps the answers of act whose decision card is still in
// the pipeline's current stage and round. The rest are never sent as a turn
// of a stage they were not asked in: they ride in the next turn the run
// owes as context, or, with none queued, leave a notice card. ok is false
// when no answer is left to send.
func (d *focusDriver) currentAnswers(run *focusRun, act focuspipeline.Action, log zerolog.Logger) (focuspipeline.Action, bool) {
	store := focusStoreFor(d.sessions)
	p, found, err := store.Get(run.sessionID)
	if err != nil || !found {
		return act, false
	}
	fresh, late := focuspipeline.SplitStaleAnswers(p, act.Answers)
	if len(late) > 0 && !run.foldLate(late) {
		if _, _, err := store.Update(run.sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
			next := focuspipeline.AddNotice(p, focuspipeline.NoticeLateAnswers, map[string]any{"answers": late}, d.now())
			if len(fresh) == 0 && next.PendingTurn == act.Prompt {
				next.PendingTurn = "" // nothing of it is sent
			}
			return next, nil
		}); err != nil {
			log.Warn().Err(err).Msg("focus: record late decision answers")
		}
	}
	if len(fresh) == 0 {
		log.Debug().Int("late", len(late)).Msg("focus: decision answers arrived after their stage moved on")
		return act, false
	}
	if len(late) > 0 {
		act = focuspipeline.Action{Kind: focuspipeline.ActionSendTurn, Prompt: focuspipeline.AnswersPrompt(fresh), Answers: fresh}
	}
	return act, true
}

// turnFailed raises the resumable blocked gate for a server turn that
// ended in an error, so the loop never stops silently.
func (d *focusDriver) turnFailed(sessionID string, cause error, log zerolog.Logger) {
	log.Info().Err(cause).Msg("focus: driver turn failed; raising the blocked gate")
	if _, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, _ := focuspipeline.FailTurn(p, cause.Error(), d.now())
		return next, nil
	}); err != nil {
		log.Warn().Err(err).Msg("focus: record failed turn")
	}
}

// waitIdle waits until no turn holds the run's session (a person's turn
// still finishing, the turn whose hook started this run). False when the
// run was cancelled meanwhile.
func (d *focusDriver) waitIdle(run *focusRun) bool {
	for d.cancels.Running(run.sessionID) {
		select {
		case <-run.ctx.Done():
			return false
		case <-time.After(d.idlePoll):
		}
	}
	return run.ctx.Err() == nil
}

// runVerification runs the plan's verify commands — and in review its
// end-to-end commands after them — and applies the result. It claims the session like a
// turn: the session shows as running, its progress goes to a turn feed a
// console follows on GET /v1/chat/stream, and POST /v1/chat/cancel stops
// it. claimed is false when another turn held the session.
func (d *focusDriver) runVerification(run *focusRun, log zerolog.Logger) (focuspipeline.Action, bool) {
	none := focuspipeline.Action{Kind: focuspipeline.ActionNone}
	claim, release, ok := d.cancels.Claim(run.sessionID)
	if !ok {
		return none, false
	}
	defer release()
	claim.setCancel(run.cancel)
	store := focusStoreFor(d.sessions)
	p, found, err := store.Get(run.sessionID)
	if err != nil || !found || !p.AwaitingVerification {
		return none, true
	}
	commands := focusVerifyCommands(p)

	feed, endFeed := d.feeds.begin(run.sessionID)
	defer endFeed()
	defer d.activity.begin(run.sessionID)()
	stream := newChatStreamWriter(discardResponseWriter{header: http.Header{}}, run.sessionID, log)
	stream.feed = feed
	stream.status("stream_open", "stream connected", "", "", "", "")

	v := focuspipeline.Verification{Passed: true, Results: make([]focuspipeline.VerificationResult, 0, len(commands))}
	for i, command := range commands {
		stream.focusProgress("verifying", command, i+1, len(commands), nil)
		result, err := d.verify(run.ctx, run.sessionID, command)
		if run.ctx.Err() != nil {
			stream.cancelled()
			return none, true
		}
		if err != nil {
			// A command that could not run at all is a failed fact, not a
			// reason to guess: the loop sees the error as its output.
			result = focuspipeline.VerificationResult{Command: command, ExitCode: -1, Excerpt: err.Error()}
		}
		if !result.Passed {
			v.Passed = false
		}
		v.Results = append(v.Results, result)
		stream.focusProgress("verified", command, i+1, len(commands), &result)
	}

	turn := countUserTurns(d.sessions.TranscriptPath(run.sessionID))
	act := none
	updated, _, err := store.Update(run.sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, a, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventVerification, Turn: turn, Verification: &v}, d.now())
		act = a
		return next, err
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: apply verification failed")
		return none, true
	}
	stream.pipeline(updated, focusNextPrompt(act))
	stream.focusDone()
	return act, true
}

// focusVerifyCommands are the commands a verification runs: the plan's
// verify commands, and in review its end-to-end commands after them.
func focusVerifyCommands(p focuspipeline.Pipeline) []string {
	if p.Plan == nil {
		return nil
	}
	lists := [][]string{p.Plan.Verify}
	if p.Current == focuspipeline.StageReview {
		lists = append(lists, p.Plan.E2E)
	}
	var commands []string
	for _, list := range lists {
		for _, c := range list {
			if c = strings.TrimSpace(c); c != "" {
				commands = append(commands, c)
			}
		}
	}
	return commands
}

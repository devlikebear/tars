package tarsserver

import (
	"context"
	"net/http"
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
// lifetime. A run ends when the pipeline asks for nothing more (a gate, a
// decision, a stop), when a step fails, on POST /v1/chat/cancel for the
// session, at shutdown, and when a person starts a turn on the session —
// human input wins; that turn's post-turn hook starts a new run.
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

	mu   sync.Mutex
	runs map[string]*focusRun
	wg   sync.WaitGroup
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

type focusRun struct {
	sessionID string
	role      string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	mu   sync.Mutex
	next focuspipeline.Action
}

func (r *focusRun) setNext(act focuspipeline.Action) {
	r.mu.Lock()
	r.next = act
	r.mu.Unlock()
}

func (r *focusRun) takeNext() focuspipeline.Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	act := r.next
	r.next = focuspipeline.Action{Kind: focuspipeline.ActionNone}
	return act
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
		runs:     map[string]*focusRun{},
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
	d.runTurn = func(ctx context.Context, sessionID, message string) error {
		_, err := runServerChatTurn(ctx, deps, sessionID, message, "")
		return err
	}
	d.verify = func(ctx context.Context, sessionID, command string) (focuspipeline.VerificationResult, error) {
		results, _, err := runTaskVerificationCommands(ctx, deps.store, sessionID, "", []string{command}, focusVerifyTimeout)
		if err != nil || len(results) == 0 {
			return focuspipeline.VerificationResult{Command: command}, err
		}
		r := results[0]
		return focuspipeline.VerificationResult{
			Command: command, ExitCode: r.ExitCode, TimedOut: r.TimedOut,
			Passed: r.Status == "passed", Excerpt: r.Output,
		}, nil
	}
}

// Close stops every run and waits for them: server shutdown.
func (d *focusDriver) Close() {
	if d == nil {
		return
	}
	d.stop()
	d.wg.Wait()
}

// start begins a run that carries out act for sessionID, replacing a run
// already going there (the newest intent wins). role is the role of the
// person whose action led here; the run's turns run as that role.
func (d *focusDriver) start(sessionID string, act focuspipeline.Action, role string) {
	if d == nil || act.Kind == focuspipeline.ActionNone || strings.TrimSpace(sessionID) == "" || d.runTurn == nil {
		return
	}
	d.mu.Lock()
	if d.ctx.Err() != nil {
		d.mu.Unlock()
		return
	}
	previous := d.runs[sessionID]
	ctx, cancel := context.WithCancel(d.ctx)
	run := &focusRun{sessionID: sessionID, role: role, cancel: cancel, done: make(chan struct{})}
	run.ctx = context.WithValue(serverauth.WithRoleContext(ctx, role), focusRunKey{}, run)
	d.runs[sessionID] = run
	d.wg.Add(1)
	d.mu.Unlock()
	var after <-chan struct{}
	if previous != nil {
		previous.cancel()
		after = previous.done
	}
	go d.loop(run, act, after)
}

// afterTurn is the post-turn hook: a turn the run itself started hands the
// next action back to it; any other turn (a person's) starts a run.
func (d *focusDriver) afterTurn(ctx context.Context, sessionID string, act focuspipeline.Action, role string) {
	if d == nil {
		return
	}
	if run := focusRunFrom(ctx); run != nil && run.sessionID == sessionID {
		run.setNext(act)
		return
	}
	d.start(sessionID, act, role)
}

// cancel stops the session's run, if any, and waits (bounded) for it to
// end. It reports whether there was a run.
func (d *focusDriver) cancel(sessionID string) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	run := d.runs[sessionID]
	if run != nil {
		delete(d.runs, sessionID)
	}
	d.mu.Unlock()
	if run == nil {
		return false
	}
	run.cancel()
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

func (d *focusDriver) loop(run *focusRun, act focuspipeline.Action, after <-chan struct{}) {
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
		// The run this one replaced winds down first: one run per session.
		select {
		case <-after:
		case <-run.ctx.Done():
			return
		}
	}
	log := d.logger.With().Str("session_id", run.sessionID).Logger()
	for act.Kind != focuspipeline.ActionNone {
		if !d.waitIdle(run) {
			return
		}
		switch act.Kind {
		case focuspipeline.ActionSendTurn:
			run.setNext(focuspipeline.Action{Kind: focuspipeline.ActionNone})
			if err := d.runTurn(run.ctx, run.sessionID, act.Prompt); err != nil {
				log.Info().Err(err).Msg("focus: driver turn ended without a reply; the loop stops")
				return
			}
			act = run.takeNext()
		case focuspipeline.ActionRunVerification:
			act = d.runVerification(run, log)
		default:
			log.Warn().Str("action", act.Kind).Msg("focus: unknown action")
			return
		}
	}
}

// waitIdle waits until no turn runs on the run's session (a person's turn
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

// runVerification runs the plan's verify commands (end-to-end commands
// belong to review) and applies the result. While it runs the session
// shows as running and its progress goes to a turn feed, so a console
// follows it on GET /v1/chat/stream like a turn, and POST /v1/chat/cancel
// stops it.
func (d *focusDriver) runVerification(run *focusRun, log zerolog.Logger) focuspipeline.Action {
	none := focuspipeline.Action{Kind: focuspipeline.ActionNone}
	store := focusStoreFor(d.sessions)
	p, ok, err := store.Get(run.sessionID)
	if err != nil || !ok || !p.AwaitingVerification {
		return none
	}
	var commands []string
	if p.Plan != nil {
		for _, c := range p.Plan.Verify {
			if c = strings.TrimSpace(c); c != "" {
				commands = append(commands, c)
			}
		}
	}

	feed, endFeed := d.feeds.begin(run.sessionID)
	defer endFeed()
	defer d.activity.begin(run.sessionID)()
	defer d.cancels.Register(run.sessionID, run.cancel)()
	stream := newChatStreamWriter(discardResponseWriter{header: http.Header{}}, run.sessionID, log)
	stream.feed = feed
	stream.status("stream_open", "stream connected", "", "", "", "")

	v := focuspipeline.Verification{Passed: true, Results: make([]focuspipeline.VerificationResult, 0, len(commands))}
	for i, command := range commands {
		stream.focusProgress("verifying", command, i+1, len(commands), nil)
		result, err := d.verify(run.ctx, run.sessionID, command)
		if run.ctx.Err() != nil {
			stream.cancelled()
			return none
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
		return none
	}
	stream.pipeline(updated, focusNextPrompt(act))
	return act
}

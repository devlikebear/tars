package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Who reports a background result (tars#1002, initiative.share_result).
// Every result has exactly one conversational report, and share_result
// only takes the ones that would otherwise have none or only a raw note:
//
//   - The notification bell / desktop notification (cron's "Cron
//     completed"/"Cron failed" events, pulse's ops alerts) is the alert and
//     history channel. It is left as it is for every result; share_result
//     never replaces or repeats it as a notification.
//   - An agent run that a Wait call was blocked on (a chat turn's subagent
//     or sessions_send tool) is already in that turn's answer: not reported.
//   - An agent run's [RUN SUMMARY] note bound for the main session, or a
//     run with no note at all (it ran in its own session, nobody waited):
//     share_result reports it in the main session in place of the note.
//     A note bound for another session stays there — the user is working
//     in that session — and is not reported. A child step of a flow is
//     covered by its root.
//   - A cron job's [CRON] note is the delivery the user chose
//     (delivery_mode session/both), so it is left alone and that result is
//     not reported. A cron result with no session note — delivery_mode
//     daily_log, or any failure (failures write no note) — is reported.
//     delivery_mode none means "do not tell me": never reported. A job bound
//     to its own session is that session's business: not reported.
//   - Reminders belong to cron or initiative.remind, never here.

// maxResultLines caps how many results one report lists; the rest are
// counted ("and N more") and still marked reported.
const (
	maxResultLines   = 3
	maxResultSummary = 160
	maxFollowUpLines = 3
	maxFollowUpLabel = 60
	maxFollowUpGoal  = 90
)

// queuedResult is one finished piece of background work waiting for the
// share_result report. Note/NoteSessionID are the [RUN SUMMARY] note it
// replaced, written back if the report never goes out.
type queuedResult struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`   // agent_run | cron
	Name          string    `json:"name"`   // agent or job name
	Status        string    `json:"status"` // completed | failed
	Summary       string    `json:"summary,omitempty"`
	At            time.Time `json:"at"`
	NoteSessionID string    `json:"note_session_id,omitempty"`
	Note          string    `json:"note,omitempty"`
}

// initiativeResultQueue holds results handed to share_result
// (_shared/initiative/results.json) until a report says them. One that
// waits past initiative.ResultGrace goes back: its replaced note is
// written where it would have gone, and a result with no note is dropped
// (its notification is still in the history).
type initiativeResultQueue struct {
	path     string
	fallback func(ctx context.Context, r queuedResult) error
	logger   zerolog.Logger

	mu      sync.Mutex
	loaded  bool
	entries []queuedResult
}

var resultIDCounter atomic.Uint64

func newInitiativeResultQueue(path string, fallback func(context.Context, queuedResult) error, logger zerolog.Logger) *initiativeResultQueue {
	return &initiativeResultQueue{path: path, fallback: fallback, logger: logger}
}

// Enqueue adds one finished result.
func (q *initiativeResultQueue) Enqueue(r queuedResult) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	r.ID = fmt.Sprintf("res_%d_%d", r.At.UnixNano(), resultIDCounter.Add(1))
	q.entries = append(q.entries, r)
	return q.saveLocked()
}

// Pending returns the report every queued result fits in (oldest first),
// after giving back the ones older than initiative.ResultGrace. nil when
// nothing is queued. The fallback runs after the lock is released.
func (q *initiativeResultQueue) Pending(ctx context.Context, now time.Time) *initiative.Subject {
	q.mu.Lock()
	q.loadLocked()
	expired := q.takeLocked(func(r queuedResult) bool { return now.Sub(r.At) > initiative.ResultGrace })
	var subject *initiative.Subject
	if len(q.entries) > 0 {
		subject = resultSubject(q.entries)
	}
	q.mu.Unlock()
	q.runFallback(ctx, expired)
	return subject
}

// Done removes the results a delivered report covered (its Subject.ID).
func (q *initiativeResultQueue) Done(ids string) {
	want := map[string]bool{}
	for _, id := range strings.Split(ids, ",") {
		if id = strings.TrimSpace(id); id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	q.takeLocked(func(r queuedResult) bool { return want[r.ID] })
}

// FlushAll gives every queued result back — at start-up when share_result
// will not report them.
func (q *initiativeResultQueue) FlushAll(ctx context.Context) {
	q.mu.Lock()
	q.loadLocked()
	taken := q.takeLocked(func(queuedResult) bool { return true })
	q.mu.Unlock()
	q.runFallback(ctx, taken)
}

// Len reports how many results wait in the queue.
func (q *initiativeResultQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	return len(q.entries)
}

func (q *initiativeResultQueue) takeLocked(match func(queuedResult) bool) []queuedResult {
	var taken, kept []queuedResult
	for _, r := range q.entries {
		if match(r) {
			taken = append(taken, r)
		} else {
			kept = append(kept, r)
		}
	}
	if len(taken) == 0 {
		return nil
	}
	q.entries = kept
	if err := q.saveLocked(); err != nil {
		q.logger.Warn().Err(err).Msg("initiative: save result queue failed")
	}
	return taken
}

func (q *initiativeResultQueue) runFallback(ctx context.Context, results []queuedResult) {
	for _, r := range results {
		if r.Note == "" || strings.TrimSpace(r.NoteSessionID) == "" || q.fallback == nil {
			continue
		}
		if err := q.fallback(ctx, r); err != nil {
			q.logger.Warn().Err(err).Str("kind", r.Kind).Msg("initiative: result note fallback failed")
		}
	}
}

func (q *initiativeResultQueue) loadLocked() {
	if q.loaded {
		return
	}
	q.loaded = true
	raw, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err == nil {
		err = json.Unmarshal(raw, &q.entries)
	}
	if err != nil {
		q.logger.Warn().Err(err).Msg("initiative: read result queue failed")
	}
}

func (q *initiativeResultQueue) saveLocked() error {
	if len(q.entries) == 0 {
		if err := os.Remove(q.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	raw, err := json.Marshal(q.entries)
	if err != nil {
		return err
	}
	return atomicwrite.Write(q.path, raw)
}

// resultSubject is the report over every queued result: one line each for
// the oldest maxResultLines, a count for the rest, and every id so a
// delivered report marks them all said.
func resultSubject(entries []queuedResult) *initiative.Subject {
	ids := make([]string, 0, len(entries))
	lines := make([]string, 0, maxResultLines+1)
	for i, r := range entries {
		ids = append(ids, r.ID)
		if i < maxResultLines {
			lines = append(lines, resultLine(r))
		}
	}
	if extra := len(entries) - maxResultLines; extra > 0 {
		lines = append(lines, fmt.Sprintf("and %d more", extra))
	}
	return &initiative.Subject{ID: strings.Join(ids, ","), Text: strings.Join(lines, "\n"), At: entries[0].At}
}

func resultLine(r queuedResult) string {
	kind := "agent run"
	if r.Kind == "cron" {
		kind = "cron job"
	}
	if name := strings.TrimSpace(r.Name); name != "" {
		kind += fmt.Sprintf(" %q", clipRunes(name, maxFollowUpLabel))
	}
	line := kind + " " + r.Status
	if summary := clipRunes(oneLine(r.Summary), maxResultSummary); summary != "" {
		line += ": " + summary
	}
	return line
}

// agentRunReportable applies the ownership rules above to a finished run.
func agentRunReportable(f agentruntime.FinishedRun, mainSessionID string) bool {
	run := f.Run
	mainSessionID = strings.TrimSpace(mainSessionID)
	if f.Awaited || mainSessionID == "" || strings.TrimSpace(run.ParentRunID) != "" {
		return false
	}
	if run.Status != agentruntime.RunStatusCompleted && run.Status != agentruntime.RunStatusFailed {
		return false
	}
	// A run in the main session itself already wrote its answer there.
	if strings.TrimSpace(run.SessionID) == mainSessionID {
		return false
	}
	note := strings.TrimSpace(f.NoteSessionID)
	return note == "" || note == mainSessionID
}

// initiativeAgentResultRouter hands reportable runs to the queue; the
// runtime then skips their main-session [RUN SUMMARY] note.
func initiativeAgentResultRouter(queue *initiativeResultQueue, mainSessionID string, now func() time.Time) agentruntime.ResultRouter {
	if queue == nil {
		return nil
	}
	return func(f agentruntime.FinishedRun) bool {
		if !agentRunReportable(f, mainSessionID) {
			return false
		}
		summary := f.Run.Response
		if f.Run.Status == agentruntime.RunStatusFailed {
			summary = f.Run.Error
		}
		return queue.Enqueue(queuedResult{
			Kind: "agent_run", Name: f.Run.Agent, Status: string(f.Run.Status), Summary: summary,
			At: now().UTC(), NoteSessionID: f.NoteSessionID, Note: f.Note,
		}) == nil
	}
}

// cronResultReportable applies the ownership rules above to a cron run.
func cronResultReportable(job cron.Job, succeeded bool) bool {
	if cronReminderMessage(job) != "" || strings.TrimSpace(job.SessionID) != "" {
		return false
	}
	mode := effectiveCronDeliveryMode(job.DeliveryMode, job.SessionTarget, job.SessionID)
	if mode == "none" {
		return false
	}
	if succeeded {
		return mode == "daily_log"
	}
	return true
}

// withCronResultReport queues a cron run's result for share_result when
// it left no session note. queue nil returns runner unchanged.
func withCronResultReport(runner func(context.Context, cron.Job) (string, error), queue *initiativeResultQueue, now func() time.Time) func(context.Context, cron.Job) (string, error) {
	if runner == nil || queue == nil {
		return runner
	}
	return func(ctx context.Context, job cron.Job) (string, error) {
		response, err := runner(ctx, job)
		if cronResultReportable(job, err == nil) {
			r := queuedResult{Kind: "cron", Name: job.Name, Status: "completed", Summary: summarizeCronResponse(response), At: now().UTC()}
			if err != nil {
				r.Status, r.Summary = "failed", err.Error()
			}
			_ = queue.Enqueue(r)
		}
		return response, err
	}
}

// followUpSubject lists work left unfinished on an earlier local day
// within initiative.FollowUpLookback (tars#1002): unfinished focus
// pipelines (with their pull request once there is one) and visible
// sessions with an active goal, newest first. A session whose pipeline is
// listed is not listed again for its goal. nil when there is none.
func followUpSubject(now time.Time, loc *time.Location, sessions []session.Session, pipelines []focuspipeline.Pipeline) *initiative.Subject {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	since := dayStart.Add(-initiative.FollowUpLookback)
	stopped := func(t time.Time) bool { return t.Before(dayStart) && t.After(since) }

	type item struct {
		at   time.Time
		line string
	}
	var items []item
	visible := map[string]bool{}
	for _, s := range sessions {
		if !s.Hidden && strings.TrimSpace(s.Kind) != "worker" {
			visible[s.ID] = true
		}
	}
	covered := map[string]bool{}
	for _, p := range pipelines {
		if p.FinishedAt != nil || !visible[p.SessionID] || !stopped(p.UpdatedAt) {
			continue
		}
		covered[p.SessionID] = true
		items = append(items, item{p.UpdatedAt, fmt.Sprintf("focus %q: %s", clipRunes(oneLine(p.Goal), maxFollowUpLabel), followUpStage(p))})
	}
	for _, s := range sessions {
		if !visible[s.ID] || covered[s.ID] || !s.Goal.IsActive() || !stopped(s.UpdatedAt) {
			continue
		}
		label := strings.TrimSpace(s.Title)
		if label == "" {
			label = "session"
		}
		items = append(items, item{s.UpdatedAt, fmt.Sprintf("session %q goal: %s", clipRunes(oneLine(label), maxFollowUpLabel), clipRunes(oneLine(s.Goal.Description), maxFollowUpGoal))})
	}
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	lines := make([]string, 0, maxFollowUpLines+1)
	for i, it := range items {
		if i == maxFollowUpLines {
			lines = append(lines, fmt.Sprintf("and %d more", len(items)-maxFollowUpLines))
			break
		}
		lines = append(lines, it.line)
	}
	return &initiative.Subject{Text: strings.Join(lines, "\n"), At: items[0].at}
}

// followUpStage says where an unfinished pipeline stopped, naming its pull
// request once one exists.
func followUpStage(p focuspipeline.Pipeline) string {
	pr := "the pull request"
	if p.PR != nil && p.PR.Number > 0 {
		pr = fmt.Sprintf("pull request #%d", p.PR.Number)
	}
	var where string
	switch p.CurrentKind() {
	case focuspipeline.StagePlan:
		where = "stopped at planning"
	case focuspipeline.StageBuild:
		where = "stopped while building"
	case focuspipeline.StageReview:
		where = "stopped in review"
	case focuspipeline.StagePR:
		where = "ready to open " + pr
	case focuspipeline.StagePRReview:
		where = pr + " waiting on checks or review"
	case focuspipeline.StageMerge:
		where = pr + " waiting to merge"
	default:
		where = "unfinished"
	}
	if !p.Active() && p.Current != "" {
		where += " (blocked)"
	}
	return where
}

// initiativeFollowUpSource reads sessions and focus pipelines for
// followUpSubject. Errors read as nothing to follow up.
func initiativeFollowUpSource(store *session.Store, loc *time.Location) func(time.Time) *initiative.Subject {
	if store == nil {
		return nil
	}
	focus := focusStoreFor(store)
	return func(now time.Time) *initiative.Subject {
		sessions, err := store.List()
		if err != nil {
			return nil
		}
		var pipelines []focuspipeline.Pipeline
		if focus != nil {
			pipelines, _ = focus.List()
		}
		return followUpSubject(now, loc, sessions, pipelines)
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clipRunes(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if limit <= 0 || len(r) <= limit {
		return string(r)
	}
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}

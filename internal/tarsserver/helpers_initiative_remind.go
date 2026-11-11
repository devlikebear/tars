package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/rs/zerolog"
)

// initiativeReminderQueue holds cron reminders handed to initiative
// (initiative.remind, tars#1001) until the speak path delivers them. It is a
// file (_shared/initiative/reminders.json) so a restart does not lose one.
// A reminder that waits longer than initiative.ReminderGrace — typing, a
// turn in flight, nobody reachable, a failed compose — goes back to cron's
// own delivery (fallback), so it is late by at most the grace and never
// dropped.
type initiativeReminderQueue struct {
	path     string
	fallback func(ctx context.Context, h cronReminderHandoff) error
	logger   zerolog.Logger

	mu      sync.Mutex
	loaded  bool
	entries []queuedReminder
}

type queuedReminder struct {
	ID string `json:"id"`
	cronReminderHandoff
}

var reminderIDCounter atomic.Uint64

func newInitiativeReminderQueue(path string, fallback func(context.Context, cronReminderHandoff) error, logger zerolog.Logger) *initiativeReminderQueue {
	return &initiativeReminderQueue{path: path, fallback: fallback, logger: logger}
}

// bindFallback sets how a reminder goes back to cron's own delivery. The
// cron runner binds it once its sender exists, before any tick runs.
func (q *initiativeReminderQueue) bindFallback(fallback func(context.Context, cronReminderHandoff) error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.fallback = fallback
}

// Enqueue adds one fired reminder.
func (q *initiativeReminderQueue) Enqueue(h cronReminderHandoff) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	id := fmt.Sprintf("rem_%d_%d", h.At.UnixNano(), reminderIDCounter.Add(1))
	q.entries = append(q.entries, queuedReminder{ID: id, cronReminderHandoff: h})
	return q.saveLocked()
}

// Due returns the oldest queued reminder that is due at now, after handing
// every one older than initiative.ReminderGrace back to cron's delivery.
// The fallback runs after the lock is released: it writes a transcript and
// may send to Telegram, and a reminder firing meanwhile must not wait on it.
func (q *initiativeReminderQueue) Due(ctx context.Context, now time.Time) *initiative.Subject {
	q.mu.Lock()
	q.loadLocked()
	expired := q.takeLocked(func(e queuedReminder) bool { return now.Sub(e.At) > initiative.ReminderGrace })
	var subject *initiative.Subject
	for _, e := range q.entries {
		if !e.At.After(now) {
			subject = &initiative.Subject{ID: e.ID, Text: e.Text, At: e.At}
			break
		}
	}
	q.mu.Unlock()
	q.runFallback(ctx, expired)
	return subject
}

// Done removes a reminder the speak path delivered.
func (q *initiativeReminderQueue) Done(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	q.takeLocked(func(e queuedReminder) bool { return e.ID == id })
}

// FlushAll hands every queued reminder back to cron's delivery — at start-up
// when initiative will not deliver them (remind turned off, or initiative
// no longer live).
func (q *initiativeReminderQueue) FlushAll(ctx context.Context) {
	q.mu.Lock()
	q.loadLocked()
	taken := q.takeLocked(func(queuedReminder) bool { return true })
	q.mu.Unlock()
	q.runFallback(ctx, taken)
}

// Len reports how many reminders wait in the queue.
func (q *initiativeReminderQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.loadLocked()
	return len(q.entries)
}

// takeLocked removes and returns the entries match selects, saving the file
// when anything was removed.
func (q *initiativeReminderQueue) takeLocked(match func(queuedReminder) bool) []queuedReminder {
	var taken, kept []queuedReminder
	for _, e := range q.entries {
		if match(e) {
			taken = append(taken, e)
		} else {
			kept = append(kept, e)
		}
	}
	if len(taken) == 0 {
		return nil
	}
	q.entries = kept
	if err := q.saveLocked(); err != nil {
		q.logger.Warn().Err(err).Msg("initiative: save reminder queue failed")
	}
	return taken
}

func (q *initiativeReminderQueue) runFallback(ctx context.Context, entries []queuedReminder) {
	if len(entries) == 0 {
		return
	}
	q.mu.Lock()
	fallback := q.fallback
	q.mu.Unlock()
	for _, e := range entries {
		if fallback == nil {
			q.logger.Warn().Str("job_id", strings.TrimSpace(e.Job.ID)).Msg("initiative: reminder dropped, no cron fallback bound")
			continue
		}
		if err := fallback(ctx, e.cronReminderHandoff); err != nil {
			q.logger.Warn().Err(err).Str("job_id", strings.TrimSpace(e.Job.ID)).Msg("initiative: reminder fallback delivery failed")
		}
	}
}

func (q *initiativeReminderQueue) loadLocked() {
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
		q.logger.Warn().Err(err).Msg("initiative: read reminder queue failed")
	}
}

func (q *initiativeReminderQueue) saveLocked() error {
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

// initiativeRemindRouter returns the cron reminder router for
// initiative.remind, or nil when cron keeps every reminder (initiative off,
// shadow mode, or remind off). It takes a reminder only when cron would
// otherwise deliver it to the main session: not bound to another session,
// and with a delivery mode other than none.
func initiativeRemindRouter(queue *initiativeReminderQueue, mainSessionID string) cronReminderRouter {
	if queue == nil {
		return nil
	}
	mainSessionID = strings.TrimSpace(mainSessionID)
	return func(_ context.Context, h cronReminderHandoff) bool {
		if strings.TrimSpace(h.Job.SessionID) != "" {
			return false
		}
		if effectiveCronDeliveryMode(h.Job.DeliveryMode, h.Job.SessionTarget, h.Job.SessionID) == "none" {
			return false
		}
		if target := strings.TrimSpace(h.TargetSessionID); target != "" && target != mainSessionID {
			return false
		}
		return queue.Enqueue(h) == nil
	}
}

package initiative

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/textutil"
)

const defaultLedgerMaxBytes = 5 << 20

// Entry is one recorded decision. It never holds user text or composed
// speech: only signals, probabilities and the outcome, so the ledger is
// safe to keep and to use as tuning data (tars#1003).
type Entry struct {
	// ID identifies this entry so a delivered speak attempt's assistant
	// message can reference it (its Initiative metadata) without storing
	// the words twice. Always set, even on a tick that decides none.
	// tars#1220.
	ID   string    `json:"id,omitempty"`
	At   time.Time `json:"at"`
	Mode string    `json:"mode"`
	Decision
	Signals GoSignals   `json:"signals"`
	Text    TextSignals `json:"text"`
	// Called reports whether this tick actually invoked the text-signal
	// backend (success or error) — as opposed to a cached, skipped, or
	// never-attempted reading — and counts toward the daily call cap.
	Called bool `json:"called"`
	// Composed reports whether this tick actually invoked the speak
	// composer (live mode only, success or failure) — counts toward
	// DailySpeakCalls and the speak backoff schedule, independent of
	// Delivery. tars#1220.
	Composed bool `json:"composed,omitempty"`
	// Delivery is live mode's outcome for a Speak intent: delivered,
	// unreachable, skipped, or error. Empty in shadow mode and on every
	// tick that did not decide to speak. tars#1220.
	Delivery string `json:"delivery,omitempty"`
	// DeliveryReason explains Delivery with a fixed reason code — never
	// user or assistant text (tars#1003's rule applies here too):
	// no_console (unreachable); backoff, daily_speak_cap,
	// speak_unavailable, read_error, busy (skipped, Composed false — no
	// compose call was made); busy, user_spoke (skipped, Composed true —
	// Superseded: Compose ran but the text was discarded unwritten, not a
	// failure, see SpeakOutcome.Superseded); compose_error, compose_empty,
	// compose_tool_attempt, write_error (error, Composed true — Compose
	// ran and failed, or the write itself did); delivered (delivered).
	DeliveryReason string `json:"delivery_reason,omitempty"`
	// Channel is which transport a Delivered speak used: "console" or
	// "telegram" (tars#1221 P2c). Empty whenever Delivery is not
	// "delivered". Go chooses the channel before any compose call:
	// console whenever connected (so "console" is recorded on every
	// console delivery regardless of Config.Telegram — review f3, this
	// field is not telegram-only), else "telegram" only when
	// Config.Telegram is on and a default paired chat resolves. Never
	// both, and never "telegram" when Config.Telegram is off.
	Channel string `json:"channel,omitempty"`
	// ChannelError is a fixed reason code for a channel-specific send
	// failure that happened AFTER the main session's write already
	// succeeded (so Delivery stays "delivered" — tars#1003/#1220's "the
	// ledger never holds words" rule: never the error text, a chat id, or
	// a token). Empty on success. Only ever set when Channel ==
	// "telegram" today.
	ChannelError string `json:"channel_error,omitempty"`
	Body         string `json:"body,omitempty"`
	LatencyMS    int64  `json:"latency_ms"`
	Error        string `json:"error,omitempty"`
	// Kind distinguishes a normal tick entry ("") from a reply-tracking
	// outcome entry ("outcome", tars#1221): a separate ledger line recorded
	// when a previously delivered speak (Ref, an earlier entry's ID)
	// resolves to Outcome. An outcome entry carries no
	// Decision/Signals/Text/Delivery of its own and never holds user or
	// assistant text, the same tars#1003 rule every other Entry field
	// follows.
	Kind string `json:"kind,omitempty"`
	// Ref is the ID of the delivered-speak entry this outcome resolves,
	// set only when Kind == "outcome".
	Ref string `json:"ref,omitempty"`
	// Outcome is replied, ignored, or stopped — set only when Kind ==
	// "outcome". See OutcomeReplied/OutcomeIgnored/OutcomeStopped.
	Outcome string `json:"outcome,omitempty"`
}

// Ledger is an append-only JSONL file that rotates to <path>.1 when full.
type Ledger struct {
	path     string
	maxBytes int64
	mu       sync.Mutex
}

func OpenLedger(path string, maxBytes int64) *Ledger {
	if maxBytes <= 0 {
		maxBytes = defaultLedgerMaxBytes
	}
	return &Ledger{path: path, maxBytes: maxBytes}
}

func (l *Ledger) Append(e Entry) error {
	if l == nil {
		return nil
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(l.path); err == nil && info.Size()+int64(len(line))+1 > l.maxBytes {
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// Recent returns up to n entries from the current file, newest last. A line
// that is not an entry (cut off by a crash, corrupt, oversized) is skipped
// and the rest are kept: pacing state is rebuilt from this file, so one bad
// line must not hide the day's count and the cooldown. On a read error the
// entries read before it are returned with the error.
func (l *Ledger) Recent(n int) ([]Entry, error) {
	if l == nil {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []Entry
	readErr := textutil.EachLine(f, func(line []byte) error {
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
		return nil
	})
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out, readErr
}

// historyFrom rebuilds pacing state from recorded entries, so a restart
// keeps the daily cap, cooldown and spacing.
//
// Whether a Speak entry counts toward Today/LastSpokeAt/LastCheckInAt uses
// the entry's own recorded Mode, not the runtime's current configuration:
// a shadow entry (or one from before tars#1220, which is always "shadow")
// always counts as spoken (shadow has no delivery concept, every Speak
// decision is the pacing truth), while a live entry counts only when it
// was actually Delivery == DeliveryDelivered. This keeps historyFrom and
// applyLocked agreeing even across a mode change, since both key off the
// same per-entry Mode.
func historyFrom(entries []Entry, now time.Time, loc *time.Location) History {
	var h History
	today := now.In(loc).Format("2006-01-02")
	for _, e := range entries {
		if e.spoken() {
			if e.At.In(loc).Format("2006-01-02") == today {
				h.Today++
			}
			if e.At.After(h.LastSpokeAt) {
				h.LastSpokeAt = e.At
			}
			if e.Intent == IntentCheckIn && e.At.After(h.LastCheckInAt) {
				h.LastCheckInAt = e.At
			}
		}
		if e.Intent == IntentBodyOnly && e.At.After(h.LastBodyAt) {
			h.LastBodyAt = e.At
		}
		if e.Called {
			if e.At.In(loc).Format("2006-01-02") == today {
				h.TextCallsToday++
			}
			if e.At.After(h.LastTextCallAt) {
				h.LastTextCallAt = e.At
			}
			if e.Text.Source == "error" {
				h.TextReadFailCount++
			} else {
				h.TextReadFailCount = 0
			}
		}
		if e.Composed {
			if e.At.In(loc).Format("2006-01-02") == today {
				h.SpeakCallsToday++
			}
			if e.At.After(h.LastSpeakCallAt) {
				h.LastSpeakCallAt = e.At
			}
			// Mirrors applyLocked's own switch (runtime.go): a Superseded
			// attempt (Composed true, Delivery skipped — the claim was
			// lost, or the user had already spoken) is neither a success
			// nor a failure, so a restart must not treat it as one either
			// — otherwise historyFrom would fabricate backoff from a
			// ledger applyLocked itself never would have (tars#1220
			// review).
			switch e.Delivery {
			case DeliveryDelivered:
				h.SpeakFailCount = 0
			case DeliveryError:
				h.SpeakFailCount++
			}
		}
	}
	h.ConsecutiveIgnored, h.LastStoppedAt, h.PendingReplies = ReplyState(entries)
	return h
}

// spoken reports whether e counts as an actually-spoken initiative for
// pacing purposes (Today, LastSpokeAt, LastCheckInAt, cooldown, daily cap).
// See historyFrom's comment for why this keys off e.Mode rather than the
// runtime's current configuration.
func (e Entry) spoken() bool {
	if !e.Speak {
		return false
	}
	if e.Mode != ModeLive {
		return true
	}
	return e.Delivery == DeliveryDelivered
}

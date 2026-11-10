package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// The recurrence ledger (promotion.go) lives in the repository's harness
// library folder as recurrence.json: the recurrence keys of the last
// recurrenceKeep pipelines that opted in to verify promotion, and the keys
// a promotion already tried without a check surviving (so the same
// candidate is not sent to the model again until it recurs once more).

const (
	recurrenceFile = "recurrence.json"
	// recurrenceKeep caps the pipelines the ledger remembers.
	recurrenceKeep = 30
	// recurrenceLedgerVersion is the file format version.
	recurrenceLedgerVersion = 1
)

// RecurrenceLedger is one repository's recurrence record.
type RecurrenceLedger struct {
	Version int `json:"version"`
	// Pipelines are the recorded pipelines, oldest first.
	Pipelines []RecurrencePipeline `json:"pipelines"`
	// Attempts are the keys a promotion tried without a check surviving,
	// with the count it tried at and why it failed (a reason code).
	Attempts map[string]PromotionAttempt `json:"attempts,omitempty"`
}

// RecurrencePipeline is one finished pipeline's keys.
type RecurrencePipeline struct {
	SessionID  string          `json:"session_id"`
	Goal       string          `json:"goal,omitempty"`
	PRURL      string          `json:"pr_url,omitempty"`
	RecordedAt time.Time       `json:"recorded_at"`
	Hits       []RecurrenceHit `json:"hits"`
}

// PromotionAttempt is a candidate tried without a surviving check.
type PromotionAttempt struct {
	Count  int       `json:"count"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// RecurrenceCandidate is a key that recurs often enough to promote.
type RecurrenceCandidate struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Count is how many pipelines of the window hit the key, Sessions
	// which (oldest first), and Hits their hits.
	Count    int             `json:"count"`
	Sessions []string        `json:"sessions"`
	Hits     []RecurrenceHit `json:"hits"`
}

// withPipeline records a pipeline's hits: replaced when the pipeline was
// recorded before, appended otherwise, and the oldest dropped past
// recurrenceKeep.
func (l RecurrenceLedger) withPipeline(rp RecurrencePipeline) RecurrenceLedger {
	out := RecurrenceLedger{Version: recurrenceLedgerVersion, Attempts: l.Attempts}
	for _, p := range l.Pipelines {
		if p.SessionID != rp.SessionID {
			out.Pipelines = append(out.Pipelines, p)
		}
	}
	out.Pipelines = append(out.Pipelines, rp)
	if n := len(out.Pipelines); n > recurrenceKeep {
		out.Pipelines = out.Pipelines[n-recurrenceKeep:]
	}
	return out
}

// RecurrenceCandidates are the keys of the current pipeline (current: its
// hits) that PromotionThreshold or more of the ledger's last
// PromotionWindow pipelines hit — most pipelines first, then by key — left
// out when exclude holds them (a check already proposed or enforced) or a
// promotion already tried them at the same count. At most
// PromotionMaxCandidates. Only the current pipeline's keys: its fix turns
// are what the server replays a proposed check on.
func RecurrenceCandidates(l RecurrenceLedger, current []RecurrenceHit, exclude map[string]bool) []RecurrenceCandidate {
	window := l.Pipelines
	if len(window) > PromotionWindow {
		window = window[len(window)-PromotionWindow:]
	}
	var out []RecurrenceCandidate
	for _, hit := range current {
		if exclude[hit.Key] {
			continue
		}
		c := RecurrenceCandidate{Key: hit.Key, Kind: hit.Kind}
		for _, p := range window {
			for _, h := range p.Hits {
				if h.Key == hit.Key {
					c.Count++
					c.Sessions = append(c.Sessions, p.SessionID)
					c.Hits = append(c.Hits, h)
					break
				}
			}
		}
		if c.Count < PromotionThreshold {
			continue
		}
		if a, ok := l.Attempts[hit.Key]; ok && a.Count >= c.Count {
			continue
		}
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b RecurrenceCandidate) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Key, b.Key)
	})
	if len(out) > PromotionMaxCandidates {
		out = out[:PromotionMaxCandidates]
	}
	return out
}

// RecordRecurrence adds a finished pipeline's hits to the repository's
// ledger and returns the ledger as it now is.
func (l HarnessLibrary) RecordRecurrence(rp RecurrencePipeline) (RecurrenceLedger, error) {
	unlock := l.lock()
	defer unlock()
	ledger, err := l.readRecurrence()
	if err != nil {
		return RecurrenceLedger{}, err
	}
	ledger = ledger.withPipeline(rp)
	return ledger, l.writeRecurrence(ledger)
}

// Recurrence reads the repository's ledger (empty when never written).
func (l HarnessLibrary) Recurrence() (RecurrenceLedger, error) {
	unlock := l.lock()
	defer unlock()
	return l.readRecurrence()
}

// RecordPromotionAttempts remembers candidates a promotion tried without a
// surviving check (reasons: key → reason code), at their current count.
func (l HarnessLibrary) RecordPromotionAttempts(cands []RecurrenceCandidate, reasons map[string]string, now time.Time) error {
	if len(reasons) == 0 {
		return nil
	}
	unlock := l.lock()
	defer unlock()
	ledger, err := l.readRecurrence()
	if err != nil {
		return err
	}
	if ledger.Attempts == nil {
		ledger.Attempts = map[string]PromotionAttempt{}
	}
	for _, c := range cands {
		if reason, ok := reasons[c.Key]; ok {
			ledger.Attempts[c.Key] = PromotionAttempt{Count: c.Count, Reason: reason, At: now.UTC()}
		}
	}
	ledger.Version = recurrenceLedgerVersion
	return l.writeRecurrence(ledger)
}

func (l HarnessLibrary) readRecurrence() (RecurrenceLedger, error) {
	raw, err := os.ReadFile(filepath.Join(l.dir, recurrenceFile))
	if errors.Is(err, os.ErrNotExist) {
		return RecurrenceLedger{Version: recurrenceLedgerVersion}, nil
	}
	if err != nil {
		return RecurrenceLedger{}, err
	}
	var out RecurrenceLedger
	if err := json.Unmarshal(raw, &out); err != nil {
		return RecurrenceLedger{}, fmt.Errorf("read recurrence ledger: %w", err)
	}
	return out, nil
}

func (l HarnessLibrary) writeRecurrence(ledger RecurrenceLedger) error {
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Write(filepath.Join(l.dir, recurrenceFile), append(raw, '\n'))
}

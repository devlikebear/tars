package initiative

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/devlikebear/tars/internal/textutil"
)

// DatasetRecord is one exported tick of the initiative ledger (tars#1003):
// the signals the policy saw, what it decided, how a live delivery went,
// the reply-tracking outcome joined from its separate outcome line, and an
// empty slot for the user's own correction. It holds the same facts the
// ledger does and nothing more — timestamps, booleans, probabilities,
// intents and fixed reason codes. Entry.Error (a backend's error text) and
// Entry.Body are left out, so an export never carries more than the
// ledger's tuning facts even if an error message quoted something.
type DatasetRecord struct {
	ID             string      `json:"id"`
	At             time.Time   `json:"at"`
	Mode           string      `json:"mode"`
	Decision       Decision    `json:"decision"`
	Signals        GoSignals   `json:"signals"`
	Text           TextSignals `json:"text"`
	Called         bool        `json:"called,omitempty"`
	Composed       bool        `json:"composed,omitempty"`
	Delivery       string      `json:"delivery,omitempty"`
	DeliveryReason string      `json:"delivery_reason,omitempty"`
	Channel        string      `json:"channel,omitempty"`
	ChannelError   string      `json:"channel_error,omitempty"`
	Outcome        string      `json:"outcome,omitempty"`
	OutcomeAt      *time.Time  `json:"outcome_at,omitempty"`
	// Answer is the button pressed on a delivered check-in (ok, snooze,
	// quiet_today), when there was one.
	Answer string       `json:"answer,omitempty"`
	Label  DatasetLabel `json:"label"`
}

// DatasetLabel is the user's correction of one record. Every field is a
// pointer and serialized without omitempty, so an export shows each slot
// as null until someone fills it in: intent/speak say what the policy
// should have decided, and the three text booleans say what the user's own
// words actually meant (the ground truth `tars initiative calibrate` scores
// jev probabilities against). There is deliberately no free-text note —
// the dataset follows the ledger's rule of never holding words.
type DatasetLabel struct {
	Intent         *Intent `json:"intent"`
	Speak          *bool   `json:"speak"`
	QuietRequested *bool   `json:"quiet_requested"`
	UserStrained   *bool   `json:"user_strained"`
	SpecialDay     *bool   `json:"special_day"`
}

// Empty reports whether no slot of the label has been filled in.
func (l DatasetLabel) Empty() bool {
	return l.Intent == nil && l.Speak == nil && l.QuietRequested == nil && l.UserStrained == nil && l.SpecialDay == nil
}

// ReadLedgerEntries reads every entry of a ledger, the rotated <path>.1
// first and then <path>, oldest first. A missing file is not an error. A
// line that is not an entry is skipped, as Ledger.Recent does.
func ReadLedgerEntries(path string) ([]Entry, error) {
	var out []Entry
	for _, p := range []string{path + ".1", path} {
		f, err := os.Open(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return out, err
		}
		readErr := textutil.EachLine(f, func(line []byte) error {
			var e Entry
			if json.Unmarshal(line, &e) == nil {
				out = append(out, e)
			}
			return nil
		})
		_ = f.Close()
		if readErr != nil {
			return out, readErr
		}
	}
	return out, nil
}

// recordID is the key labels are matched by. Entries written before
// tars#1220 have no ID; their timestamp is unique enough within one ledger.
func recordID(e Entry) string {
	if e.ID != "" {
		return e.ID
	}
	return "at:" + e.At.UTC().Format(time.RFC3339Nano)
}

// BuildDataset turns ledger entries (oldest first) into one record per
// tick, with each outcome line joined onto the delivered speak it resolves
// (Ref). Outcome lines themselves are not records. Ticks before since are
// dropped (a zero since keeps everything). labels, keyed by record ID,
// fill the Label slot.
func BuildDataset(entries []Entry, since time.Time, labels map[string]DatasetLabel) []DatasetRecord {
	var out []DatasetRecord
	index := map[string]int{}
	for _, e := range entries {
		if e.Kind == entryKindAnswer {
			if i, ok := index[e.Ref]; ok {
				out[i].Answer = e.Answer
			}
			continue
		}
		if e.Kind == entryKindOutcome {
			if i, ok := index[e.Ref]; ok && out[i].Outcome == "" {
				at := e.At
				out[i].Outcome, out[i].OutcomeAt = e.Outcome, &at
			}
			continue
		}
		if !since.IsZero() && e.At.Before(since) {
			continue
		}
		rec := DatasetRecord{
			ID:             recordID(e),
			At:             e.At,
			Mode:           e.Mode,
			Decision:       e.Decision,
			Signals:        e.Signals,
			Text:           e.Text,
			Called:         e.Called,
			Composed:       e.Composed,
			Delivery:       e.Delivery,
			DeliveryReason: e.DeliveryReason,
			Channel:        e.Channel,
			ChannelError:   e.ChannelError,
			Label:          labels[recordID(e)],
		}
		if e.ID != "" {
			index[e.ID] = len(out)
		}
		out = append(out, rec)
	}
	return out
}

// ReadDataset reads exported records (one JSON object per line). Blank
// lines are skipped; any other line that does not parse is an error naming
// its line number, since a dataset is edited by hand and a typo should not
// silently drop a label.
func ReadDataset(r io.Reader) ([]DatasetRecord, error) {
	var out []DatasetRecord
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var rec DatasetRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return out, fmt.Errorf("line %d: %w", line, err)
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

// LabelsFrom collects the non-empty labels of records, keyed by ID — what a
// re-export merges back so corrections survive a fresh export.
func LabelsFrom(records []DatasetRecord) map[string]DatasetLabel {
	out := map[string]DatasetLabel{}
	for _, r := range records {
		if r.ID != "" && !r.Label.Empty() {
			out[r.ID] = r.Label
		}
	}
	return out
}

// WriteDataset writes records as JSONL.
func WriteDataset(w io.Writer, records []DatasetRecord) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

package initiative

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultLedgerMaxBytes = 5 << 20

// Entry is one recorded decision. It never holds user text: only signals,
// probabilities and the outcome, so the ledger is safe to keep and to use
// as tuning data (tars#1003).
type Entry struct {
	At   time.Time `json:"at"`
	Mode string    `json:"mode"`
	Decision
	Signals   GoSignals   `json:"signals"`
	Text      TextSignals `json:"text"`
	Body      string      `json:"body,omitempty"`
	LatencyMS int64       `json:"latency_ms"`
	Error     string      `json:"error,omitempty"`
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

// Recent returns up to n entries from the current file, newest last.
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
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out, sc.Err()
}

// historyFrom rebuilds pacing state from recorded entries, so a restart
// keeps the daily cap, cooldown and spacing.
func historyFrom(entries []Entry, now time.Time, loc *time.Location) History {
	var h History
	today := now.In(loc).Format("2006-01-02")
	for _, e := range entries {
		if e.Speak {
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
	}
	return h
}

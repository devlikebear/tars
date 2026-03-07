package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (t *Tracker) usagePathFor(ts time.Time) string {
	return filepath.Join(t.usageDir, ts.UTC().Format("2006-01-02")+".jsonl")
}

func (t *Tracker) Record(entry Entry) error {
	if t == nil {
		return fmt.Errorf("usage tracker is nil")
	}

	e := normalizeEntry(entry, t.nowFn)
	path := t.usagePathFor(e.Timestamp)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(payload, '\n'))
	return err
}

func normalizeEntry(entry Entry, nowFn func() time.Time) Entry {
	e := entry
	if e.Timestamp.IsZero() {
		e.Timestamp = nowFn().UTC()
	}
	e.Provider = strings.TrimSpace(strings.ToLower(e.Provider))
	e.Model = strings.TrimSpace(e.Model)
	e.Source = normalizeCallMeta(CallMeta{Source: e.Source}).Source
	e.SessionID = strings.TrimSpace(e.SessionID)
	e.ProjectID = strings.TrimSpace(e.ProjectID)
	e.RunID = strings.TrimSpace(e.RunID)
	return e
}

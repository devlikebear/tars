package sentinel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type eventStore struct {
	path       string
	maxRecords int
}

func newEventStore(path string, maxRecords int) eventStore {
	return eventStore{
		path:       strings.TrimSpace(path),
		maxRecords: maxRecords,
	}
}

func (s eventStore) enabled() bool {
	return strings.TrimSpace(s.path) != ""
}

func (s eventStore) read() ([]Event, error) {
	if !s.enabled() {
		return []Event{}, nil
	}
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer f.Close()

	events := make([]Event, 0)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return nil, fmt.Errorf("decode event jsonl: %w", err)
		}
		events = append(events, evt)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return trimEventRecords(events, s.maxRecords), nil
}

func (s eventStore) write(events []Event) error {
	if !s.enabled() {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(f)
	for _, evt := range trimEventRecords(events, s.maxRecords) {
		raw, err := json.Marshal(evt)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if _, err := writer.Write(raw); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func trimEventRecords(events []Event, max int) []Event {
	if len(events) == 0 {
		return []Event{}
	}
	if max > 0 && len(events) > max {
		events = events[len(events)-max:]
	}
	out := make([]Event, 0, len(events))
	var seq int64
	for _, evt := range events {
		copyEvt := evt
		if copyEvt.ID > seq {
			seq = copyEvt.ID
		}
		if copyEvt.ID <= 0 {
			seq++
			copyEvt.ID = seq
		}
		if len(copyEvt.Meta) > 0 {
			meta := make(map[string]any, len(copyEvt.Meta))
			for key, value := range copyEvt.Meta {
				meta[key] = value
			}
			copyEvt.Meta = meta
		}
		out = append(out, copyEvt)
	}
	return out
}

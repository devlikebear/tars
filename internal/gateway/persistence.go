package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	runsSnapshotFilename     = "runs.json"
	channelsSnapshotFilename = "channels.json"
)

type snapshotStore struct {
	dir          string
	runsPath     string
	channelsPath string
}

type runsSnapshot struct {
	Runs []Run `json:"runs"`
}

type channelsSnapshot struct {
	Channels map[string][]ChannelMessage `json:"channels"`
}

func newSnapshotStore(dir string) snapshotStore {
	trimmed := filepath.Clean(dir)
	return snapshotStore{
		dir:          trimmed,
		runsPath:     filepath.Join(trimmed, runsSnapshotFilename),
		channelsPath: filepath.Join(trimmed, channelsSnapshotFilename),
	}
}

func (s snapshotStore) readRuns() ([]Run, error) {
	var payload runsSnapshot
	if err := readJSONFile(s.runsPath, &payload); err != nil {
		return nil, err
	}
	return append([]Run(nil), payload.Runs...), nil
}

func (s snapshotStore) writeRuns(runs []Run) error {
	payload := runsSnapshot{
		Runs: append([]Run(nil), runs...),
	}
	return writeJSONAtomic(s.runsPath, payload)
}

func (s snapshotStore) readChannels() (map[string][]ChannelMessage, error) {
	var payload channelsSnapshot
	if err := readJSONFile(s.channelsPath, &payload); err != nil {
		return nil, err
	}
	out := make(map[string][]ChannelMessage, len(payload.Channels))
	for channelID, messages := range payload.Channels {
		out[channelID] = append([]ChannelMessage(nil), messages...)
	}
	return out, nil
}

func (s snapshotStore) writeChannels(channels map[string][]ChannelMessage) error {
	payload := channelsSnapshot{
		Channels: trimChannels(channels, 0),
	}
	return writeJSONAtomic(s.channelsPath, payload)
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read json file %q: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode json file %q: %w", path, err)
	}
	return nil
}

func writeJSONAtomic(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %q: %w", path, err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %q: %w", path, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp for %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp for %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp for %q: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp for %q: %w", path, err)
	}
	cleanup = false
	return nil
}

func trimRuns(runs []Run, max int) []Run {
	if max > 0 && len(runs) > max {
		runs = runs[len(runs)-max:]
	}
	return append([]Run(nil), runs...)
}

func trimChannels(channels map[string][]ChannelMessage, maxPerChannel int) map[string][]ChannelMessage {
	out := make(map[string][]ChannelMessage, len(channels))
	for channelID, messages := range channels {
		list := append([]ChannelMessage(nil), messages...)
		if maxPerChannel > 0 && len(list) > maxPerChannel {
			list = list[len(list)-maxPerChannel:]
		}
		out[channelID] = list
	}
	return out
}

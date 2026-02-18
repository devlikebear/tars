package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (r *Runtime) persistArchiveSnapshot(runs []Run, channels map[string][]ChannelMessage) error {
	if r == nil || !r.opts.GatewayArchiveEnabled {
		return nil
	}
	archiveDir := strings.TrimSpace(r.opts.GatewayArchiveDir)
	if archiveDir == "" {
		return nil
	}
	now := r.nowFn().UTC()
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	_ = cleanupGatewayArchiveFiles(archiveDir, now, r.opts.GatewayArchiveRetentionDays)

	payload := map[string]any{
		"time":     now.Format(time.RFC3339),
		"runs":     runs,
		"channels": channels,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	path, err := selectGatewayArchiveFile(archiveDir, now, int64(len(raw)+1), int64(r.opts.GatewayArchiveMaxFileBytes))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}
	return nil
}

func selectGatewayArchiveFile(dir string, now time.Time, nextSize int64, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 10485760
	}
	stamp := now.UTC().Format("20060102")
	for idx := 0; idx < 1000; idx++ {
		name := fmt.Sprintf("gateway-%s.jsonl", stamp)
		if idx > 0 {
			name = fmt.Sprintf("gateway-%s-%d.jsonl", stamp, idx)
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return path, nil
			}
			return "", err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Size() == 0 || info.Size()+nextSize <= maxBytes {
			return path, nil
		}
	}
	return "", fmt.Errorf("gateway archive file rotation exhausted")
}

func cleanupGatewayArchiveFiles(dir string, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "gateway-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
	return nil
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionRetentionDefaultsAndOverrides(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionAutoArchiveDays != 7 || cfg.SessionAutoDeleteDays != 30 {
		t.Fatalf("defaults = archive %d / delete %d, want 7 / 30", cfg.SessionAutoArchiveDays, cfg.SessionAutoDeleteDays)
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "runtime:\n  session:\n    auto_archive_days: 14\n    auto_delete_days: 0\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionAutoArchiveDays != 14 {
		t.Fatalf("archive days = %d, want 14", cfg.SessionAutoArchiveDays)
	}
	if cfg.SessionAutoDeleteDays != 0 {
		t.Fatalf("delete days = %d: 0 must turn deletion off, not fall back to the default", cfg.SessionAutoDeleteDays)
	}
}

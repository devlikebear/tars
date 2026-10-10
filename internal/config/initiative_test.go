package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitiativeConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		in := cfg.Initiative
		if in.Enabled || in.Mode != "shadow" || in.Backend != "llm" || in.Tick != "1m" || in.QuietHours != "23:00-07:00" ||
			in.DailyCap != 6 || in.Cooldown != "45m" || in.DailyTextCalls != 60 || in.DailySpeakCalls != 12 ||
			in.QuietRequestedThreshold != 0.55 ||
			in.UserStrainedThreshold != 0.60 || in.SpecialDayThreshold != 0.55 {
			t.Fatalf("initiative defaults = %+v", in)
		}
		if cfg.Jev.BaseURL != "" || cfg.Jev.TimeoutSeconds != 10 {
			t.Fatalf("jev defaults = %+v", cfg.Jev)
		}
		if cfg.LLMRoleDefaults["initiative"] != "light" {
			t.Fatalf("initiative role default = %q, want light", cfg.LLMRoleDefaults["initiative"])
		}
	})

	t.Run("backend validation", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(`
initiative:
  backend: nonsense
`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("Load: want error for invalid initiative.backend")
		}
		if _, err := LoadFile(path); err == nil {
			t.Fatal("LoadFile: want error for invalid initiative.backend")
		}
	})

	t.Run("mode validation", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(`
initiative:
  mode: nonsense
`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("Load: want error for invalid initiative.mode")
		}
		if _, err := LoadFile(path); err == nil {
			t.Fatal("LoadFile: want error for invalid initiative.mode")
		}
	})

	t.Run("mode live", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(`
initiative:
  mode: live
`), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Initiative.Mode != "live" {
			t.Fatalf("initiative.mode = %q, want live", cfg.Initiative.Mode)
		}
	})

	t.Run("mode live via env", func(t *testing.T) {
		t.Setenv("TARS_INITIATIVE_MODE", "live")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Initiative.Mode != "live" {
			t.Fatalf("initiative.mode = %q, want live", cfg.Initiative.Mode)
		}
	})

	t.Run("backend and daily text calls override", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(`
initiative:
  backend: jev
  daily_text_calls: 10
`), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Initiative.Backend != "jev" || cfg.Initiative.DailyTextCalls != 10 {
			t.Fatalf("initiative = %+v", cfg.Initiative)
		}
	})

	t.Run("daily text calls via env", func(t *testing.T) {
		t.Setenv("TARS_INITIATIVE_DAILY_TEXT_CALLS", "25")
		t.Setenv("TARS_INITIATIVE_BACKEND", "jev")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Initiative.DailyTextCalls != 25 || cfg.Initiative.Backend != "jev" {
			t.Fatalf("initiative = %+v", cfg.Initiative)
		}
	})

	t.Run("daily speak calls override", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(`
initiative:
  daily_speak_calls: 4
`), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Initiative.DailySpeakCalls != 4 {
			t.Fatalf("initiative.daily_speak_calls = %d, want 4", cfg.Initiative.DailySpeakCalls)
		}
	})

	t.Run("daily speak calls via env", func(t *testing.T) {
		t.Setenv("TARS_INITIATIVE_DAILY_SPEAK_CALLS", "3")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Initiative.DailySpeakCalls != 3 {
			t.Fatalf("initiative.daily_speak_calls = %d, want 3", cfg.Initiative.DailySpeakCalls)
		}
	})

	t.Run("yaml and env", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(`
initiative:
  enabled: true
  quiet_hours: "22:30-08:00"
  daily_cap: 3
  body_provider: stackchan
  thresholds:
    user_strained: 0.7
jev:
  base_url: http://127.0.0.1:8009
  model: kev-latest
`), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TYPESAFE_API_KEY", "ts-key")
		t.Setenv("TARS_INITIATIVE_COOLDOWN", "90m")
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		in := cfg.Initiative
		if !in.Enabled || in.QuietHours != "22:30-08:00" || in.DailyCap != 3 || in.Cooldown != "90m" ||
			in.BodyProvider != "stackchan" || in.UserStrainedThreshold != 0.7 || in.QuietRequestedThreshold != 0.55 {
			t.Fatalf("initiative = %+v", in)
		}
		if cfg.Jev.BaseURL != "http://127.0.0.1:8009" || cfg.Jev.Model != "kev-latest" || cfg.Jev.APIKey != "ts-key" {
			t.Fatalf("jev = %+v", cfg.Jev)
		}
	})

	t.Run("api key is a sensitive schema field", func(t *testing.T) {
		for _, field := range Schema() {
			if field.Key == "jev_api_key" {
				if !field.Sensitive {
					t.Fatal("jev_api_key must be sensitive")
				}
				return
			}
		}
		t.Fatal("jev_api_key missing from schema")
	})
}

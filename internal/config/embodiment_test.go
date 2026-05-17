package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbodimentConfig(t *testing.T) {
	t.Run("defaults disabled when unset", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load defaults: %v", err)
		}
		if cfg.Embodiment.Enabled {
			t.Fatalf("embodiment should be disabled by default: %+v", cfg.Embodiment)
		}
		if len(cfg.Embodiment.Providers) != 0 {
			t.Fatalf("default embodiment providers = %+v", cfg.Embodiment.Providers)
		}
	})

	t.Run("loads yaml and env overrides", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(`
embodiment:
  enabled: true
  providers:
    - name: stackchan
      enabled: true
      transport: mcp
      endpoint: stackchan
      capabilities: [speech, expression]
      session_id: sess_main
      agent: embodied
      owner_only_directive: true
      salience_min_sound_level: 0.6
      min_trigger_interval: 30s
      max_triggers_per_hour: 60
`), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load yaml: %v", err)
		}
		if !cfg.Embodiment.Enabled {
			t.Fatal("expected embodiment enabled from yaml")
		}
		if len(cfg.Embodiment.Providers) != 1 {
			t.Fatalf("providers = %+v", cfg.Embodiment.Providers)
		}
		if cfg.Embodiment.Providers[0].Name != "stackchan" ||
			cfg.Embodiment.Providers[0].SessionID != "sess_main" ||
			cfg.Embodiment.Providers[0].Agent != "embodied" ||
			cfg.Embodiment.Providers[0].MinTriggerInterval != "30s" ||
			cfg.Embodiment.Providers[0].MaxTriggersPerHour != 60 ||
			!cfg.Embodiment.Providers[0].OwnerOnlyDirective {
			t.Fatalf("provider not loaded from yaml: %+v", cfg.Embodiment.Providers[0])
		}

		t.Setenv("TARS_EMBODIMENT_PROVIDERS_JSON", `[{"name":"host","enabled":true,"transport":"webhook","endpoint":"http://127.0.0.1:43180/v1/embodiment/percept/host","capabilities":["hearing","speech"],"session_id":"sess_host","min_trigger_interval":"15s","max_triggers_per_hour":10}]`)
		cfg, err = Load(path)
		if err != nil {
			t.Fatalf("load env override: %v", err)
		}
		if len(cfg.Embodiment.Providers) != 1 ||
			cfg.Embodiment.Providers[0].Name != "host" ||
			cfg.Embodiment.Providers[0].SessionID != "sess_host" ||
			cfg.Embodiment.Providers[0].MinTriggerInterval != "15s" ||
			cfg.Embodiment.Providers[0].MaxTriggersPerHour != 10 {
			t.Fatalf("providers did not come from env override: %+v", cfg.Embodiment.Providers)
		}
	})
}

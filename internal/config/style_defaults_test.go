package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_SessionStyleDefaultFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
runtime:
  style:
    directness_default: 84
    humor_default: 16
    caution_default: 68
    autonomy_default: 42
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.StyleDirectnessDefault != 84 || cfg.StyleHumorDefault != 16 ||
		cfg.StyleCautionDefault != 68 || cfg.StyleAutonomyDefault != 42 {
		t.Fatalf("unexpected style defaults: directness=%d humor=%d caution=%d autonomy=%d",
			cfg.StyleDirectnessDefault,
			cfg.StyleHumorDefault,
			cfg.StyleCautionDefault,
			cfg.StyleAutonomyDefault,
		)
	}
}

func TestLoad_SessionStyleDefaultsAllowZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
runtime:
  style:
    directness_default: 0
    humor_default: 0
    caution_default: 0
    autonomy_default: 0
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.StyleDirectnessDefault != 0 || cfg.StyleHumorDefault != 0 ||
		cfg.StyleCautionDefault != 0 || cfg.StyleAutonomyDefault != 0 {
		t.Fatalf("unexpected style defaults: directness=%d humor=%d caution=%d autonomy=%d",
			cfg.StyleDirectnessDefault,
			cfg.StyleHumorDefault,
			cfg.StyleCautionDefault,
			cfg.StyleAutonomyDefault,
		)
	}
}

func TestApplyDefaults_ClampsSessionStyleDefaultFields(t *testing.T) {
	cfg := Default()
	cfg.StyleDirectnessDefault = 140
	cfg.StyleHumorDefault = -12
	cfg.StyleCautionDefault = 0
	cfg.StyleAutonomyDefault = 101

	applyDefaults(&cfg)

	if cfg.StyleDirectnessDefault != 100 {
		t.Fatalf("directness = %d, want 100", cfg.StyleDirectnessDefault)
	}
	if cfg.StyleHumorDefault != defaultStyleHumor {
		t.Fatalf("humor = %d, want default %d", cfg.StyleHumorDefault, defaultStyleHumor)
	}
	if cfg.StyleCautionDefault != 0 {
		t.Fatalf("caution = %d, want 0", cfg.StyleCautionDefault)
	}
	if cfg.StyleAutonomyDefault != 100 {
		t.Fatalf("autonomy = %d, want 100", cfg.StyleAutonomyDefault)
	}
}

func TestConfigToMap_IncludesSessionStyleDefaultFields(t *testing.T) {
	cfg := Default()
	cfg.StyleDirectnessDefault = 0
	cfg.StyleHumorDefault = 12
	cfg.StyleCautionDefault = 100
	cfg.StyleAutonomyDefault = 1

	values := ConfigToMap(cfg)

	if values["style_directness_default"] != 0 {
		t.Fatalf("style_directness_default = %#v, want 0", values["style_directness_default"])
	}
	if values["style_humor_default"] != 12 {
		t.Fatalf("style_humor_default = %#v, want 12", values["style_humor_default"])
	}
	if values["style_caution_default"] != 100 {
		t.Fatalf("style_caution_default = %#v, want 100", values["style_caution_default"])
	}
	if values["style_autonomy_default"] != 1 {
		t.Fatalf("style_autonomy_default = %#v, want 1", values["style_autonomy_default"])
	}
}

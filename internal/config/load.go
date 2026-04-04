package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load resolves runtime settings with the following precedence:
// defaults < YAML file < environment variables.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		fileCfg, err := loadYAML(path)
		if err != nil {
			return Config{}, err
		}
		merge(&cfg, fileCfg)
	}

	applyEnv(&cfg)
	applyDefaults(&cfg)
	return cfg, nil
}

// LoadRaw reads the raw content of the config file at the given path.
func LoadRaw(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is empty")
	}
	return os.ReadFile(path)
}

// SaveRaw writes raw content to the config file at the given path.
// It validates that the content is valid YAML before writing.
func SaveRaw(path string, content []byte) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	return os.WriteFile(path, content, 0644)
}

// PatchYAML reads the YAML file at path, merges the given key-value updates,
// and writes the result back. Unknown keys are ignored.
func PatchYAML(path string, updates map[string]any) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}

	// Read existing content or start fresh
	existing := map[string]any{}
	raw, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(raw, &existing)
	}

	// Merge updates (only known keys)
	for key, value := range updates {
		if _, ok := configInputFieldByYAMLKey(key); ok {
			existing[key] = value
		}
	}

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, out, 0644)
}

func ResolveConfigPath(raw string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return os.ExpandEnv(v)
	}
	if v := strings.TrimSpace(firstNonEmpty(os.Getenv("TARS_CONFIG"), os.Getenv("TARS_CONFIG_PATH"))); v != "" {
		return os.ExpandEnv(v)
	}
	if _, err := os.Stat(DefaultConfigFilename); err == nil {
		return DefaultConfigFilename
	}
	if fixed := FixedConfigPath(); fixed != "" {
		if _, err := os.Stat(fixed); err == nil {
			return fixed
		}
	}
	return ""
}

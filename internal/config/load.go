package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	if cfg.ToolsComputerUseBackend != "llm" && cfg.ToolsComputerUseBackend != "jev" {
		return Config{}, fmt.Errorf("tools.computer_use.backend must be llm or jev")
	}
	if cfg.Initiative.Backend != "llm" && cfg.Initiative.Backend != "jev" {
		return Config{}, fmt.Errorf("initiative.backend must be llm or jev")
	}
	if cfg.Initiative.Mode != "shadow" && cfg.Initiative.Mode != "live" {
		return Config{}, fmt.Errorf("initiative.mode must be shadow or live")
	}
	return cfg, nil
}

// LoadFile resolves settings from defaults + YAML only, intentionally skipping
// environment overrides. Use this for editing the config file itself; Load is
// still the runtime path where env vars have highest precedence.
func LoadFile(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		fileCfg, err := loadYAML(path)
		if err != nil {
			return Config{}, err
		}
		merge(&cfg, fileCfg)
	}

	applyDefaults(&cfg)
	if cfg.ToolsComputerUseBackend != "llm" && cfg.ToolsComputerUseBackend != "jev" {
		return Config{}, fmt.Errorf("tools.computer_use.backend must be llm or jev")
	}
	if cfg.Initiative.Backend != "llm" && cfg.Initiative.Backend != "jev" {
		return Config{}, fmt.Errorf("initiative.backend must be llm or jev")
	}
	if cfg.Initiative.Mode != "shadow" && cfg.Initiative.Mode != "live" {
		return Config{}, fmt.Errorf("initiative.mode must be shadow or live")
	}
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

var patchMu sync.Mutex

// PatchYAML reads the YAML file at path, merges the given key-value updates,
// and atomically writes the result back. Unknown keys are rejected.
func PatchYAML(path string, updates map[string]any) error {
	patchMu.Lock()
	defer patchMu.Unlock()
	if path == "" {
		return fmt.Errorf("config path is empty")
	}

	// Read existing content or start fresh
	existing := map[string]any{}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := yaml.Unmarshal(raw, &existing); err != nil {
			return err
		}
	}
	if existing == nil {
		existing = map[string]any{}
	}

	// Merge updates (only known keys)
	for key, value := range updates {
		resolvedKey := normalizeConfigUpdateKey(key, value)
		if resolvedKey == "" {
			return &PatchValidationError{Message: "unknown config key: " + key}
		}
		if err := validatePatchValue(resolvedKey, value); err != nil {
			return err
		}
		var patchedValue any
		if resolvedKey == "llm_providers" {
			providers, err := mergeRawProviderEdit(existing, value)
			if err != nil {
				return err
			}
			patchedValue = providers
		} else {
			patchedValue = normalizePatchedConfigValue(resolvedKey, value)
			if newMap := anyToStringMap(patchedValue); newMap != nil {
				if existingMap := readConfigYAMLMap(existing, resolvedKey); existingMap != nil {
					nestedMapMerge(existingMap, newMap)
					patchedValue = existingMap
				}
			}
		}
		deleteConfigYAMLRepresentations(existing, resolvedKey)
		setConfigYAMLValue(existing, resolvedKey, patchedValue)
	}

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// Ensure the parent directory exists so the wizard's first PATCH
	// can land at a brand-new location (e.g. ~/.tars/config/) without
	// the operator running tars init first.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir %s: %w", dir, err)
		}
	}
	if err := validateCandidate(existing); err != nil {
		return err
	}
	return atomicConfigWrite(path, out)
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

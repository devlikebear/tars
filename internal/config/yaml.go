package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func loadYAML(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config file %q: %w", path, err)
	}

	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}

	var cfg Config
	for key, rawValue := range parsed {
		value := yamlValueString(rawValue)
		normalizedKey := strings.TrimSpace(strings.ToLower(key))
		if field, ok := configInputFieldByYAMLKey(normalizedKey); ok {
			field.apply(&cfg, value)
		}
	}
	return cfg, nil
}

func yamlValueString(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return os.ExpandEnv(strings.TrimSpace(value))
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(value))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
		return strings.TrimSpace(string(encoded))
	}
}

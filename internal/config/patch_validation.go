package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// PatchValidationError describes input which must not be written to disk.
type PatchValidationError struct{ Message string }

func (e *PatchValidationError) Error() string { return e.Message }

func validatePatchValue(key string, value any) error {
	for _, f := range Schema() {
		if f.Key != key {
			continue
		}
		valid := false
		switch f.Type {
		case "string":
			_, valid = value.(string)
		case "select":
			s, ok := value.(string)
			valid = ok && slices.Contains(f.Options, s)
		case "bool":
			_, valid = value.(bool)
		case "int", "float":
			switch n := value.(type) {
			case int:
				valid = true
			case float64:
				valid = !math.IsNaN(n) && !math.IsInf(n, 0) && (f.Type != "int" || (math.Trunc(n) == n && n < float64(math.MaxInt) && n > float64(math.MinInt)))
			}
			if valid {
				// Use the field's own parser with a nonzero fallback so invalid
				// zero/negative/range inputs cannot silently become defaults.
				if field, ok := configInputFieldByYAMLKey(key); ok {
					var parsed Config
					field.apply(&parsed, "1")
					field.apply(&parsed, yamlValueString(value))
					valid = yamlValueString(extractValue(key, parsed)) == yamlValueString(value)
				}
			}
		case "string_list":
			raw, err := json.Marshal(value)
			if s, ok := value.(string); ok {
				raw = []byte(s)
			}
			var list []string
			valid = err == nil && json.Unmarshal(raw, &list) == nil && value != nil
		case "json":
			raw, err := json.Marshal(value)
			if s, ok := value.(string); ok {
				raw = []byte(s)
			}
			if err == nil && json.Valid(raw) {
				var cfg Config
				field, ok := configInputFieldByYAMLKey(key)
				if ok {
					target := extractValue(key, Default())
					switch key {
					case "llm_providers":
						target = map[string]LLMProviderSettings{}
					case "llm_tiers":
						target = map[string]LLMTierBinding{}
					case "llm_role_defaults":
						target = map[string]string{}
					}
					if target != nil {
						decoder := json.NewDecoder(bytes.NewReader(raw))
						decoder.DisallowUnknownFields()
						valid = decoder.Decode(reflect.New(reflect.TypeOf(target)).Interface()) == nil && string(raw) != "null"
					} else {
						field.apply(&cfg, string(raw))
						valid = extractValue(key, cfg) != nil
					}
					if valid && key == "llm_role_defaults" {
						var roles map[string]string
						_ = json.Unmarshal(raw, &roles)
						for role, tier := range roles {
							if _, ok := llm.ParseRole(role); !ok {
								return &PatchValidationError{Message: fmt.Sprintf("invalid llm role %q in llm_role_defaults", role)}
							}
							if _, err := llm.ParseTier(tier); err != nil {
								return &PatchValidationError{Message: fmt.Sprintf("invalid tier %q for llm role %q: %v", tier, role, err)}
							}
						}
					}
				}
			}
		}
		if !valid {
			return &PatchValidationError{Message: fmt.Sprintf("invalid %s value for %s", f.Type, key)}
		}
		return nil
	}
	return &PatchValidationError{Message: "unknown config key: " + key}
}

func validateCandidate(candidate map[string]any) error {
	flattened := flattenConfigYAML(candidate)
	var cfg Config
	for key, value := range flattened {
		if field, ok := configInputFieldByYAMLKey(key); ok {
			field.apply(&cfg, yamlValueString(value))
		}
	}
	for tier := range cfg.LLMTiers {
		if _, err := llm.ParseTier(tier); err != nil {
			return &PatchValidationError{Message: fmt.Sprintf("invalid llm tier %q: %v", tier, err)}
		}
		resolved, err := ResolveLLMTier(&cfg, tier)
		if err != nil {
			return &PatchValidationError{Message: err.Error()}
		}
		// Match the kinds accepted by llm.NewProvider without constructing
		// clients or requiring credentials during configuration validation.
		switch resolved.Kind {
		case "openai", "kimi", "gemini", "gemini-native", "anthropic", "openai-codex", "claude-code-cli", "antigravity-cli":
		default:
			return &PatchValidationError{Message: fmt.Sprintf("unsupported llm provider: %s", resolved.Kind)}
		}
		if err := llm.ValidateProviderAuthMode(resolved.Kind, resolved.AuthMode); err != nil {
			return &PatchValidationError{Message: err.Error()}
		}
	}
	if len(cfg.LLMTiers) > 0 {
		for _, tier := range llm.AllTiers() {
			if _, ok := cfg.LLMTiers[tier.String()]; !ok {
				return &PatchValidationError{Message: fmt.Sprintf("required llm tier %q is not present in llm_tiers", tier)}
			}
		}
	}
	// Match runtime defaulting without mutating the candidate YAML. Partial
	// settings files without tiers remain editable; once tiers are configured,
	// the selected default must resolve to one of them.
	defaultName := cfg.LLMDefaultTier
	if strings.TrimSpace(defaultName) == "" {
		defaultName = "standard"
	}
	defaultTier, err := llm.ParseTier(defaultName)
	if err != nil {
		return &PatchValidationError{Message: fmt.Sprintf("invalid llm_default_tier %q: %v", cfg.LLMDefaultTier, err)}
	}
	if len(cfg.LLMTiers) > 0 {
		if _, ok := cfg.LLMTiers[defaultTier.String()]; !ok {
			return &PatchValidationError{Message: fmt.Sprintf("llm_default_tier %q is not present in llm_tiers", defaultTier)}
		}
	}
	for roleName, tierName := range cfg.LLMRoleDefaults {
		if _, ok := llm.ParseRole(roleName); !ok {
			return &PatchValidationError{Message: fmt.Sprintf("invalid llm role %q in llm_role_defaults", roleName)}
		}
		tier, err := llm.ParseTier(tierName)
		if err != nil {
			return &PatchValidationError{Message: fmt.Sprintf("invalid tier %q for llm role %q: %v", tierName, roleName, err)}
		}
		if len(cfg.LLMTiers) > 0 {
			if _, ok := cfg.LLMTiers[tier.String()]; !ok {
				return &PatchValidationError{Message: fmt.Sprintf("tier %q for llm role %q is not present in llm_tiers", tier, roleName)}
			}
		}
	}
	for key, value := range flattened {
		// Expand scalar YAML references as the loader does, without
		// applying unrelated environment overrides or changing the file.
		for _, f := range Schema() {
			if f.Key == key {
				if _, ok := value.(string); ok {
					expanded := yamlValueString(value)
					switch f.Type {
					case "bool":
						if n, err := strconv.ParseBool(expanded); err == nil {
							value = n
						}
					case "int":
						if n, err := strconv.Atoi(expanded); err == nil {
							value = n
						}
					case "float":
						if n, err := strconv.ParseFloat(expanded, 64); err == nil {
							value = n
						}
					default:
						value = expanded
					}
				}
				if err := validatePatchValue(key, value); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func atomicConfigWrite(path string, data []byte) error {
	mode := os.FileMode(0600)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to replace symlink config")
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// ConfigMapsDiffer compares unmasked runtime and candidate values.
func ConfigMapsDiffer(a, b map[string]any) []string {
	var keys []string
	for key, value := range b {
		if !reflect.DeepEqual(a[key], value) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

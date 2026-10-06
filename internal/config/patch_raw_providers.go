package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

type providerObjectSource bool

const (
	submittedProviderObjects providerObjectSource = false
	existingProviderObjects  providerObjectSource = true
)

// rawProviderObjects shares one alias/field boundary for values and presence.
// Do not expand environment references: this is the persistence representation.
func rawProviderObjects(value any, source providerObjectSource) (map[string]any, error) {
	var raw []byte
	var err error
	if text, ok := value.(string); ok {
		raw = []byte(text)
	} else {
		raw, err = json.Marshal(value)
	}
	if err != nil {
		return nil, &PatchValidationError{Message: err.Error()}
	}
	var submitted map[string]map[string]any
	if err = json.Unmarshal(raw, &submitted); err != nil {
		return nil, &PatchValidationError{Message: err.Error()}
	}
	result := make(map[string]any, len(submitted))
	for original, fields := range submitted {
		alias := strings.TrimSpace(original)
		if alias == "" {
			return nil, &PatchValidationError{Message: "empty provider alias"}
		}
		if _, exists := result[alias]; exists {
			return nil, &PatchValidationError{Message: fmt.Sprintf("duplicate normalized provider alias %q", alias)}
		}
		if fields == nil {
			return nil, &PatchValidationError{Message: "null provider object"}
		}
		stringsByField := make(map[string]string, len(fields))
		for key, value := range fields {
			// Existing YAML null string fields have the loader zero-value meaning.
			// Submitted JSON remains strictly string-only.
			if value == nil && source == existingProviderObjects {
				switch key {
				case "kind", "auth_mode", "base_url", "api_key":
					value = ""
				}
			}
			text, ok := value.(string)
			if !ok {
				return nil, &PatchValidationError{Message: fmt.Sprintf("provider %q field %q must be a string", alias, key)}
			}
			stringsByField[key] = text
		}
		object := map[string]any{"kind": strings.TrimSpace(stringsByField["kind"]), "auth_mode": strings.TrimSpace(stringsByField["auth_mode"]), "base_url": strings.TrimSpace(stringsByField["base_url"])}
		for key := range fields {
			switch key {
			case "kind", "auth_mode", "base_url", "api_key":
			default:
				return nil, &PatchValidationError{Message: "unknown provider field: " + key}
			}
		}
		if _, present := fields["api_key"]; present {
			object["api_key"] = stringsByField["api_key"]
		}
		result[alias] = object
	}
	return result, nil
}

func mergeRawProviderEdit(existing map[string]any, submitted any) (map[string]any, error) {
	next, err := rawProviderObjects(submitted, submittedProviderObjects)
	if err != nil {
		return nil, err
	}
	previous := map[string]any{}
	if source, present := flattenConfigYAML(existing)["llm_providers"]; present {
		previous, err = rawProviderObjects(source, existingProviderObjects)
		if err != nil {
			return nil, err
		}
	}
	for alias, value := range next {
		object := value.(map[string]any)
		secret, present := object["api_key"].(string)
		if !present || strings.Contains(secret, "*") {
			delete(object, "api_key")
			if old, exists := previous[alias]; exists {
				if key, exists := old.(map[string]any)["api_key"]; exists {
					object["api_key"] = key
				}
			}
		}
	}
	return next, nil
}

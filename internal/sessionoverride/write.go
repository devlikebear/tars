package sessionoverride

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/session"
)

// WriteLocalToolConfig updates only the tool_config object in
// <cwd>/.tars/settings.local.json, preserving other local override fields.
func WriteLocalToolConfig(cwd string, config session.SessionToolConfig) error {
	if cwd == "" {
		return fmt.Errorf("cwd is required")
	}
	path := filepath.Join(cwd, settingsDirName, localSettingsName)
	payload := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil {
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &payload); err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if isEmptyToolConfig(config) {
		delete(payload, "tool_config")
	} else {
		rawConfig, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("encode tool_config: %w", err)
		}
		payload["tool_config"] = rawConfig
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	raw = append(raw, '\n')
	return atomicwrite.Write(path, raw)
}

func isEmptyToolConfig(config session.SessionToolConfig) bool {
	return !config.ToolsCustom &&
		!config.SkillsCustom &&
		!config.CommandsCustom &&
		len(config.ToolsEnabled) == 0 &&
		len(config.ToolsDisabled) == 0 &&
		len(config.ToolsAllowGroups) == 0 &&
		len(config.ToolsDenyGroups) == 0 &&
		len(config.SkillsEnabled) == 0 &&
		len(config.CommandsEnabled) == 0 &&
		len(config.MCPEnabled) == 0
}

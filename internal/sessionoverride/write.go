package sessionoverride

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/session"
)

type LocalScaffoldResult struct {
	CWD               string
	SettingsPath      string
	LocalSettingsPath string
	SkillsDir         string
	CommandsDir       string
	GitignorePath     string
	Created           []string
	Existing          []string
}

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

func ScaffoldLocal(cwd string, force bool) (LocalScaffoldResult, error) {
	if strings.TrimSpace(cwd) == "" {
		return LocalScaffoldResult{}, fmt.Errorf("cwd is required")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return LocalScaffoldResult{}, fmt.Errorf("resolve cwd: %w", err)
	}
	tarsDir := filepath.Join(abs, settingsDirName)
	result := LocalScaffoldResult{
		CWD:               abs,
		SettingsPath:      filepath.Join(tarsDir, sharedSettingsName),
		LocalSettingsPath: filepath.Join(tarsDir, localSettingsName),
		SkillsDir:         filepath.Join(tarsDir, "skills"),
		CommandsDir:       filepath.Join(tarsDir, "commands"),
		GitignorePath:     filepath.Join(tarsDir, ".gitignore"),
	}

	for _, dir := range []string{result.SkillsDir, result.CommandsDir} {
		created, err := ensureScaffoldDir(dir)
		if err != nil {
			return LocalScaffoldResult{}, err
		}
		if created {
			result.Created = append(result.Created, dir)
		} else {
			result.Existing = append(result.Existing, dir)
		}
	}

	if err := writeScaffoldFile(result.SettingsPath, []byte("{\n  \"tool_config\": {}\n}\n"), force, &result); err != nil {
		return LocalScaffoldResult{}, err
	}
	if err := writeScaffoldFile(result.LocalSettingsPath, []byte("{}\n"), force, &result); err != nil {
		return LocalScaffoldResult{}, err
	}
	if err := ensureScaffoldGitignore(result.GitignorePath, force, &result); err != nil {
		return LocalScaffoldResult{}, err
	}
	return result, nil
}

func ensureScaffoldDir(path string) (bool, error) {
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("%s exists and is not a directory", path)
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", path, err)
	}
	return true, nil
}

func writeScaffoldFile(path string, data []byte, force bool, result *LocalScaffoldResult) error {
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s exists and is a directory", path)
		}
		if !force {
			result.Existing = append(result.Existing, path)
			return nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := atomicwrite.Write(path, data); err != nil {
		return err
	}
	result.Created = append(result.Created, path)
	return nil
}

func ensureScaffoldGitignore(path string, force bool, result *LocalScaffoldResult) error {
	entry := localSettingsName + "\n"
	if force {
		return writeScaffoldFile(path, []byte(entry), true, result)
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains("\n"+string(raw), "\n"+localSettingsName+"\n") {
			result.Existing = append(result.Existing, path)
			return nil
		}
		next := append([]byte{}, raw...)
		if len(next) > 0 && next[len(next)-1] != '\n' {
			next = append(next, '\n')
		}
		next = append(next, []byte(entry)...)
		if err := atomicwrite.Write(path, next); err != nil {
			return err
		}
		result.Created = append(result.Created, path)
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return writeScaffoldFile(path, []byte(entry), false, result)
}

func isEmptyToolConfig(config session.SessionToolConfig) bool {
	return !config.ToolsCustom &&
		!config.SkillsCustom &&
		!config.CommandsCustom &&
		!config.MCPCustom &&
		len(config.ToolsEnabled) == 0 &&
		len(config.ToolsDisabled) == 0 &&
		len(config.ToolsAllowGroups) == 0 &&
		len(config.ToolsDenyGroups) == 0 &&
		len(config.SkillsEnabled) == 0 &&
		len(config.CommandsEnabled) == 0 &&
		len(config.MCPEnabled) == 0
}

package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const disabledFileName = "extensions_disabled.json"

// DisabledSet tracks which extensions are disabled by the user.
type DisabledSet struct {
	Skills     []string `json:"skills,omitempty"`
	Plugins    []string `json:"plugins,omitempty"`
	MCPServers []string `json:"mcp_servers,omitempty"`
}

type disabledStore struct {
	mu   sync.Mutex
	path string
}

func newDisabledStore(workspaceDir string) *disabledStore {
	return &disabledStore{
		path: filepath.Join(strings.TrimSpace(workspaceDir), disabledFileName),
	}
}

func (s *disabledStore) Load() (DisabledSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return DisabledSet{}, nil
		}
		return DisabledSet{}, err
	}
	var ds DisabledSet
	if err := json.Unmarshal(data, &ds); err != nil {
		return DisabledSet{}, err
	}
	return ds, nil
}

func (s *disabledStore) Save(ds DisabledSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(data, '\n'), 0o644)
}

// SetDisabled enables or disables a specific extension.
func (s *disabledStore) SetDisabled(kind, name string, disabled bool) error {
	ds, err := s.Load()
	if err != nil {
		ds = DisabledSet{}
	}
	key := strings.TrimSpace(strings.ToLower(name))

	var list *[]string
	switch strings.TrimSpace(kind) {
	case "skill":
		list = &ds.Skills
	case "plugin":
		list = &ds.Plugins
	case "mcp":
		list = &ds.MCPServers
	default:
		return nil
	}

	if disabled {
		// Add if not present
		for _, v := range *list {
			if strings.ToLower(v) == key {
				return nil // already disabled
			}
		}
		*list = append(*list, name)
	} else {
		// Remove
		filtered := make([]string, 0, len(*list))
		for _, v := range *list {
			if strings.ToLower(v) != key {
				filtered = append(filtered, v)
			}
		}
		*list = filtered
	}

	return s.Save(ds)
}

func (ds DisabledSet) isSkillDisabled(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, v := range ds.Skills {
		if strings.ToLower(v) == key {
			return true
		}
	}
	return false
}

func (ds DisabledSet) isPluginDisabled(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, v := range ds.Plugins {
		if strings.ToLower(v) == key {
			return true
		}
	}
	return false
}

func (ds DisabledSet) isMCPDisabled(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, v := range ds.MCPServers {
		if strings.ToLower(v) == key {
			return true
		}
	}
	return false
}

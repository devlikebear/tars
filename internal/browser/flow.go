package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SiteFlow defines a site-specific browser automation flow.
type SiteFlow struct {
	ID           string                `yaml:"id" json:"id"`
	Enabled      bool                  `yaml:"enabled" json:"enabled"`
	Profile      string                `yaml:"profile,omitempty" json:"profile,omitempty"`
	AllowedHosts []string              `yaml:"allowed_hosts,omitempty" json:"allowed_hosts,omitempty"`
	Login        SiteLogin             `yaml:"login,omitempty" json:"login,omitempty"`
	Checks       []SiteCheck           `yaml:"checks,omitempty" json:"checks,omitempty"`
	Actions      map[string]SiteAction `yaml:"actions,omitempty" json:"actions,omitempty"`
}

type SiteLogin struct {
	Mode             string `yaml:"mode,omitempty" json:"mode,omitempty"`
	VaultPath        string `yaml:"vault_path,omitempty" json:"vault_path,omitempty"`
	EnvPrefix        string `yaml:"env_prefix,omitempty" json:"env_prefix,omitempty"`
	UsernameField    string `yaml:"username_field,omitempty" json:"username_field,omitempty"`
	PasswordField    string `yaml:"password_field,omitempty" json:"password_field,omitempty"`
	UsernameSelector string `yaml:"username_selector,omitempty" json:"username_selector,omitempty"`
	PasswordSelector string `yaml:"password_selector,omitempty" json:"password_selector,omitempty"`
	SubmitSelector   string `yaml:"submit_selector,omitempty" json:"submit_selector,omitempty"`
	SuccessSelector  string `yaml:"success_selector,omitempty" json:"success_selector,omitempty"`
	OTPRequired      bool   `yaml:"otp_required,omitempty" json:"otp_required,omitempty"`
	OTPTimeoutSec    int    `yaml:"otp_timeout_sec,omitempty" json:"otp_timeout_sec,omitempty"`
}

type SiteCheck struct {
	Selector  string `yaml:"selector,omitempty" json:"selector,omitempty"`
	Contains  string `yaml:"contains,omitempty" json:"contains,omitempty"`
	TimeoutMS int    `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty"`
}

type SiteAction struct {
	Steps []SiteStep `yaml:"steps,omitempty" json:"steps,omitempty"`
}

type SiteStep struct {
	Open    string `yaml:"open,omitempty" json:"open,omitempty"`
	Click   string `yaml:"click,omitempty" json:"click,omitempty"`
	Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	Wait    int    `yaml:"wait,omitempty" json:"wait,omitempty"`
	Extract string `yaml:"extract,omitempty" json:"extract,omitempty"`
}

// LoadSiteFlows parses all site flow YAML files from dir.
func LoadSiteFlows(dir string) ([]SiteFlow, error) {
	trimmedDir := strings.TrimSpace(dir)
	if trimmedDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(trimmedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read site flows dir failed: %w", err)
	}
	flows := make([]SiteFlow, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(entry.Name()))
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		fullPath := filepath.Join(trimmedDir, entry.Name())
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read site flow %s failed: %w", entry.Name(), err)
		}
		var flow SiteFlow
		if err := yaml.Unmarshal(raw, &flow); err != nil {
			return nil, fmt.Errorf("parse site flow %s failed: %w", entry.Name(), err)
		}
		flow.ID = strings.TrimSpace(flow.ID)
		if flow.ID == "" {
			return nil, fmt.Errorf("site flow %s missing id", entry.Name())
		}
		if !flow.Enabled && !containsExplicitEnabled(raw) {
			flow.Enabled = true
		}
		flows = append(flows, flow)
	}
	sort.Slice(flows, func(i, j int) bool {
		return strings.ToLower(flows[i].ID) < strings.ToLower(flows[j].ID)
	})
	return flows, nil
}

func containsExplicitEnabled(raw []byte) bool {
	text := strings.ToLower(string(raw))
	return strings.Contains(text, "enabled:")
}

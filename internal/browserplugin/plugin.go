package browserplugin

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/browser"
	"github.com/devlikebear/tars/internal/plugin"
)

const pluginID = "tars-browser"

// Plugin implements plugin.BuiltinPlugin for browser automation.
type Plugin struct {
	service     *browser.Service
	vaultStatus any // vaultStatusSnapshot passed via config
}

func (p *Plugin) ID() string { return pluginID }

func (p *Plugin) Definition() plugin.Definition {
	return plugin.Definition{
		SchemaVersion: 3,
		ID:            pluginID,
		Name:          "Browser Automation",
		Description:   "Provides browser control and site automation tools",
		Version:       "1.0.0",
		Source:        plugin.SourceBundled,
		ToolsProvider: &plugin.ToolsProvider{
			Type:  "go_plugin",
			Entry: "builtin:" + pluginID,
		},
		HTTPRoutes: []plugin.HTTPRoute{
			{Path: "/v1/browser/status"},
			{Path: "/v1/browser/profiles"},
			{Path: "/v1/browser/login"},
			{Path: "/v1/browser/check"},
			{Path: "/v1/browser/run"},
			{Path: "/v1/vault/status"},
		},
	}
}

func (p *Plugin) Init(ctx plugin.PluginContext) error {
	cfg := ctx.Config
	enabled, _ := cfg["browser_runtime_enabled"].(bool)
	if !enabled {
		return nil
	}

	p.vaultStatus = cfg["vault_status"]

	var vault browser.SecretReader
	if v, ok := cfg["vault_reader"]; ok && v != nil {
		vault, _ = v.(browser.SecretReader)
	}
	var otp browser.OTPRequester
	if v, ok := cfg["otp_requester"]; ok && v != nil {
		otp, _ = v.(browser.OTPRequester)
	}

	p.service = browser.NewService(browser.Config{
		WorkspaceDir:           strings.TrimSpace(ctx.WorkspaceDir),
		DefaultProfile:         configString(cfg, "browser_default_profile"),
		ManagedHeadless:        configBool(cfg, "browser_managed_headless"),
		ManagedExecutablePath:  configString(cfg, "browser_managed_executable_path"),
		ManagedUserDataDir:     configString(cfg, "browser_managed_user_data_dir"),
		SiteFlowsDir:           configString(cfg, "browser_site_flows_dir"),
		AutoLoginSiteAllowlist: configStringSlice(cfg, "browser_auto_login_site_allowlist"),
		Vault:                  vault,
		OTP:                    otp,
	})
	return nil
}

func (p *Plugin) Close() error {
	p.service = nil
	return nil
}

func configString(cfg map[string]any, key string) string {
	if v, ok := cfg[key]; ok {
		s, _ := v.(string)
		return strings.TrimSpace(s)
	}
	return ""
}

func configBool(cfg map[string]any, key string) bool {
	if v, ok := cfg[key]; ok {
		b, _ := v.(bool)
		return b
	}
	return false
}

func configStringSlice(cfg map[string]any, key string) []string {
	if v, ok := cfg[key]; ok {
		sl, _ := v.([]string)
		return sl
	}
	return nil
}

func (p *Plugin) requireService() error {
	if p.service == nil {
		return fmt.Errorf("browser service is not initialized")
	}
	return nil
}

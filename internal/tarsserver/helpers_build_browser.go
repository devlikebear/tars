package tarsserver

import (
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/browser"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/vaultclient"
)

func buildBrowserPluginConfig(cfg config.Config, vaultReader vaultclient.SecretReader, vaultStatus vaultStatusSnapshot, otpRequester browser.OTPRequester) map[string]any {
	return map[string]any{
		"browser_runtime_enabled":           cfg.BrowserRuntimeEnabled,
		"browser_default_profile":           cfg.BrowserDefaultProfile,
		"browser_managed_headless":          cfg.BrowserManagedHeadless,
		"browser_managed_executable_path":   cfg.BrowserManagedExecutablePath,
		"browser_managed_user_data_dir":     cfg.BrowserManagedUserDataDir,
		"browser_site_flows_dir":            cfg.BrowserSiteFlowsDir,
		"browser_auto_login_site_allowlist": cfg.BrowserAutoLoginSiteAllowlist,
		"vault_reader":                      vaultReader,
		"vault_status":                      vaultStatus,
		"otp_requester":                     otpRequester,
	}
}

type vaultStatusSnapshot struct {
	Enabled        bool   `json:"enabled"`
	Ready          bool   `json:"ready"`
	AuthMode       string `json:"auth_mode,omitempty"`
	Addr           string `json:"addr,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	AllowlistCount int    `json:"allowlist_count"`
	LastError      string `json:"last_error,omitempty"`
}

func buildVaultReader(cfg config.Config) (vaultclient.SecretReader, vaultStatusSnapshot, error) {
	status := vaultStatusSnapshot{
		Enabled:        cfg.VaultEnabled,
		AuthMode:       strings.TrimSpace(cfg.VaultAuthMode),
		Addr:           strings.TrimSpace(cfg.VaultAddr),
		Namespace:      strings.TrimSpace(cfg.VaultNamespace),
		AllowlistCount: len(cfg.VaultSecretPathAllowlist),
	}
	if !cfg.VaultEnabled {
		return nil, status, nil
	}
	client, err := vaultclient.New(vaultclient.ClientOptions{
		Enabled:             cfg.VaultEnabled,
		Addr:                cfg.VaultAddr,
		AuthMode:            cfg.VaultAuthMode,
		Token:               cfg.VaultToken,
		Namespace:           cfg.VaultNamespace,
		Timeout:             time.Duration(cfg.VaultTimeoutMS) * time.Millisecond,
		KVMount:             cfg.VaultKVMount,
		KVVersion:           cfg.VaultKVVersion,
		AppRoleMount:        cfg.VaultAppRoleMount,
		AppRoleRoleID:       cfg.VaultAppRoleRoleID,
		AppRoleSecretID:     cfg.VaultAppRoleSecretID,
		SecretPathAllowlist: cfg.VaultSecretPathAllowlist,
	})
	if err != nil {
		status.LastError = err.Error()
		return nil, status, err
	}
	status.Ready = true
	return client, status, nil
}

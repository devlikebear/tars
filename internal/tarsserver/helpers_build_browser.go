package tarsserver

import (
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/browser"
	"github.com/devlikebear/tarsncase/internal/browserrelay"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/vaultclient"
)

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

func buildBrowserRelay(cfg config.Config) (*browserrelay.Server, error) {
	if !cfg.BrowserRelayEnabled {
		return nil, nil
	}
	return browserrelay.New(browserrelay.Options{
		Addr:            cfg.BrowserRelayAddr,
		RelayToken:      cfg.BrowserRelayToken,
		AllowQueryToken: cfg.BrowserRelayAllowQueryToken,
		OriginAllowlist: cfg.BrowserRelayOriginAllowlist,
	})
}

func buildBrowserService(cfg config.Config, relay *browserrelay.Server, vaultReader vaultclient.SecretReader, otpRequester browser.OTPRequester) *browser.Service {
	if !cfg.BrowserRuntimeEnabled {
		return nil
	}
	return browser.NewService(browser.Config{
		WorkspaceDir:           cfg.WorkspaceDir,
		DefaultProfile:         cfg.BrowserDefaultProfile,
		ManagedHeadless:        cfg.BrowserManagedHeadless,
		ManagedExecutablePath:  cfg.BrowserManagedExecutablePath,
		ManagedUserDataDir:     cfg.BrowserManagedUserDataDir,
		SiteFlowsDir:           cfg.BrowserSiteFlowsDir,
		AutoLoginSiteAllowlist: cfg.BrowserAutoLoginSiteAllowlist,
		Vault:                  vaultReader,
		OTP:                    otpRequester,
		Relay:                  relay,
	})
}

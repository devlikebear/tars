package tarsserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/apihandlers"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/devlikebear/tars/pkg/llm"
)

// newFocusReadinessHandler wires a desktop's readiness for the phone
// (apihandlers.NewFocusReadinessHandler, docs/decisions/mobile-remote.md §2)
// to this server: the chat tier's provider, gh through the PR probe's
// binary, and Tailscale through the same detection as remote access.
func newFocusReadinessHandler(cfg config.Config) http.Handler {
	port := effectiveRemoteAccessPort(cfg.RemoteAccessTailscaleServeHTTPSPort)
	return apihandlers.NewFocusReadinessHandler(apihandlers.ReadinessProbes{
		ModelKind: func() string { return chatProviderKind(cfg) },
		FindCLI:   findProviderCLI,
		GH:        focusprobe.GHAuthState,
		Tailscale: func(ctx context.Context) (remoteaccess.Status, error) {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return remoteaccess.Detect(ctx, remoteaccess.Options{HTTPSPort: port, TargetURL: remoteaccess.DefaultTargetURL})
		},
	})
}

// chatProviderKind is the provider kind behind the chat role's tier; ""
// when none resolves.
func chatProviderKind(cfg config.Config) string {
	resolved, _ := resolveChatTier(cfg)
	return resolved.Kind
}

// findProviderCLI finds the binary a CLI provider kind runs, the way the
// provider itself does.
func findProviderCLI(kind string) error {
	var err error
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "claude-code-cli":
		_, err = llm.FindClaudeCodeCLIPath()
	case "antigravity-cli":
		_, err = llm.FindAntigravityCLIPath()
	}
	return err
}

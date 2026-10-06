package llm

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/llmdefaults"
)

// ValidateProviderAuthMode checks the constructor's authentication-mode contract
// without resolving credentials, reading environment variables, or creating clients.
// CLI constructors intentionally ignore the mode; keep that existing contract.
func ValidateProviderAuthMode(kind, mode string) error {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "claude-code-cli" || kind == "antigravity-cli" {
		return nil
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		if defaults, ok := llmdefaults.ForKind(kind); ok {
			mode = defaults.AuthMode
		}
		if mode == "" {
			mode = "api-key"
		}
	}
	if mode != "oauth" && mode != "api-key" {
		return fmt.Errorf("%s unsupported auth mode: %s", kind, mode)
	}
	return nil
}

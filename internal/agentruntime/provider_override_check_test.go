package agentruntime

import (
	"errors"
	"testing"
)

func TestCheckProviderOverride(t *testing.T) {
	unknown := errors.New(`unknown provider alias "default"`)
	runtime := NewRuntime(RuntimeOptions{
		Enabled: true, WorkspaceDir: t.TempDir(),
		ResolveProviderOverride: func(_ string, override *ProviderOverride) (ResolvedProviderOverride, error) {
			if override.Alias != "anthropic" {
				return ResolvedProviderOverride{}, unknown
			}
			return ResolvedProviderOverride{}, nil
		},
	})

	if err := runtime.CheckProviderOverride("standard", nil); err != nil {
		t.Fatalf("no override should pass, got %v", err)
	}
	if err := runtime.CheckProviderOverride("standard", &ProviderOverride{Alias: "anthropic"}); err != nil {
		t.Fatalf("a configured alias should pass, got %v", err)
	}
	if err := runtime.CheckProviderOverride("standard", &ProviderOverride{Alias: "default"}); !errors.Is(err, unknown) {
		t.Fatalf("an unknown alias should be refused, got %v", err)
	}

	withoutResolver := NewRuntime(RuntimeOptions{Enabled: true, WorkspaceDir: t.TempDir()})
	if err := withoutResolver.CheckProviderOverride("standard", &ProviderOverride{Alias: "default"}); err != nil {
		t.Fatalf("a runtime without a resolver cannot judge, got %v", err)
	}
}

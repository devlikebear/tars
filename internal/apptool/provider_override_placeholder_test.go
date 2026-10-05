package apptool

import (
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agentruntime"
)

func TestResolveTaskProviderOverride(t *testing.T) {
	newRuntime := func(aliases ...string) *agentruntime.Runtime {
		return agentruntime.NewRuntime(agentruntime.RuntimeOptions{
			Enabled: true, WorkspaceDir: t.TempDir(),
			ResolveProviderOverride: func(_ string, override *agentruntime.ProviderOverride) (agentruntime.ResolvedProviderOverride, error) {
				for _, alias := range aliases {
					if alias == override.Alias {
						return agentruntime.ResolvedProviderOverride{}, nil
					}
				}
				return agentruntime.ResolvedProviderOverride{}, errors.New(`unknown provider alias "` + override.Alias + `"`)
			},
		})
	}
	runtime := newRuntime("anthropic")

	if got, message := resolveTaskProviderOverride(runtime, "", nil); got != nil || message != "" {
		t.Fatalf("no override: got %+v %q", got, message)
	}
	if got, message := resolveTaskProviderOverride(runtime, "", &agentruntime.ProviderOverride{Alias: "anthropic"}); got == nil || got.Alias != "anthropic" || message != "" {
		t.Fatalf("configured alias: got %+v %q", got, message)
	}
	// A filler value the model put in the optional field is no override.
	for _, alias := range []string{"default", "Auto", "none", " "} {
		if got, message := resolveTaskProviderOverride(runtime, "", &agentruntime.ProviderOverride{Alias: alias, Model: "default"}); got != nil || message != "" {
			t.Fatalf("placeholder %q: got %+v %q", alias, got, message)
		}
	}
	// Any other unknown alias is a mistake worth reporting.
	got, message := resolveTaskProviderOverride(runtime, "", &agentruntime.ProviderOverride{Alias: "opnai"})
	if got != nil || !strings.Contains(message, `unknown provider alias "opnai"`) {
		t.Fatalf("unknown alias: got %+v %q", got, message)
	}
	// A provider really configured as "default" is kept.
	if got, message := resolveTaskProviderOverride(newRuntime("default"), "", &agentruntime.ProviderOverride{Alias: "default"}); got == nil || message != "" {
		t.Fatalf("configured \"default\": got %+v %q", got, message)
	}
}

package embodiment

import (
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	t.Run("register get capabilities and enabled providers", func(t *testing.T) {
		registry := NewRegistry()
		if !registry.Empty() {
			t.Fatal("new registry should be empty")
		}

		err := registry.Register(ProviderDescriptor{
			Name:         " host ",
			Enabled:      true,
			Transport:    TransportMCP,
			Endpoint:     "stackchan",
			Capabilities: []Capability{CapabilitySpeech, "", CapabilitySpeech, CapabilityExpression},
		})
		if err != nil {
			t.Fatalf("register provider: %v", err)
		}
		if registry.Empty() {
			t.Fatal("registry with an enabled provider should not be empty")
		}

		got, ok := registry.Get("host")
		if !ok {
			t.Fatal("registered provider not found")
		}
		if got.Name != "host" {
			t.Fatalf("provider name = %q, want host", got.Name)
		}
		if strings.Join(capabilityStrings(got.Capabilities), ",") != "speech,expression" {
			t.Fatalf("capabilities = %+v", got.Capabilities)
		}

		enabled := registry.Enabled()
		if len(enabled) != 1 || enabled[0].Name != "host" {
			t.Fatalf("enabled providers = %+v", enabled)
		}

		caps := registry.CapabilitiesOf("host")
		caps[0] = CapabilityVision
		again := registry.CapabilitiesOf("host")
		if again[0] != CapabilitySpeech {
			t.Fatalf("capabilities should be returned as a copy, got %+v", again)
		}
	})

	t.Run("duplicate and empty names are rejected", func(t *testing.T) {
		registry := NewRegistry()
		if err := registry.Register(ProviderDescriptor{Name: "host"}); err != nil {
			t.Fatalf("register host: %v", err)
		}
		if err := registry.Register(ProviderDescriptor{Name: " host "}); err == nil {
			t.Fatal("expected duplicate provider name to fail")
		}
		if err := registry.Register(ProviderDescriptor{Name: "   "}); err == nil {
			t.Fatal("expected empty provider name to fail")
		}
	})

	t.Run("disabled providers do not make registry active", func(t *testing.T) {
		registry := NewRegistry()
		if err := registry.Register(ProviderDescriptor{Name: "host", Enabled: false}); err != nil {
			t.Fatalf("register disabled provider: %v", err)
		}
		if !registry.Empty() {
			t.Fatal("registry with only disabled providers should be empty")
		}
		if enabled := registry.Enabled(); len(enabled) != 0 {
			t.Fatalf("enabled providers = %+v, want empty", enabled)
		}
	})
}

func capabilityStrings(values []Capability) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

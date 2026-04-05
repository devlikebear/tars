package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func makeTool(name string) Tool {
	return Tool{
		Name:        name,
		Description: "test tool",
		Parameters:  json.RawMessage(`{}`),
	}
}

func TestRegistryScopeString(t *testing.T) {
	cases := []struct {
		scope RegistryScope
		want  string
	}{
		{RegistryScopeAny, "any"},
		{RegistryScopeUser, "user"},
		{RegistryScopePulse, "pulse"},
		{RegistryScopeReflection, "reflection"},
	}
	for _, c := range cases {
		if got := c.scope.String(); got != c.want {
			t.Errorf("RegistryScope(%d).String() = %q, want %q", int(c.scope), got, c.want)
		}
	}
}

func TestNewRegistryDefaultsToAnyScope(t *testing.T) {
	r := NewRegistry()
	if r.Scope() != RegistryScopeAny {
		t.Fatalf("NewRegistry() scope = %v, want RegistryScopeAny", r.Scope())
	}
}

func TestNewRegistryWithScopeStoresScope(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopePulse)
	if r.Scope() != RegistryScopePulse {
		t.Fatalf("scope = %v, want pulse", r.Scope())
	}
}

func TestRegistryAnyScopeAcceptsAnyPrefix(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeAny)
	// Should not panic for any prefix.
	names := []string{"read_file", "ops_status", "pulse_decide", "reflection_compact"}
	for _, n := range names {
		r.Register(makeTool(n))
	}
	if got := len(r.All()); got != len(names) {
		t.Fatalf("registered %d tools, want %d", got, len(names))
	}
}

func TestRegistryUserScopeRejectsPulsePrefix(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeUser)
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic for pulse_ tool in user scope")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("panic value not string: %T %v", rec, rec)
		}
		if !strings.Contains(msg, "pulse_decide") || !strings.Contains(msg, "user scope") {
			t.Errorf("panic message missing expected parts: %q", msg)
		}
	}()
	r.Register(makeTool("pulse_decide"))
}

func TestRegistryUserScopeRejectsReflectionPrefix(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeUser)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for reflection_ tool in user scope")
		}
	}()
	r.Register(makeTool("reflection_compact"))
}

func TestRegistryUserScopeAllowsNormalTools(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeUser)
	// Canonical user-facing tools must register without panicking.
	normal := []string{"read_file", "exec", "memory_search", "cron"}
	for _, n := range normal {
		r.Register(makeTool(n))
	}
	if got := len(r.All()); got != len(normal) {
		t.Fatalf("registered %d, want %d", got, len(normal))
	}
}

func TestRegistryUserScopeRejectsOpsPrefix(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeUser)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for ops_ tool in user scope")
		}
	}()
	r.Register(makeTool("ops_status"))
}

func TestRegistryPulseScopeRejectsOpsAndReflection(t *testing.T) {
	cases := []string{"ops_status", "reflection_compact"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewRegistryWithScope(RegistryScopePulse)
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for %q in pulse scope", name)
				}
			}()
			r.Register(makeTool(name))
		})
	}
}

func TestRegistryPulseScopeAllowsPulsePrefix(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopePulse)
	r.Register(makeTool("pulse_decide"))
	if _, ok := r.Get("pulse_decide"); !ok {
		t.Fatal("pulse_decide not found after Register")
	}
}

func TestRegistryReflectionScopeRejectsOpsAndPulse(t *testing.T) {
	cases := []string{"ops_status", "pulse_decide"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewRegistryWithScope(RegistryScopeReflection)
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for %q in reflection scope", name)
				}
			}()
			r.Register(makeTool(name))
		})
	}
}

func TestRegistryReflectionScopeAllowsReflectionPrefix(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeReflection)
	r.Register(makeTool("reflection_compact"))
	if _, ok := r.Get("reflection_compact"); !ok {
		t.Fatal("reflection_compact not found after Register")
	}
}

func TestRegistryPanicMessageIncludesPrefixAndScope(t *testing.T) {
	r := NewRegistryWithScope(RegistryScopeUser)
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic")
		}
		msg := rec.(string)
		// Must include tool name, scope name, and forbidden prefix.
		for _, want := range []string{`"pulse_decide"`, "user", `"pulse_"`} {
			if !strings.Contains(msg, want) {
				t.Errorf("panic message %q missing %q", msg, want)
			}
		}
	}()
	r.Register(makeTool("pulse_decide"))
}

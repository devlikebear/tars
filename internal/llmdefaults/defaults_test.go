package llmdefaults

import "testing"

func TestAntigravityCLIDefaults(t *testing.T) {
	got, ok := ForKind(" Antigravity-CLI ")
	if !ok {
		t.Fatal("antigravity-cli defaults are not registered")
	}
	if got.AuthMode != "cli" || got.AuthModeWhenAPIKeyAbsent != "cli" {
		t.Errorf("auth defaults = %+v, want cli", got)
	}
	if got.BaseURL != "" || got.Model != "" || got.OAuthProvider != "" || len(got.APIKeyEnv) != 0 {
		t.Errorf("antigravity-cli must leave endpoint, model, OAuth, and API-key defaults empty: %+v", got)
	}
}

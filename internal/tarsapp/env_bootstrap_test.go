package tarsapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/secrets"
)

func TestLoadRuntimeEnvFiles_PrecedenceAndSecretRegistration(t *testing.T) {
	secrets.ResetForTests()
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	secretPath := filepath.Join(tmp, ".env.secret")

	const plainKey = "TARS_TEST_PLAIN_VALUE"
	const tokenKey = "TARS_TEST_API_TOKEN"
	const customKey = "TARS_TEST_CUSTOM_VALUE"

	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		plainKey + "=from_env",
		tokenKey + "=from_env_token",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(secretPath, []byte(strings.Join([]string{
		tokenKey + "=from_secret_token",
		customKey + "=from_secret_custom",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write .env.secret: %v", err)
	}

	prevPlain, hadPlain := os.LookupEnv(plainKey)
	prevToken, hadToken := os.LookupEnv(tokenKey)
	prevCustom, hadCustom := os.LookupEnv(customKey)
	t.Cleanup(func() {
		restoreEnvValue(plainKey, prevPlain, hadPlain)
		restoreEnvValue(tokenKey, prevToken, hadToken)
		restoreEnvValue(customKey, prevCustom, hadCustom)
	})
	_ = os.Unsetenv(plainKey)
	_ = os.Unsetenv(tokenKey)
	_ = os.Unsetenv(customKey)
	_ = os.Setenv(plainKey, "from_os_plain")

	loadRuntimeEnvFiles(envPath, secretPath)

	if got := os.Getenv(plainKey); got != "from_os_plain" {
		t.Fatalf("expected OS env precedence, got %q", got)
	}
	if got := os.Getenv(tokenKey); got != "from_secret_token" {
		t.Fatalf("expected .env.secret precedence over .env, got %q", got)
	}
	if got := os.Getenv(customKey); got != "from_secret_custom" {
		t.Fatalf("expected .env.secret variable loaded, got %q", got)
	}

	redacted := secrets.RedactText("token=from_secret_token custom=from_secret_custom")
	if strings.Contains(redacted, "from_secret_token") || strings.Contains(redacted, "from_secret_custom") {
		t.Fatalf("expected .env.secret values redacted, got %q", redacted)
	}
}

func restoreEnvValue(key, value string, had bool) {
	if had {
		_ = os.Setenv(key, value)
		return
	}
	_ = os.Unsetenv(key)
}

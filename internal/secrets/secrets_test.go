package secrets

import (
	"strings"
	"testing"
)

func TestLooksSensitiveKey_PrefixSuffixKeywords(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{key: "OPENAI_API_KEY", want: true},
		{key: "TOKEN_GITHUB", want: true},
		{key: "MY_PRIVATE_CERT", want: true},
		{key: "DB_PASSWORD", want: true},
		{key: "SERVICE_PASSWD", want: true},
		{key: "NORMAL_VARIABLE", want: false},
	}
	for _, tc := range cases {
		got := LooksSensitiveKey(tc.key)
		if got != tc.want {
			t.Fatalf("LooksSensitiveKey(%q)=%v want=%v", tc.key, got, tc.want)
		}
	}
}

func TestRedactText_ForcedSecretValue(t *testing.T) {
	ResetForTests()
	secretValue := "forced_secret_value_1234567890"
	RegisterForced("ANY_NAME", secretValue)

	out := RedactText("payload=" + secretValue)
	if strings.Contains(out, secretValue) {
		t.Fatalf("expected forced secret value to be redacted, got %q", out)
	}
}

func TestRegisterNamed_OnlySensitiveKeysAreRegistered(t *testing.T) {
	ResetForTests()
	regular := "regular_value_1234567890"
	RegisterNamed("REGULAR_NAME", regular)
	if got := RedactText(regular); got != regular {
		t.Fatalf("expected non-sensitive key to remain unchanged, got %q", got)
	}

	sensitive := "named_secret_value_1234567890"
	RegisterNamed("SERVICE_TOKEN", sensitive)
	out := RedactText(sensitive)
	if strings.Contains(out, sensitive) {
		t.Fatalf("expected sensitive key value to be redacted, got %q", out)
	}
}

func TestRedactText_RedactsBearerAndJSONFields(t *testing.T) {
	ResetForTests()
	out := RedactText(`{"token":"sample-token"} authorization=Bearer sample-token`)
	if strings.Contains(out, "sample-token") {
		t.Fatalf("expected token string redacted, got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), `"token":"***"`) {
		t.Fatalf("expected json token redaction, got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "authorization=***") {
		t.Fatalf("expected bearer redaction, got %q", out)
	}
}

package llm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/secrets"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

func TestLogLLMRequestPayload_RedactsSensitiveValues(t *testing.T) {
	secrets.ResetForTests()
	secret := "sk_test_request_secret_value_1234567890"
	secrets.RegisterNamed("OPENAI_API_KEY", secret)

	var buf bytes.Buffer
	prev := zlog.Logger
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	defer func() { zlog.Logger = prev }()

	logLLMRequestPayload("openai", []byte(`{"api_key":"`+secret+`","message":"hi"}`))
	logged := buf.String()
	if strings.Contains(logged, secret) {
		t.Fatalf("expected request payload redaction, got %q", logged)
	}
}

func TestLogLLMStreamPayload_RedactsSensitiveValues(t *testing.T) {
	secrets.ResetForTests()
	secret := "sk_test_stream_secret_value_1234567890"
	secrets.RegisterNamed("OPENAI_API_KEY", secret)

	var buf bytes.Buffer
	prev := zlog.Logger
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	defer func() { zlog.Logger = prev }()

	logLLMStreamPayload("openai", `{"delta":"`+secret+`"}`)
	logged := buf.String()
	if strings.Contains(logged, secret) {
		t.Fatalf("expected stream payload redaction, got %q", logged)
	}
}

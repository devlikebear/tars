package tarsserver

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/rs/zerolog"
)

func TestLogE2EResult(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	logE2EResult(log, focuspipeline.VerificationResult{Command: "make test", Passed: true}, time.Second)
	if buf.Len() != 0 {
		t.Fatalf("a shell command was logged: %s", buf.String())
	}
	logE2EResult(log, focuspipeline.VerificationResult{Command: "@Browser http://127.0.0.1:1/ x", Passed: true, E2E: true, Excerpt: "VERDICT: PASS secret page text"}, time.Second)
	line := buf.String()
	if !strings.Contains(line, `"level":"info"`) || !strings.Contains(line, `"passed":true`) || !strings.Contains(line, "@Browser http://127.0.0.1:1/ x") {
		t.Fatalf("pass line = %s", line)
	}
	if strings.Contains(line, "secret page text") {
		t.Fatalf("the checker's transcript was logged: %s", line)
	}
	buf.Reset()
	logE2EResult(log, focuspipeline.VerificationResult{Command: strings.Repeat("g", 300), E2E: true}, time.Second)
	if line := buf.String(); !strings.Contains(line, `"level":"warn"`) || strings.Contains(line, strings.Repeat("g", 201)) {
		t.Fatalf("fail line = %s", line)
	}
}

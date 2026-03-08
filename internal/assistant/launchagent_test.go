package assistant

import (
	"strings"
	"testing"
)

func TestBuildLaunchAgentPlist(t *testing.T) {
	plist := BuildLaunchAgentPlist(LaunchAgentConfig{
		Label:            "io.tars.assistant",
		ProgramArguments: []string{"/usr/local/bin/tars", "assistant", "start", "--server-url", "http://127.0.0.1:43180"},
		WorkingDirectory: "/tmp/tars-workspace",
		StdoutPath:       "/tmp/tars-assistant.out.log",
		StderrPath:       "/tmp/tars-assistant.err.log",
		KeepAlive:        true,
		RunAtLoad:        true,
	})
	if plist == "" {
		t.Fatalf("expected plist content")
	}
	mustContain := []string{
		"<key>Label</key>",
		"io.tars.assistant",
		"/usr/local/bin/tars",
		"assistant",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
	}
	for _, token := range mustContain {
		if !strings.Contains(plist, token) {
			t.Fatalf("expected plist to contain %q, got: %s", token, plist)
		}
	}
}
